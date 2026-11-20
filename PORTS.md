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

## Known debt / open items

- `test_data/id_ed25519` remains in git history on older branches; key
  must be rotated everywhere it was authorized.
- Hub.go startup ordering (fork schedulers vs upstream agent server)
  reviewed at merge; runtime smoke test still recommended.
- PocketBase v0.42.2 migration of fork collections compiles; live DB
  migration on an existing data dir untested.
- Functional domain-fetching correctness (WHOIS/registrar parsing)
  still needs live-data verification — compile-clean only.
