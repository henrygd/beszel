## 0.12.8

- Add setting for time format (12h / 24h). (#424)

- Add experimental one-time password (OTP) support.

- Add `TRUSTED_AUTH_HEADER` environment variable for authentication forwarding. (#399)

- Add `AUTO_LOGIN` environment variable for automatic login. (#399)

- Add FreeBSD support for agent install script and update command.

## 0.12.7

- Make LibreHardwareMonitor opt-in with `LHM=true` environment variable. (#1130)

- Fix bug where token was not refreshed when adding a new system. (#1141)

- Add `USER_EMAIL` and `USER_PASSWORD` environment variables to set the email and password of the initial user. (#1137)

- Display system counts (active, paused, down) in All Systems 'view' options. (#1078)

- Remember All Systems sort order during session.

## 0.12.6

- Add maximum 1 minute memory usage.

- Add status filters to All Systems table.

- Virtualize All Systems table to improve performance with hundreds of systems. (#1100)

- Fix Safari system link CSS bug.

- Use older cuda image for increased compatibility (#1103)

- Truncate long system names in All Systems table. (#1104)

- Fix update mirror and add `--china-mirrors` flag. (#1035)

## 0.12.5

- Downgrade `gopsutil` to `v4.25.6` to fix panic on FreeBSD (#1083)

- Exclude FreeBSD from battery charge monitoring to fix deadlock. (#1081)

- Minor hub UI improvements.

## 0.12.4

- Add battery charge monitoring.

- Add fallback mirror to the `update` commands. (#1035)

- Fix blank token field in insecure contexts.

- Allow opening internal router links in new tab.

- Add `/api/beszel/user-alerts` endpoint. Remove use of batch API for alerts in hub.

- Require auth for `/api/beszel/getkey` endpoint that returns the public key.

- Change `GET /api/beszel/send-test-notification` endpoint to `POST /api/beszel/test-notification`.

- Update Go and JS dependencies.

- New translations by @Radotornado, @AlexVanSteenhoven, @harupong, @dymek37, @NaNomicon, Tommaso Cavazza, Caio Garcia, and others.

## Older

Release notes are available at https://github.com/henrygd/beszel/releases
