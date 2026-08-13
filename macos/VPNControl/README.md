# VPN Control (macOS)

Простая нативная утилита (SwiftUI) для наблюдения за VPS и AmneziaWG.

## Открыть в Xcode

1. Xcode → File → New → Project → App (macOS, SwiftUI)
2. Назовите проект `VPNControl`
3. Замените сгенерированные файлы содержимым из папки `VPNControl/`
4. Run (⌘R)

## Данные со статусом

На VPS должен работать `vpn-statusd`. С Mac пробросьте порт:

```bash
ssh -L 8090:127.0.0.1:8090 root@138.249.141.164
export STATUS_TOKEN='…из /opt/vpn/monitor.env…'
export VPN_STATUS_URL=http://127.0.0.1:8090
```

Потом запускайте приложение из Xcode.
