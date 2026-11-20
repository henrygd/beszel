# Fork Sync Ledger — upstream 2026-11

Fork point: `c1c1cd1b` (upstream, 2026-04-20). Histories had no merge base
(fork root was squashed), so the fork root was grafted onto the fork point
and merged three-way into `upstream/main` (v0.21.2) on branch
`sync/upstream-2026-11`.

## Preserved (fork-specific)

| Area | State |
|------|-------|
| `internal/hub/monitors/` uptime checks, heartbeats, scheduler | kept |
| `internal/hub/domains/` WHOIS, expiry, SSL, DNS, subdomains | kept |
| `internal/hub/incidents/`, `statuspages/`, `maintenance/`, `badges/` | kept |
| `internal/hub/notifications/` (7 providers + web push) | kept |
| `internal/hub/pagespeed/`, `export/`, `bulk/`, `settings/` | kept |
| `internal/hub/app_update.go` in-app updates | kept (superset UpdateInfo) |
| Site: monitors/domains/status-pages/calendar routes, PWA | kept |
| Navbar "Monitoring" + lang-toggle | kept |
| monitor entity status/type constants | merged with upstream Config |
| Docker workflow | upstream matrix + fork GHCR-flattened image names |

## Taken from upstream (supersedes fork equivalents)

| Area | Reason |
|------|--------|
| `systems/`, `heartbeat/`, `transport/`, `ws/` | updater WaitGroup, status mutex, monitor sync, ZFS |
| `mcp/` server, API keys, `internal/email` | new features, no fork overlap |
| Agent network monitors (ping/cert/http probes) | complementary to hub-side uptime checks |
| `ganigeorgiev.com/pocketbase` v0.42.2 | old pocketbase module archived |
| `release.yml`, `vulncheck.yml` | fork's deletions reverted intentionally |
| `systems-table.tsx` | fork drag/drop never persisted (dead code) |
| `types.d.ts` `icon: any` | upstream relaxation, fork's LucideIcon too strict |

## Kept deleted (fork intent)

| File | Reason |
|------|--------|
| `.github/workflows/inactivity-actions.yml` | upstream community janitor, unwanted |
| `SECURITY.md` | pointed at upstream advisories |
| `test_data/` | contained a private SSH key — ROTATE IT |

## Removed during sync

| File | Reason |
|------|--------|
| `hub` (root) | committed x86-64 dev build; now gitignored |
| `reference/` gitlinks | dead entries, no .gitmodules |

## Skipped

None — the delta applied wholesale; conflicts were per-file, not per-commit.

## Verify state at head

- `bun run build` (internal/site): PASS
- `go build ./...`: PASS
- `go vet ./...`: PASS
- `go test ./...`: PASS
- `gofmt -l`: clean

## Fork releases

Tag scheme: `v<upstream-version>-fork.<n>` — the fork syncs origin is
upstream's tag, and the suffix keeps fork tags from ever colliding with an
upstream tag on a future sync.

What a tag triggers (`release.yml`, `docker-images.yml`, `helm-charts.yml`):

- GoReleaser creates a **draft** release; publish it by hand after checking
  the assets (`gh release edit <tag> --draft=false --latest`).
- Container images go to `ghcr.io/dvorinka/*`. Because the tag is a
  semver *prerelease*, `metadata-action`'s `latest=auto` never applies, so
  `docker-images.yml` publishes `:latest` explicitly on tag pushes. Without
  that line `:latest` silently stays behind — and both the README quick
  start and the in-app updater (`internal/hub/app_update.go`) pull it.
- Helm charts publish to `ghcr.io/dvorinka/beszel-charts`. Pointing this at
  upstream's namespace makes CI fail, because upstream already published
  those chart versions.

Container builds pin Go in the Dockerfiles, not in CI: when `go.mod`'s
`go` directive outruns `golang:x.y.z` in `internal/dockerfile_*`, every
image job dies at `go mod download`. Bump both together.

## Verified against a live hub

- Existing `beszel_data` dir migrated to PocketBase v0.42.2 and booted
  clean; `:latest` image booted clean on a fresh volume (`/api/health` 200,
  site served, schedulers running).
- Domain rows show live WHOIS data (registrar, expiry); public status page
  renders per-check bars, error text and correct uptime on desktop and
  390px mobile; monitor detail charts and incident banner render.

## Known debt / open items

- `test_data/id_ed25519` remains in git history on older branches; key
  must be rotated everywhere it was authorized.
- `docker-images.yml` publishes `edge` on tag pushes as well as on `main`,
  so tagging an old commit would move `edge` backwards. Only matters if a
  release is cut from a non-`main` commit.
