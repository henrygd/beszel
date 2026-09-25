# Connected Wi-Fi signal

Reports connected station interfaces only (no scans). `info.wf` holds the current
snapshot keyed by interface (`s` SSID, `r` RSSI in dBm when available);
`stats.wf` stores available RSSI as integer dBm. Collected on the default
interval only; real-time requests reuse the last snapshot.

- Linux: nl80211 via `github.com/mdlayher/wifi`. Docker needs `network_mode: host`.
- macOS: CoreWLAN via `osascript` (JXA). SSID may be redacted by privacy settings.
- Windows: native WLAN API, keyed by interface GUID.
- Other platforms: unsupported.
