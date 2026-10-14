# Plan: Filesystem Read-Only Alert

> Status: **implemented on this branch** (agent + hub alert + UI + Docker compose notes). Rebuild/push images when ready.

## Goal

Detect when a **host** filesystem watched by the Beszel agent suddenly becomes read-only (typical kernel remount after I/O errors) and:

1. Highlight the problem in the UI immediately
2. Send an alert with no delay / no numeric threshold (binary state, like `SystemdFailed`)

Scope: **host filesystems on the machine running the agent** (root + configured extra disks). Not Docker container rootfs quirks, not intentional long-lived RO mounts.

Related need: [inode monitoring (#930)](https://github.com/henrygd/beszel/issues/930) covers exhaustion; this covers emergency remount to `ro`.

---

## Problem

Linux often remounts a filesystem read-only after disk / filesystem errors:

```text
EXT4-fs error ... Remounting filesystem read-only
```

Beszel today tracks usage, I/O, and (in our fork) inodes, but **does not inspect mount options**. Disk charts can look “fine” while the machine is already RO.

---

## Detection approach (no write probe)

Use mount flags from the kernel mount table (`ro` / `rw`), via gopsutil `disk.PartitionStat.Opts` or by parsing host `mountinfo`.

**Do not** probe with temporary file creation:

- Fails on intentional RO binds
- Can race with full disks / permissions
- Unnecessary if mount options are available

### Alert semantics

| Case | Action |
|------|--------|
| Watched FS was `rw`, now `ro` | **Trigger** alert |
| Watched FS stays `ro` (already triggered) | No re-notify |
| Watched FS becomes `rw` again | **Resolve** alert |
| FS permanently intended as `ro` (ISO, ostree `/usr`, Docker bind `:ro` only) | **Do not** treat as failure |

Recommended rule: alert only on **transition `rw → ro`** for filesystems Beszel already monitors (root + extras), optionally with an allowlist/env of “must stay writable” paths.

---

## Binary / systemd agent (host namespace)

Agent shares the host mount namespace → `disk.Partitions(true)` opts reflect the real host mounts.

### Behavior

1. For each entry in `fsStats` (root + `EXTRA_FILESYSTEMS` / discovered mounts), resolve current opts for its `Mountpoint`.
2. Set `ReadOnly bool` (and maybe previous sample for transition) on `FsStats` / `Info`.
3. Hub evaluates binary alert `FilesystemReadOnly` (name TBD) when any watched FS reports unexpected RO.

### Config

Usually none beyond existing disk config:

```bash
FILESYSTEM=nvme0n1p1          # optional root override
EXTRA_FILESYSTEMS=sdb1,/mnt/data__Data
```

Ensure the agent user can read mount info (normal for root / typical agent user).

---

## Docker agent — why `:ro` volumes are not enough

Official extra-disk pattern:

```yaml
volumes:
  - /mnt/disk1/.beszel:/extra-filesystems/disk1:ro
```

Inside the container that bind is **always** `ro`, even when the host FS is healthy `rw`.  
Checking container-local opts for `/extra-filesystems/...` would **false-positive forever**.

Emergency remount happens on the **host superblock**. To see it from Docker, the agent must read the **host** mount table (or an equivalent host view), not the container bind flags.

### Required volume / config pattern

#### A. Host mount table (recommended minimum)

Bind host init mountinfo into the container:

```yaml
services:
  beszel-agent:
    image: antirek/beszel2-agent:latest   # or henrygd/beszel-agent
    network_mode: host
    volumes:
      - ./beszel_agent_data:/var/lib/beszel-agent
      - /var/run/docker.sock:/var/run/docker.sock:ro
      # Host mount table (PID 1 on the host = real host mounts when sourced from the host path)
      - /proc/1/mountinfo:/host/proc/1/mountinfo:ro
      # Extra disks (usage/inodes); :ro is fine for usage
      - /mnt/disk1/.beszel:/extra-filesystems/disk1:ro
      - /mnt/data/.beszel:/extra-filesystems/sda1__Data:ro
    environment:
      LISTEN: 45876
      KEY: "<public key>"
      HUB_URL: "<hub url>"
      TOKEN: "<token>"
      # Path to host mountinfo inside the container
      HOST_MOUNTINFO: /host/proc/1/mountinfo
      FILESYSTEM: nvme0n1p1   # recommended in containers
```

Agent logic:

1. Prefer `HOST_MOUNTINFO` (or `/host/proc/1/mountinfo` if present).
2. Map each watched FS to a **host** mountpoint/device (from `FILESYSTEM` / folder name / `EXTRA_FILESYSTEMS`).
3. Read `ro`/`rw` from that host mountinfo line — **ignore** container bind opts for `/extra-filesystems/*`.

#### B. Host root bind (optional, stronger)

Useful when root usage itself must come from the host (and for aligning device ↔ mountpoint):

```yaml
volumes:
  - /:/host-root:ro,rslave
  - /proc/1/mountinfo:/host/proc/1/mountinfo:ro
environment:
  # Point root stats at the real host FS if needed
  FILESYSTEM: /host-root   # or device name still preferred for I/O
  HOST_MOUNTINFO: /host/proc/1/mountinfo
  HOST_ROOT: /host-root
```

Notes:

- `rslave` helps nested mounts show up correctly.
- Still use host mountinfo for RO detection; `/host-root` bind may itself be `ro` by design.

#### C. What not to do

| Anti-pattern | Why |
|--------------|-----|
| Alert if `/extra-filesystems/x` opts contain `ro` | Always true with `:ro` binds |
| Only mount usage dirs, no host mountinfo | Cannot see host remount |
| Write-probe under `:ro` bind | Always fails; useless |

### Helm sketch

```yaml
volumeMounts:
  - name: host-mountinfo
    mountPath: /host/proc/1/mountinfo
    readOnly: true
  - name: extra-disk1
    mountPath: /extra-filesystems/disk1
    readOnly: true
volumes:
  - name: host-mountinfo
    hostPath:
      path: /proc/1/mountinfo
      type: File
  - name: extra-disk1
    hostPath:
      path: /mnt/disk1/.beszel
      type: DirectoryOrCreate
env:
  HOST_MOUNTINFO: /host/proc/1/mountinfo
```

---

## Data model

### Agent → hub

Extend existing structs (JSON/CBOR, new keys):

```text
FsStats:
  ReadOnly bool   // current host mount is ro

Info (dashboard):
  RootReadOnly bool
  ExtraFsReadOnly map[string]bool   // keyed like ExtraFsPct
```

Optional: list of RO mount names for alert body (`["/", "Data"]`).

### Hub alert

- Name: `FilesystemReadOnly` (select field + migration, same as `Inodes`)
- Type: binary / `noThreshold` / `noDuration` (like `SystemdFailed`)
- Trigger: any watched FS unexpected RO
- Resolve: none remain RO
- Notification body: list affected mount/display names

### Frontend

- Alert sheet entry (icon + description)
- Systems table / disk tooltip: badge or “RO” next to affected disk
- System page: warning on disk/inode chart card when RO

---

## Implementation outline

### Phase 1 — Agent

1. Helper `isReadOnlyMount(opts []string) bool` (`ro` present and not overridden by later `rw` if applicable).
2. `resolveHostMountOpts(mountpoint|device)`:
   - binary: from `disk.Partitions`
   - docker: parse `HOST_MOUNTINFO` / `/host/proc/1/mountinfo`, match by mountpoint or major:minor/device
3. On each disk usage refresh (same cadence as usage), set `ReadOnly` on `fsStats` and mirror into `Info`.
4. Env: `HOST_MOUNTINFO` (optional), document Docker requirement.

### Phase 2 — Hub alerts

1. Migration: add `FilesystemReadOnly` to `alerts.name` values.
2. Handler similar to `HandleSystemdAlerts` (state change only, immediate).
3. Wire from system update path alongside other system alerts.
4. Tests: trigger on `rw→ro`, no duplicate notify, resolve on `ro→rw`, ignore missing mountinfo.

### Phase 3 — UI

1. `alertInfo.FilesystemReadOnly` in `alerts.ts`.
2. Disk cell / chart indicators.
3. Docs / compose examples under `supplemental/docker/agent`.

### Phase 4 — Docs & packaging (our infra)

1. Update internal compose to mount `/proc/1/mountinfo`.
2. Rebuild/push `antirek/beszel2` + `antirek/beszel2-agent`.
3. Optional: upstream PR + docs PR on beszel-docs.

---

## Edge cases

| Case | Handling |
|------|----------|
| Immutable OS (`/sysroot`, ostree) | Use existing `getRootMountPoint` / `FILESYSTEM`; only alert on watched writable mounts |
| Network FS (NFS) remounted RO | Same opts logic if present in host mountinfo |
| ZFS dataset | Match host mountpoint used for usage; ZFS list has no RO flag — rely on mountinfo |
| Mountinfo missing in Docker | Log warning once; skip RO alert (do not false-trigger) |
| Agent cannot read mountinfo | Same as missing — degrade gracefully |
| Windows | Out of scope initially (or use volume attributes later) |
| Intentional host RO mount added to extras | Prefer transition-only; or `READWRITE_FILESYSTEMS=/mnt/data` allowlist |

---

## Testing plan

**Unit**

- Parse mountinfo lines → `ro`/`rw`
- Match device/`__Name` folder to host mount
- Alert state machine (trigger / hold / resolve)

**Integration**

- Binary: remount a test loop/tmpfs `ro` and assert alert
- Docker: host tmpfs remounted RO while bind is `:ro` — alert only if host mountinfo says `ro`

**Manual (infra)**

```bash
# careful / lab only
sudo mount -o remount,ro /mnt/testdisk
# expect alert within one agent poll
sudo mount -o remount,rw /mnt/testdisk
# expect resolve
```

---

## Docker compose checklist (ops)

For each host with a Dockerized agent that should alert on emergency RO:

- [ ] Mount host mountinfo: `/proc/1/mountinfo:/host/proc/1/mountinfo:ro`
- [ ] Set `HOST_MOUNTINFO=/host/proc/1/mountinfo`
- [ ] Set `FILESYSTEM` to the correct host device/partition
- [ ] Mount each extra disk under `/extra-filesystems/...` ( `:ro` OK for usage)
- [ ] Enable alert **Filesystem Read-Only** in the hub UI for that system
- [ ] Confirm agent logs do not warn about missing mountinfo

Minimal example:

```yaml
volumes:
  - ./beszel_agent_data:/var/lib/beszel-agent
  - /var/run/docker.sock:/var/run/docker.sock:ro
  - /proc/1/mountinfo:/host/proc/1/mountinfo:ro
  - /mnt/data/.beszel:/extra-filesystems/data:ro
environment:
  HOST_MOUNTINFO: /host/proc/1/mountinfo
  FILESYSTEM: sda1
```

---

## Open decisions

1. **Alert name**: `FilesystemReadOnly` vs `DiskReadOnly` vs `MountReadOnly`.
2. **Scope default**: root only vs root + all extras.
3. **Allowlist env** (`READWRITE_FILESYSTEMS`) vs pure transition detection.
4. **UI urgency**: treat like Status-down (strong) vs Disk warning (softer).
5. Ship only on fork (`antirek/beszel2*`) first, then upstream PR?

---

## Success criteria

- Host root remounted RO → alert within one agent stats cycle, UI shows RO.
- Remount RW → alert resolves.
- Docker agent with documented volumes behaves the same as binary for host FS.
- Extra disks mounted `:ro` in Docker do **not** alert unless the **host** mount is RO.
- No write probes; no false positives from intentional bind RO.
