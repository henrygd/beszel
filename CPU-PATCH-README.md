## To be clear this PR is AI-assisted not fully AI based 

## The problem

On an unrooted Android device, the beszel agent (running inside Termux, built as a normal
`GOOS=linux` binary) reports **0% CPU** and no load average. The cause is SELinux, not gopsutil:

As inside the termux( unrooted environment ) there is no permission to see /proc-* data

`cpu.Times()` therefore returns an error / empty slice, and the agent publishes empty CPU
metrics forever. Android's own toybox `top` has the same restriction and works around it the
same way this PR does — by summing per-process CPU ticks — which is the model for this fix.

A second, unrelated-to-SELinux crash: **Go ≥ 1.24's `exec.LookPath` probes candidates with the
`faccessat2` syscall (439)**, which Android < 12 seccomp filters reject with **SIGSYS (bad
system call)**. The agent called `LookPath` during startup (package-update detection, GPU tool
discovery, restarter detection), so on an Android 8 device it died with SIGSYS before ever
serving a connection. Both problems are fixed here because they must be fixed together.

# Everything new is behind one gate:

```go
var androidNoProcStat = sync.OnceValue(func() bool {
    if runtime.GOOS != "linux" { return false }
    _, statErr := os.ReadFile(androidProcStatPath)
    return statErr != nil
})
```



## How the fallback works (`agent/cpu_fallback.go`)

`androidCpuTimes(perCore bool)` is a drop-in for `cpu.Times()`: it returns **cumulative**
`cpu.TimesStat` values (User = busy seconds, Idle = wall − busy) so all existing delta logic
in `cpu.go` — `cpuMetricsFromTimes`, `calculateBusy`, the 60 s baseline maps, per-core
clamping — works unchanged.

Two data sources, tried in order of quality:

### 1. Process ticks — primary

Sums `utime+stime` (fields 14/15 of `/proc/[pid]/stat`, parsed after the last `)` since
`comm` may contain spaces) over every readable PID, divided by `USER_HZ`.

- These are **genuine busy seconds**, no unit heuristics — the same estimation toybox `top`
  performs in this exact environment.
- SELinux hides other UIDs' processes, but for a Termux agent that is precisely the workload
  being monitored (the agent, sshd, tailscaled, shells…). The visibility is identical to what
  `top` on the same device shows, so the two can be cross-checked 1:1.
- Aggregate busy is split evenly across cores for the per-core view (per-core attribution is
  not recoverable without `/proc/stat`).

### 2. cpufreq residency ratios — secondary (last resort)

`/sys/devices/system/cpu/cpuN/cpufreq/stats/time_in_state`, per core:

```
idle fraction ≈ residency(lowest freq) / residency(total)
busy seconds  = (1 − idle fraction) × elapsed wall time
```

Only **ratios** of the raw counters are ever used, deliberately: the counter unit is
kernel-dependent (10 ms jiffies on most SoCs, other scales elsewhere), and many kernels
accumulate idle time into the current frequency's bucket, so treating counts as seconds pegs
the display at 100%. A second real-world trap, discovered on an MSM8917 test device:
clock-gated idle cores **stop accumulating `time_in_state` entirely**, which is why this
source is demoted to last resort. Both pitfalls are documented in the code.

### Load average

`androidLoadAvg()` synthesizes 1/5/15-min load from the `sysinfo(2)` syscall, converting the
kernel's fixed-point values with `SI_LOAD_SHIFT = 16`. This is system-wide (not per-UID) and
matches `top`'s numbers on-device.

### Robustness details

- **First call returns an empty slice** (no delta yet), matching what `init()`-time baseline
  seeding in `cpu.go` expects — the first poll after agent start simply has no CPU value,
  exactly like a fresh gopsutil baseline.
- `clampAndroid()` clamps every delta to `[0, wall]`: PID recycling or a process dying can
  only ever shrink the tick sum; clamping keeps the cumulative counters monotonic so graphs
  never dip negative.
- Per-core state resets cleanly on hotplug (core-count change re-seeds the baseline).
- The active source is logged once at startup:
  `android CPU fallback: using process ticks (/proc/[pid]/stat)`.

## Seccomp-safe exec (`agent/android_compat.go` new, `agent/utils/utils.go`)

`safeLookPath` / `safeCommand` replicate `exec.LookPath` / `exec.Command` semantics using
`os.Stat` (`newfstatat` — allowed by every Android seccomp profile) instead of `faccessat2`.
All call sites that resolve or spawn external binaries were switched:

- `gpu.go` (nvtop / nvidia-smi / rocm-smi / tegrastats / intel_gpu_top / macmon / powermetrics discovery, `gpuCollector.collect`)
- `gpu_intel.go`, `gpu_nvtop.go`
- `update.go` (restarter detection, `chown` in self-update)
- `package_updates.go` (`commandExists`)
- `agent/utils` gets its own `safeLookPathHomebrewExec` (plus `statExecutable`) because
  `utils` cannot import `agent` — `LookPathHomebrew` keeps its Homebrew fallback behavior.

On non-Android hosts `safeCommand` resolves the same path `exec.Command` would; the only
observable difference is that the not-found error surfaces at `Run()` rather than at
construction (documented in the code, same as `exec.LookPath` failing after PATH scan).

Additionally, package-update detection returns `nil` early when the gate is true: Termux's
dpkg database describes Termux packages, not the Android system, so the "OS updates" figure
would be meaningless anyway — and the detection path was one of the SIGSYS crash sites.

## Testing

**Unit** (`go test ./agent/ -run TestAndroidCpuFallback`): hermetic fixtures — an unreadable
`/proc/stat`, a fake PID with `utime=300 stime=100`, two synthetic cores with `time_in_state`
files, and an injectable clock. It asserts both the **strategy priority** (process ticks win:
~3.33% busy, and the 40% cpufreq fixture must *not* leak into the result) and the cpufreq
ratio math (~40% ± 5) when process ticks are unavailable, plus baseline-seeding semantics
(empty first sample) and per-core shape.

**On-device** (Vivo Y71, Android 8, Snapdragon 425 / MSM8917, arm64, Termux):

- Before: CPU panel empty, agent SIGSYS-crashed at startup.
- After: idle reads ~0.5%; a controlled 3-core × 5 s stress burst inside a 60 s polling window
  produced the expected elevated reading; numbers track toybox `top` on the same screen.
- Load average matches `top`'s system-wide values.

Verified `git apply` onto a pristine v0.21.1 tree, `go vet`/tests pass, and a static
`CGO_ENABLED=0 GOOS=linux GOARCH=arm64` build works.

## Known limitations (documented for reviewers)

- CPU usage covers only processes the agent's UID can see (identical to `top`'s view).
- Per-core values are an even split of the aggregate; per-core attribution needs `/proc/stat`.
- `androidNoProcStat()` also engages the fallback in any non-Android environment where
  `/proc/stat` is unreadable. I consider that a feature (currently such hosts get empty
  metrics), but it's the main semantic question worth a second opinion.
