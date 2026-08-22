# Beszel for Android

A native Material 3 client for a self-hosted Beszel hub.

## Run

1. Open this `android` directory in Android Studio.
2. Let Gradle sync, then run the `app` configuration on Android 8.0 or newer.
3. Enter the full URL of your Beszel hub and sign in with a Beszel user account.

LAN HTTP hubs are supported for local setups, but HTTPS is recommended whenever the hub is reachable outside a trusted network.

## Included

- Password authentication against PocketBase/Beszel
- Session restoration and auth refresh
- Auto-refreshing system overview and active alert summary
- Responsive phone/tablet navigation
- System detail metrics and historical charts
- Alert history
- Dynamic color, light/dark/system themes
