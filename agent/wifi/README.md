# Connected Wi-Fi signal

Reports connected station interfaces (no scans), plus OpenWrt access points on
Linux. `info.wf` holds the current snapshot keyed by interface (`s` SSID, `r`
RSSI in dBm when available, `a` set for an access point, whose `r` is the
average RSSI of associated clients); `stats.wf` stores available RSSI as integer
dBm. The systems table shows the strongest station, falling back to the
strongest access point. Collected on the default
interval only; real-time requests reuse the last snapshot. Set `SKIP_WIFI=true`
on the agent to disable collection.

- Linux: nl80211 via `github.com/mdlayher/wifi`. Docker needs `network_mode: host`.
  nl80211 is skipped when `/sys/class/ieee80211` lists no wireless devices.
  Access points use `iwinfo` (OpenWrt): only the APs nl80211 lists are queried,
  or every interface from bare `iwinfo` when nl80211 has no usable inventory.
- macOS: CoreWLAN via `osascript` (JXA). SSID may be redacted by privacy settings.
- Windows: native WLAN API, keyed by adapter alias (e.g. `Wi-Fi`), falling back
  to interface GUID.
- Other platforms: unsupported.
