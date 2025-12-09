## 0.17.0

- Add quiet hours to silence alerts during specific time periods. (#265)

- Add dedicated S.M.A.R.T. page.

- Add alerts for S.M.A.R.T. failures.

- Add `DISK_USAGE_CACHE` environment variable. (#1426)

- Add `SKIP_SYSTEMD` environment variable. (#1448)

- Add hub builds for Windows and FreeBSD.

- Change extra disk indicators in systems table to display usage range as dots. (#1409)

- Strip ANSI escape sequences from docker logs. (#1478)

- Fix issue where the Add System button is visible to read-only users. (#1442)

- Fix font ligatures creating unwanted artifacts in random ids. (#1434)

- Update Go dependencies.

## 0.16.1

- Add services column to All Systems table. (#1153)

- Add `SERVICE_PATTERNS` environment variable to filter systemd services. (#1153)

- Fix detection and handling of immutable filesystems like Fedora Silverblue. (#1405)

- Persist alert history page size preference. (#1404)

- Add setting for layout width.

- Update Go dependencies.

## 0.16.0

- Add basic systemd service monitoring. (#1153)

- Add GPU usage alerts.

- Show additional disk percentages in systems table. (#1365)

- Embed `smartctl` in the Windows binary (experimental). (#1362)

- Add `EXCLUDE_SMART` environment variable to exclude devices from S.M.A.R.T. monitoring. (#1392)

- Change alert links to use system ID instead of name.

- Update Go dependencies.

## 0.15.4

- Refactor containers table to fix clock issue causing no results. (#1337)

- Fix Windows extra disk detection. (#1361)

- Add total line to the tooltip of charts with multiple values. (#1280)

- Add fallback paths for `smartctl` lookup. (#1362, #1363)

- Fix `intel_gpu_top` parsing when engine instance id is in column. (#1230)

- Update `henrygd/beszel-agent-nvidia` Dockerfile to build latest smartmontools. (#1335)

## 0.15.3

- Add CPU state details and per-core usage. (#1356)

- Add `EXCLUDE_CONTAINERS` environment variable to exclude containers from being monitored. (#1352)

- Add `INTEL_GPU_DEVICE` environment variable to specify Intel GPU device. (#1285)

- Improve parsing of edge case S.M.A.R.T. power on times. (#1347)

- Fix empty disk I/O values for extra disks. (#1355)

- Fix battery nil pointer error. (#1353)

- Add Hebrew with translations by @gabay.

- Update `shoutrrr` and `gopsutil` dependencies.

## 0.15.2

- Improve S.M.A.R.T. device detection logic (fix regression in 0.15.1) (#1345)

## 0.15.1

- Add `SMART_DEVICES` environment variable to specify devices and types. (#373, #1335)

- Add support for `scsi`, `sntasmedia`, and `sntrealtek` S.M.A.R.T. types. (#373, #1335)

- Handle power-on time attributes that are formatted as strings (e.g., "0h+0m+0.000s").

- Skip virtual disks in S.M.A.R.T. monitoring. (#1332)

- Add sorting to the S.M.A.R.T. table. (#1333)

- Fix incorrect disk rendering in S.M.A.R.T. device details. (#1336)

- Fix `SHARE_ALL_SYSTEMS` setting not working for containers. (#1334)

- Fix text contrast issue when container details are disabled. (#1324)

## 0.15.0

- Add initial S.M.A.R.T. support for disk health monitoring. (#962)

- Add `henrygd/beszel-agent:alpine` Docker image and include `smartmontools` in all non-base agent images.

- Remove environment variables from container details (#1305)

- Add `CONTAINER_DETAILS` environment variable to control access to container logs and info APIs. (#1305)

- Improve temperature chart by allowing y-axis to start above 0 for better readability. (#1307)

- Improve battery detection logic. (#1287)

- Limit docker log size to prevent possible memory leak. (#1322)

- Update Go dependencies.

## 0.14.1

- Add `MFA_OTP` environment variable to enable email-based one-time password for users and/or superusers.

- Add image name to containers table. (#1302)

- Add spacing for long temperature chart tooltip. (#1299)

- Fix sorting by status in containers table. (#1294)

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
