# Beszel for Android

A native Jetpack Compose client for a self-hosted Beszel hub, built around a
"mission control" design system: a dark telemetry console (with a precise
light theme), per-metric color coding, monospaced tabular data typography
(Space Grotesk + JetBrains Mono), and a live fleet pulse header.

## Run

1. Open this `android` directory in Android Studio.
2. Let Gradle sync, then run the `app` configuration on Android 8.0 or newer.
3. Enter the full URL of your Beszel hub and sign in with a Beszel user account.

LAN HTTP hubs are supported for local setups, but HTTPS is recommended whenever the hub is reachable outside a trusted network.

## Included

- Password authentication against PocketBase/Beszel, session restore and auth refresh
- Fleet screen with a live status header (fleet pulse sparkline), search, filters, and sorting
- Pull-to-refresh plus a 15-second auto-refresh poll
- System cards with hue-coded CPU / memory / disk meters and status glow dots
- System detail: metric tiles (including network in/out), temperature / GPU / battery chips, and interactive history charts with drag-to-scrub tooltips for CPU, memory, disk, and network (dual series)
- Active alerts and a resolved-history timeline with relative timestamps
- Dark / light / system themes; optional Material You dynamic color (metric and status colors stay brand-fixed either way)
- Responsive phone (bottom bar) and tablet (navigation rail) layouts
- Reduced-motion support, tabular figures for live values, and screen-reader labels

## Design system

- `ui/theme/Color.kt` — brand color schemes plus the semantic metric palette (CPU amber, memory violet, disk sky, network teal, temperature coral, GPU fuchsia, battery green) exposed via `LocalMetricColors`
- `ui/theme/Type.kt` — Space Grotesk (UI) and JetBrains Mono (data) via downloadable Google Fonts with automatic system fallback
- `ui/charts/` — Canvas chart engine: sparklines and a full line chart with axes, gradient fill, draw-in animation, min/max markers, and scrub tooltips
- `ui/components/` — shared pieces: status dots, segmented controls, shimmer skeletons, animated numbers, fleet pulse header, system cards
