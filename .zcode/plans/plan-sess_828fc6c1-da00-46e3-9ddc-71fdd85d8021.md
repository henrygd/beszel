# Beszel Android — Complete UI/UX Overhaul

You chose "All" design directions + all UX improvements + keep the dynamic-color toggle. Since one app can't literally be three styles, I'll synthesize them into **one identity with two fully-designed themes**:

- **Core personality: "Mission Control"** — a dark-first telemetry console. Deep blue-slate surfaces (not flat black), per-metric accent hues, monospaced numerals, sparkline-first cards.
- **Light theme: "Homelab OS" daylight precision** — cool paper background, ink-navy + brand violet, hairline discipline, same component geometry.
- **From "Soft Glass": the geometry and warmth** — generous radii (20dp cards), soft glows on status dots, calm spring motion, approachable spacing.

Nothing from the current UI survives untouched — every screen, component, theme token, and the navigation shell is rebuilt.

## Design system (new `ui/theme/`)

**Color** (`Color.kt` — full dark + light M3 schemes, both hand-tuned):
- Brand anchor: Beszel's brand gradient violet `#747BFF` → mint `#24EB5C` becomes the accent system. Primary: brand violet (dark `#9BA1FF`, light `#4F46E5`); success/healthy mint.
- Dark ramp: canvas `#0A0F1A`, surfaces `#0E1626`→`#1A2740`, hairline outlines `#1E2C44`. Elevation via surface tint + 1dp outline, not shadows.
- Light ramp: cool paper `#F6F8FB`, white cards, ink text, navy-violet accents.
- **Signature semantic system — per-metric hue coding**, identical in both themes (dark-mode variants tuned for contrast): CPU amber, Memory violet, Disk sky, Network teal, Temperature coral, GPU fuchsia, Battery green. Status: healthy/warning/critical/paused. Exposed to composables via a `LocalMetricColors` CompositionLocal so wallpaper dynamic color (kept as Settings toggle, now default OFF) can restyle M3 roles but **never** the semantic hues.

**Typography** (`Type.kt`) — via `androidx.compose.ui:ui-text-google-fonts` (only new dependency; falls back to system fonts offline):
- **Space Grotesk** for display/titles/UI — technical character without gimmick.
- **JetBrains Mono** for all metric values, timestamps, host info — tabular figures everywhere data lives (prevents layout shift as numbers tick).

**Shape / Motion** (`Shape.kt`, `Motion.kt`): 20dp cards, 28dp hero, pill chips; spring tokens (150–300ms micro, ~400ms chart draw-in), 30–50ms staggered list entrance, shimmer skeleton brush. Decorative animation gated on system animator-duration-scale (reduced-motion respect).

**Strings**: every user-facing string moves from composables into `strings.xml`.

## Architecture changes

1. **Real navigation** — replace the hand-rolled enum + Crossfade + detail-overlay hack with `navigation-compose` (already in dependencies, currently unused): `fleet` (start), `alerts`, `settings` top-level with the existing bottom-bar/rail adaptive shell; `system/{id}` pushed on top with directional slide/fade transitions and proper back stack.
2. **ViewModel split** — extract thin `HubRepository` (wraps `BeszelApi`, holds current session/client). `AppViewModel` keeps session, 15s polling, fleet data, theme prefs, snackbar. New nav-scoped `SystemDetailViewModel` owns chart range/metric/stats loading. Data layer (`BeszelApi`, `Models`, `SessionStore`) otherwise unchanged.
3. **Fleet pulse data** — `AppViewModel` keeps a small ring buffer of recent fleet-aggregate CPU samples from each poll to feed the new live Fleet Pulse header.

## New component + chart library (`ui/components/`, `ui/charts/`)

- `FleetPulseHeader` — **the signature element**: fleet status word (NOMINAL/DEGRADED/CRITICAL), up/down/alerting counts, live sparkline of fleet CPU with brand-gradient glow status dot.
- `SystemCard` — glow status dot + name/host, uptime, hue-coded CPU/MEM/DISK bars with mono values, CPU sparkline, alert chip.
- `StatusDot`, `AnimatedNumber`, `ShimmerBox`/skeletons, `EmptyState`/`ErrorState` (with recovery actions), segmented control, search bar.
- **Real chart system** replacing the bare 75-line Canvas chart: `LineChart` with y-axis % labels, time x-labels, subtle gridlines, gradient fill, draw-in animation, min/max markers, **drag-scrub crosshair + tooltip (value + timestamp)**, multi-series mode; `Sparkline` axisless variant; `NetworkChart` = dual-series up/down with legend (data already exists in `StatPoint`, unused today). Chart a11y summaries via contentDescription; skeletons + error-with-retry states.

## Screen rebuilds

- **Login**: full-bleed canvas with subtle brand radial glow, floating card, visible field labels + autofill hints, styled HTTP warning, loading-state CTA, error with recovery.
- **Fleet**: Fleet Pulse header → toolbar (expanding search, filter chips All/Alerting/Down, sort menu by name/status/CPU) → responsive grid of SystemCards → pull-to-refresh (M3 `PullToRefreshBox`, haptic on trigger) + existing 15s auto-refresh; shimmer on first load; empty state pointing to the hub.
- **System Detail**: shared-element-style transition from card; status header surfacing currently-hidden data (temperature, GPU, battery); 4 hue-accented metric tiles; segmented range (1h/12h/24h/7d) + metric tabs (CPU/MEM/DISK/**NET**) over the new rich chart; mono host-info card.
- **Alerts**: severity-ruled active alert rows (icon + text, not color-only); history as a timeline with relative timestamps; distinct empty states.
- **Settings**: grouped cards — Connection (with "Open hub"), Appearance (segmented theme + dynamic-color toggle), About/version, and Sign-out visually separated with a confirm dialog.

## Quality floor (verified per theme)

4.5:1 text contrast pairs; ≥48dp touch targets; contentDescriptions on all icon buttons; status never color-only; sp-based text with no fixed-height text containers (dynamic type safe); charts get text summaries.

## Verification

`gradlew.bat :app:assembleDebug` + `:app:testDebugUnitTest` (add unit tests for fleet-pulse aggregation, search/filter/sort logic, formatters) + `:app:lintDebug`. No emulator assumed — if you want visual QA I can walk through screenshots afterward.

**Out of scope**: widgets, push notifications, container views, WebSocket streaming, multi-account, non-English localization.

**Risks**: downloadable Google Fonts need a GMS cert resource and gracefully fall back to system fonts offline; shared-element transitions are the most experimental piece and will be dropped to scale+fade if they fight the nav transitions.