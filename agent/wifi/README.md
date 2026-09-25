# Connected Wi-Fi signal

Each default-interval poll reports a snapshot of connected station interfaces
in `info.wifi`. Map keys identify interfaces, not networks. `s` is optional SSID
metadata; `r` is nullable native RSSI in dBm. Quality percentages are never
converted to dBm. An associated interface without an accessible RSSI still
appears with an unavailable signal. No scans or network changes occur.
`stats.wf` stores only available RSSI values (integer dBm) keyed by interface.
Real-time requests reuse the last snapshot instead of collecting again.

The hub panel gates exclusively on current `systems.info.wifi` and system `up`
status, independently of the selected historical period. Empty/null snapshots
clear it. Historical averages use only available readings per interface; gaps
are not zero signal. Interface colors and keys remain stable on reconnect.

## Platforms

- Linux: native nl80211 through `github.com/mdlayher/wifi`, compiled into the
  static agent, including scratch and all other agent images. No `iw`, shared
  libraries, extra capabilities, or external helper required. Host network
  namespace access (Docker `network_mode: host`) is necessary to see host Wi-Fi.
  Only managed station interfaces with explicit associated BSS status appear.
  Kernel BSS cache reads do not trigger scans; station statistics supply native
  RSSI matched to the associated AP. Denied/missing station statistics retain
  association with unavailable RSSI, never substitute stale scan-cache signal.
  Missing nl80211/driver support or denied association reads yield no readings.
  A single two-second socket deadline bounds enumeration and interface queries
  after opening the client. The library's initial nl80211 family discovery is
  synchronous and does not expose a deadline.
- macOS: system `osascript` uses public CoreWLAN via JXA. Station mode proves
  association; RSSI and optional SSID are read independently. No private airport
  binary, elevated command or compiled helper required. Privacy settings may
  redact SSIDs. The subprocess has a two-second deadline.
- Windows: native WLAN API, interface GUID identity, connected interface state,
  optional current-connection SSID and native RSSI query. No localized `netsh`
  parsing. Missing WLAN service/API yields no readings; denied SSID/RSSI query
  leaves a connected interface with missing metadata/signal. Non-UTF-8 raw
  SSIDs are omitted so they cannot invalidate CBOR text in the agent response.
  Native synchronous WLAN calls cannot be interrupted by the Go deadline.
- FreeBSD and other platforms: unsupported, empty snapshot. No approximation
  from ifconfig quality and no stale data retained.

Collectors retry each default-interval poll, allowing interfaces and capabilities to appear
without an agent restart. Standard agent response caching still applies. Existing
hub record JSON storage requires no database schema migration. Older agents
without the field keep the panel hidden. Native macOS/Windows runtime checks and
real adapter testing are still required; cross compilation is not hardware proof.
