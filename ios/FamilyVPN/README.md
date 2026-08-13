# Family VPN (iOS)

## Подпись (уже лежит в `signing/`)

| Файл | Назначение |
|------|------------|
| `cert.p12` / `cert-legacy.p12` | сертификат (пароль в `password.txt`) |
| `profile.mobileprovision` | профиль `ded9b` |

### Что в профиле

- **App ID:** `com.Xdd.48f160f` (не wildcard)
- **Team:** `9363A386MV`
- **Network Extension:** есть (`packet-tunnel-provider`) ✅
- **App Groups:** пусто (конфиг передаём через `providerConfiguration`)
- **Устройство:** один UDID (`00008120-…`)
- **Срок:** до 2027-02-10

Проект уже выставлен на `com.Xdd.48f160f`, туннель: `com.Xdd.48f160f.tunnel`.

⚠️ Для extension нужен либо второй `.mobileprovision` на `com.Xdd.48f160f.tunnel`, либо wildcard-профиль. ESign иногда подписывает оба target одним сертификатом — проверим при сборке.

## После Xcode

```bash
sudo xcode-select -s /Applications/Xcode.app/Contents/Developer
brew install xcodegen go

cd ios/FamilyVPN
xcodegen generate
open FamilyVPN.xcodeproj
```

Дальше — Archive / IPA → подпись в ESign.
