# Family VPN (Android)

Своё приложение под ваш AmneziaWG-сервер — **без Amnezia**.

## Что умеет
- Вкл / Выкл туннель (AmneziaWG userspace, `amneziawg-android`)
- Импорт `.conf` (вставка текста или выбор файла)
- Кнопка поддержки → https://t.me/ecstasyssssssss
- Кнопка личного кабинета / оплаты

## Как собрать

1. Установите [Android Studio](https://developer.android.com/studio)
2. Open → папка `android/FamilyVPN`
3. Дождитесь Gradle Sync
4. Подключите телефон (USB debugging) или эмулятор
5. Run ▶

APK после сборки:
`app/build/outputs/apk/debug/app-debug.apk`

## Как пользоваться

1. Возьмите smart-конфиг, например `configs/smart/my-phone.conf`
2. В приложении: **Файл** или вставьте текст → **Сохранить**
3. **Включить** → разрешите VPN системе
4. Проверьте, что Telegram/YouTube идут через VPS

## Дальше
- Логин в кабинет и автозагрузка конфига после оплаты
- Статус подписки на главном экране
- Quick Settings tile
