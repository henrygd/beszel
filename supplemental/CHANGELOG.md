## 0.14.1

- Add `MFA_OTP` environment variable to enable email-based one-time password for users and/or superusers. 

## 0.14.0

- Add `/containers` page for viewing current status of all running containers. (#928)

- Add ability to view container status, health, details, and basic logs. (#928)

- Probable fix for erroneous network stats when interface resets (#1267, #1246)

# 0.13.2

- Add ability to set custom name for extra filesystems. (#379)

- Improve WebSocket agent reconnection after network interruptions. (#1263)

- Allow more latency in one minute charts before visually disconnecting points. (#1247)

- Update favicon and add add down systems count in bubble.

## 0.13.1

- Fix one minute charts on systems without Docker. (#1237)

- Change system permalinks to use ID instead of name. (#1231)

## 0.13.0

- Add one minute chart with one second interval.

- Improve accuracy of disk I/O statistics.

- Add `SYSTEM_NAME` environment variable to override system name on universal token registration. (#1184)

- Add `noindex` HTML meta tag. (#1218)

- Update Go dependencies.

## 0.12.12

- Fix high CPU usage when `intel_gpu_top` returns an error. (#1203)

- Add `SKIP_GPU` environment variable to skip GPU data collection. (#1203)

- Add fallback cache/buff memory calculation when cache/buff isn't available ([#1198](https://github.com/henrygd/beszel/issues/1198))

- Fix automatic agent update / restart on OpenRC. (#1199)

## 0.12.11

- Adjust calculation of cached memory (fixes #1187, #1196)

- Add pattern matching and blacklist functionality to `NICS` env var. (#1190)

- Update Intel GPU collector to parse plain text (`-l`) instead of JSON output (#1150)

## 0.12.10

Note that the default memory calculation changed in this release, which may cause a difference in memory usage compared to previous versions.

- Add initial support for Intel GPUs (#1150, #755)

- Show connection type (WebSocket / SSH) in hub UI.

- Fix temperature unit and bytes / bits settings. (#1180)

- Add `henrygd/beszel-agent-intel` image for Intel GPUs (experimental).

- Update Go dependencies. Shoutrrr now supports notifications for Signal and WeChat Work (WeCom).

## 0.12.9

- Fix divide by zero error introduced in 0.12.8 :) (#1175)

## 0.12.8

- Add per-interface network traffic charts. (#926)

- Add cumulative network traffic charts. (#926)

- Add setting for time format (12h / 24h). (#424)

- Add experimental one-time password (OTP) support.

- Add `TRUSTED_AUTH_HEADER` environment variable for authentication forwarding. (#399)

- Add `AUTO_LOGIN` environment variable for automatic login. (#399)

- Add FreeBSD support for agent install script and update command.

- Fix status alerts not being resolved when system comes up. (#1052)

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
