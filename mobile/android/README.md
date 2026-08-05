# Android VPN Client

## Build

Open `mobile/android` in Android Studio, or:

```bash
cd mobile/android
gradle wrapper   # once, if no wrapper
./gradlew assembleDebug
adb install app/build/outputs/apk/debug/app-debug.apk
```

## Server address

| Environment | Server field |
|-------------|--------------|
| Emulator | `10.0.2.2:8443` |
| Physical device | `<your-mac-lan-ip>:8443` |

## Usage

1. Start `vpn-server` on your Mac
2. Create user: `./vpn-admin -action add -user phone -email phone@test.com`
3. Open app, enter server + token, tap Connect
4. Accept Android VPN permission dialog

## Permissions

The app uses `VpnService` and a foreground service notification while connected.
