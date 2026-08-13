// Command vpnbot is the family VPN Telegram bot: 140 RUB/month via Crypto Pay (USDT) or SBP.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"vpn-server/pkg/cryptopay"
	"vpn-server/pkg/db"
	"vpn-server/pkg/tgbot"
)

func main() {
	loadEnvFile(envOr("VPBOT_ENV_FILE", "/opt/vpn/vpnbot.env"))

	dbPath := flag.String("db", envOr("VPBOT_DB", "vpn.db"), "path to vpn.db")
	listen := flag.String("listen", envOr("VPBOT_ADDR", "127.0.0.1:8091"), "webhook listen address")
	flag.Parse()

	tgToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	cpToken := os.Getenv("CRYPTO_PAY_TOKEN")
	if tgToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required (create a bot via @BotFather)")
	}

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	svc := &tgbot.Service{
		DB: database,
		CP: &cryptopay.Client{Token: cpToken},
		Config: tgbot.Config{
			DatabasePath:   *dbPath,
			SyncCommand:    os.Getenv("VPBOT_SYNC_CMD"),
			ListenAddr:     *listen,
			WebhookPath:    envOr("CRYPTO_PAY_WEBHOOK_PATH", "/webhook/cryptopay"),
			SBPPhone:       strings.TrimSpace(os.Getenv("SBP_PHONE")),
			SBPBank:        strings.TrimSpace(os.Getenv("SBP_BANK")),
			SBPName:        strings.TrimSpace(os.Getenv("SBP_NAME")),
			AdminTelegram:  parseAdminIDs(os.Getenv("ADMIN_TELEGRAM_IDS")),
			SupportURL:     envOr("SUPPORT_URL", "https://t.me/ecstasyssssssss"),
			AndroidAPKURL:  strings.TrimSpace(os.Getenv("ANDROID_APK_URL")),
			AndroidAPKPath: strings.TrimSpace(os.Getenv("ANDROID_APK_PATH")),
		},
	}

	bot := &tgbot.Bot{Token: tgToken}
	if name, err := bot.Username(); err == nil && name != "" {
		svc.Config.BotUsername = name
		log.Printf("telegram bot @%s", name)
	}
	if err := bot.ApplyBranding(); err != nil {
		log.Printf("branding: %v", err)
	} else if n, u, err := bot.GetMeName(); err == nil {
		log.Printf("branded as %q (@%s)", n, u)
	}
	if photo := strings.TrimSpace(os.Getenv("BOT_AVATAR_PATH")); photo != "" {
		if err := bot.SetProfilePhoto(photo); err != nil {
			log.Printf("avatar: %v", err)
		} else {
			log.Printf("avatar updated from %s", photo)
		}
	}

	if cpToken != "" {
		if _, err := svc.CP.GetMe(); err != nil {
			log.Fatalf("cryptopay getMe: %v", err)
		}
		log.Printf("cryptopay connected, price %s RUB in %s", tgbot.PriceRUB, tgbot.InvoiceAsset)
	} else {
		log.Printf("cryptopay disabled (CRYPTO_PAY_TOKEN empty)")
	}
	if svc.Config.SBPEnabled() {
		log.Printf("sbp enabled → %s / %s", svc.Config.SBPPhone, svc.Config.SBPBank)
	} else {
		log.Printf("sbp disabled (set SBP_PHONE and SBP_BANK)")
	}
	if len(svc.Config.AdminTelegram) == 0 {
		log.Printf("warning: ADMIN_TELEGRAM_IDS empty — SBP confirmations will have nowhere to go")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if cpToken != "" {
		mux.HandleFunc("POST "+svc.Config.WebhookPath, handleCryptoWebhook(svc, cpToken))
	}

	go func() {
		log.Printf("http on http://%s%s", *listen, svc.Config.WebhookPath)
		if err := http.ListenAndServe(*listen, mux); err != nil {
			log.Fatalf("webhook server: %v", err)
		}
	}()

	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			if err := svc.RunMaintenance(bot.SendMenu); err != nil {
				log.Printf("maintenance: %v", err)
			}
			<-t.C
		}
	}()

	log.Printf("polling telegram updates…")
	for {
		updates, err := bot.PollUpdates()
		if err != nil {
			log.Printf("getUpdates: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, update := range updates {
			if err := handleUpdate(svc, bot, update); err != nil {
				log.Printf("update: %v", err)
			}
		}
	}
}

func handleUpdate(svc *tgbot.Service, bot *tgbot.Bot, update tgbot.Update) error {
	if update.CallbackQuery != nil {
		return handleCallback(svc, bot, update.CallbackQuery)
	}
	if update.Message == nil || update.Message.From.ID == 0 {
		return nil
	}
	msg := update.Message
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return nil
	}

	member, created, err := svc.EnsureMember(msg.From.ID, displayName(msg.From), msg.From.Username)
	if err != nil {
		return bot.SendText(msg.Chat.ID, "Ошибка регистрации: "+err.Error())
	}
	admin := svc.Config.IsAdmin(msg.From.ID)
	if created {
		_ = bot.SendMenuWith(msg.Chat.ID, welcomeText(svc, member), admin)
		return nil
	}

	switch {
	case strings.HasPrefix(text, "/start"):
		return bot.SendMenuWith(msg.Chat.ID, welcomeText(svc, member), admin)
	case strings.HasPrefix(text, "/help"), text == "Помощь", text == "❓ Помощь":
		return bot.SendMenuWith(msg.Chat.ID, helpText(svc, member), admin)
	case strings.HasPrefix(text, "/status"), text == "Статус", text == "📊 Статус":
		return bot.SendMenuWith(msg.Chat.ID, statusMessage(member), admin)
	case strings.HasPrefix(text, "/pay"), text == "Оплатить", text == "💳 Оплатить":
		return sendPayChoice(svc, bot, msg.Chat.ID)
	case strings.HasPrefix(text, "/config"), text == "Мой конфиг", text == "📄 Мой конфиг":
		return sendConfig(svc, bot, member, msg.Chat.ID)
	case strings.HasPrefix(text, "/android"), text == "Android", text == "🤖 Android":
		return sendAndroid(svc, bot, msg.Chat.ID)
	case strings.HasPrefix(text, "/support"), text == "Поддержка", text == "💬 Поддержка":
		return sendSupport(svc, bot, msg.Chat.ID)
	case strings.HasPrefix(text, "/slots"), strings.HasPrefix(text, "/manager"),
		text == "Менеджер", text == "Слоты", text == "📋 Слоты":
		return sendManager(svc, bot, msg.From.ID, msg.Chat.ID)
	case strings.HasPrefix(text, "/give"), text == "Выдать конфиг", text == "🎁 Выдать свободный":
		return giveFreeConfig(svc, bot, msg.From.ID, msg.Chat.ID)
	case strings.HasPrefix(text, "/confirm "):
		return adminConfirmCmd(svc, bot, msg.From.ID, msg.Chat.ID, strings.TrimSpace(strings.TrimPrefix(text, "/confirm")))
	case strings.HasPrefix(text, "/myid"):
		return bot.SendText(msg.Chat.ID, fmt.Sprintf("Ваш Telegram ID: <code>%d</code>", msg.From.ID))
	default:
		return bot.SendMenuWith(msg.Chat.ID,
			"<b>"+tgbot.BrandName+"</b>\nВыберите действие в меню ниже.", admin)
	}
}

func handleCallback(svc *tgbot.Service, bot *tgbot.Bot, cb *tgbot.Callback) error {
	data := cb.Data

	switch {
	case data == "pay_usdt":
		_ = bot.AnswerCallback(cb.ID, "")
		member, _, err := svc.EnsureMember(cb.From.ID, displayName(cb.From), cb.From.Username)
		if err != nil {
			return bot.SendText(cb.Msg.Chat.ID, "Ошибка: "+err.Error())
		}
		return sendUSDTInvoice(svc, bot, member, cb.Msg.Chat.ID)
	case data == "pay_sbp":
		_ = bot.AnswerCallback(cb.ID, "")
		member, _, err := svc.EnsureMember(cb.From.ID, displayName(cb.From), cb.From.Username)
		if err != nil {
			return bot.SendText(cb.Msg.Chat.ID, "Ошибка: "+err.Error())
		}
		return sendSBPInvoice(svc, bot, member, cb.Msg.Chat.ID)
	case data == "config":
		_ = bot.AnswerCallback(cb.ID, "")
		member, _, err := svc.EnsureMember(cb.From.ID, displayName(cb.From), cb.From.Username)
		if err != nil {
			return bot.SendText(cb.Msg.Chat.ID, "Ошибка: "+err.Error())
		}
		return sendConfig(svc, bot, member, cb.Msg.Chat.ID)
	case data == "pay":
		_ = bot.AnswerCallback(cb.ID, "")
		return sendPayChoice(svc, bot, cb.Msg.Chat.ID)
	case strings.HasPrefix(data, "sbp_paid:"):
		code := strings.TrimPrefix(data, "sbp_paid:")
		return handleSBPPaid(svc, bot, cb, code)
	case strings.HasPrefix(data, "sbp_ok:"):
		code := strings.TrimPrefix(data, "sbp_ok:")
		return handleSBPAdminOK(svc, bot, cb, code)
	case strings.HasPrefix(data, "sbp_no:"):
		code := strings.TrimPrefix(data, "sbp_no:")
		return handleSBPAdminNo(svc, bot, cb, code)
	case data == "mgr_refresh":
		return refreshManager(svc, bot, cb)
	case strings.HasPrefix(data, "peer_on:"):
		return togglePeer(svc, bot, cb, strings.TrimPrefix(data, "peer_on:"), true)
	case strings.HasPrefix(data, "peer_off:"):
		return togglePeer(svc, bot, cb, strings.TrimPrefix(data, "peer_off:"), false)
	case strings.HasPrefix(data, "peer_info:"):
		return peerInfo(svc, bot, cb, strings.TrimPrefix(data, "peer_info:"))
	default:
		_ = bot.AnswerCallback(cb.ID, "")
		return nil
	}
}

func sendPayChoice(svc *tgbot.Service, bot *tgbot.Bot, chatID int64) error {
	var rows [][]map[string]any
	if svc.CP != nil && svc.CP.Token != "" {
		rows = append(rows, []map[string]any{{"text": "USDT · CryptoBot", "callback_data": "pay_usdt"}})
	}
	if svc.Config.SBPEnabled() {
		rows = append(rows, []map[string]any{{"text": "СБП · перевод на карту", "callback_data": "pay_sbp"}})
	}
	if len(rows) == 0 {
		return bot.SendText(chatID, "Оплата пока не настроена.")
	}
	text := fmt.Sprintf(
		"<b>%s</b>\n────────────\nПодписка на месяц: <b>%s ₽</b>\n\nВыберите удобный способ оплаты:",
		tgbot.BrandName, tgbot.PriceRubHuman,
	)
	return bot.SendInline(chatID, text, rows)
}

func sendUSDTInvoice(svc *tgbot.Service, bot *tgbot.Bot, member *db.Member, chatID int64) error {
	if svc.CP == nil || svc.CP.Token == "" {
		return bot.SendText(chatID, "USDT сейчас недоступен")
	}
	invoice, err := svc.StartCheckout(member)
	if err != nil {
		return bot.SendText(chatID, "Не удалось создать счёт: "+err.Error())
	}
	url := invoice.BotInvoiceURL
	if url == "" {
		url = invoice.MiniAppURL
	}
	if url == "" {
		url = invoice.WebAppURL
	}
	if url == "" {
		return bot.SendText(chatID, "CryptoBot не вернул ссылку на оплату")
	}
	text := fmt.Sprintf(
		"<b>%s · оплата USDT</b>\n────────────\n"+
			"Сумма: <b>%s ₽</b>\nОплата через @CryptoBot (кошелёк заранее не нужен).\n\n"+
			"После оплаты откройте «Мой конфиг».",
		tgbot.BrandName, tgbot.PriceRubHuman,
	)
	return bot.SendPayButton(chatID, text, url)
}

func sendSBPInvoice(svc *tgbot.Service, bot *tgbot.Bot, member *db.Member, chatID int64) error {
	checkout, err := svc.StartSBPCheckout(member)
	if err != nil {
		return bot.SendText(chatID, err.Error())
	}
	text := svc.SBPInstructions(checkout.Code)
	return bot.SendInline(chatID, text, [][]map[string]any{
		{{"text": "Я оплатил", "callback_data": "sbp_paid:" + checkout.Code}},
		{{"text": "Мой конфиг", "callback_data": "config"}},
	})
}

func handleSBPPaid(svc *tgbot.Service, bot *tgbot.Bot, cb *tgbot.Callback, code string) error {
	payment, member, err := svc.MarkSBPReported(code)
	if err != nil {
		_ = bot.AnswerCallback(cb.ID, "Счёт не найден")
		return bot.SendText(cb.Msg.Chat.ID, err.Error())
	}
	if payment.Status == "paid" {
		_ = bot.AnswerCallback(cb.ID, "Уже оплачено")
		return bot.SendMenu(cb.Msg.Chat.ID, "✅ Этот счёт уже подтверждён.\n"+tgbot.StatusText(member))
	}
	_ = bot.AnswerCallback(cb.ID, "Отправила админу")

	_ = bot.SendText(cb.Msg.Chat.ID,
		fmt.Sprintf("Принято. Жду подтверждения перевода <code>VPN-%s</code>. Обычно это быстро.", code))

	uname := cb.From.Username
	if uname != "" {
		uname = "@" + uname
	} else {
		uname = "—"
	}
	adminText := fmt.Sprintf(
		"🏦 <b>СБП ожидает подтверждения</b>\n\n"+
			"Код: <code>VPN-%s</code>\n"+
			"Сумма: <b>%s ₽</b>\n"+
			"От: <b>%s</b> (%s)\n"+
			"Peer: <code>%s</code>\n"+
			"TG id: <code>%d</code>\n\n"+
			"Проверь перевод в банке и нажми кнопку.",
		code, tgbot.PriceRubHuman, escapeHTML(member.DisplayName), uname, member.PeerName, cb.From.ID,
	)
	kb := [][]map[string]any{
		{
			{"text": "Деньги пришли", "callback_data": "sbp_ok:" + code},
			{"text": "Нет перевода", "callback_data": "sbp_no:" + code},
		},
	}
	for _, adminID := range svc.Config.AdminTelegram {
		if err := bot.SendInline(adminID, adminText, kb); err != nil {
			log.Printf("notify admin %d: %v", adminID, err)
		}
	}
	if len(svc.Config.AdminTelegram) == 0 {
		log.Printf("SBP reported VPN-%s but ADMIN_TELEGRAM_IDS is empty", code)
	}
	return nil
}

func handleSBPAdminOK(svc *tgbot.Service, bot *tgbot.Bot, cb *tgbot.Callback, code string) error {
	if !svc.Config.IsAdmin(cb.From.ID) {
		_ = bot.AnswerCallback(cb.ID, "Нет доступа")
		return nil
	}
	member, err := svc.ConfirmSBPPayment(code)
	if err != nil {
		_ = bot.AnswerCallback(cb.ID, "Ошибка")
		return bot.SendText(cb.Msg.Chat.ID, err.Error())
	}
	_ = bot.AnswerCallback(cb.ID, "Подтверждено")
	_ = bot.EditInline(cb.Msg.Chat.ID, cb.Msg.MessageID,
		fmt.Sprintf("✅ СБП <code>VPN-%s</code> подтверждён.\n%s → активна до %s",
			code, escapeHTML(member.DisplayName), member.PaidUntil), nil)

	if tgID, err := svc.DB.MemberTelegramUserID(member.ID); err == nil {
		text := fmt.Sprintf("<b>%s</b>\n────────────\nОплата по СБП получена.\n%s\n\nОткройте «Мой конфиг».", tgbot.BrandName, tgbot.StatusText(member))
		_ = bot.SendMenu(tgID, text)
	}
	return nil
}

func handleSBPAdminNo(svc *tgbot.Service, bot *tgbot.Bot, cb *tgbot.Callback, code string) error {
	if !svc.Config.IsAdmin(cb.From.ID) {
		_ = bot.AnswerCallback(cb.ID, "Нет доступа")
		return nil
	}
	if err := svc.RejectSBPPayment(code); err != nil {
		_ = bot.AnswerCallback(cb.ID, "Ошибка")
		return bot.SendText(cb.Msg.Chat.ID, err.Error())
	}
	_ = bot.AnswerCallback(cb.ID, "Отклонено")
	_ = bot.EditInline(cb.Msg.Chat.ID, cb.Msg.MessageID,
		fmt.Sprintf("❌ СБП <code>VPN-%s</code> отклонён.", code), nil)

	if payment, err := svc.DB.GetPayment("sbp:" + strings.ToUpper(code)); err == nil {
		if tgID, err := svc.DB.MemberTelegramUserID(payment.MemberID); err == nil {
			_ = bot.SendText(tgID, fmt.Sprintf(
				"Перевод <code>VPN-%s</code> не найден. Проверьте сумму/комментарий и нажмите «Оплатить» снова.", code))
		}
	}
	return nil
}

func sendManager(svc *tgbot.Service, bot *tgbot.Bot, fromID, chatID int64) error {
	if !svc.Config.IsAdmin(fromID) {
		return bot.SendText(chatID, "Нет доступа")
	}
	inv, err := svc.BuildInventory()
	if err != nil {
		return bot.SendText(chatID, "Не удалось открыть менеджер: "+err.Error())
	}
	_ = bot.SendMenuWith(chatID, tgbot.FormatInventory(inv), true)
	return bot.SendInline(chatID, "Управление доступом:", tgbot.ManageKeyboard(inv))
}

func refreshManager(svc *tgbot.Service, bot *tgbot.Bot, cb *tgbot.Callback) error {
	if !svc.Config.IsAdmin(cb.From.ID) {
		_ = bot.AnswerCallback(cb.ID, "Нет доступа")
		return nil
	}
	inv, err := svc.BuildInventory()
	if err != nil {
		_ = bot.AnswerCallback(cb.ID, "Ошибка")
		return bot.SendText(cb.Msg.Chat.ID, err.Error())
	}
	_ = bot.AnswerCallback(cb.ID, "Обновлено")
	return bot.EditInline(cb.Msg.Chat.ID, cb.Msg.MessageID,
		tgbot.FormatInventory(inv)+"\n\nУправление доступом:", tgbot.ManageKeyboard(inv))
}

func togglePeer(svc *tgbot.Service, bot *tgbot.Bot, cb *tgbot.Callback, peerName string, enabled bool) error {
	if !svc.Config.IsAdmin(cb.From.ID) {
		_ = bot.AnswerCallback(cb.ID, "Нет доступа")
		return nil
	}
	peer, member, err := svc.SetPeerAccess(peerName, enabled)
	if err != nil {
		_ = bot.AnswerCallback(cb.ID, "Ошибка")
		return bot.SendText(cb.Msg.Chat.ID, err.Error())
	}
	state := "выключен"
	if enabled {
		state = "включён"
	}
	label := peer.Name
	if member != nil {
		label = memberTelegramLabelLocal(member)
		if tgID, err := svc.DB.MemberTelegramUserID(member.ID); err == nil {
			msg := fmt.Sprintf("<b>%s</b>\n────────────\nДоступ VPN %s администратором.", tgbot.BrandName, state)
			_ = bot.SendMenu(tgID, msg)
		}
	}
	toast := label + " → " + state
	if len(toast) > 190 {
		toast = toast[:190] + "…"
	}
	_ = bot.AnswerCallback(cb.ID, toast)

	inv, err := svc.BuildInventory()
	if err != nil {
		return nil
	}
	return bot.EditInline(cb.Msg.Chat.ID, cb.Msg.MessageID,
		tgbot.FormatInventory(inv)+"\n\nУправление доступом:", tgbot.ManageKeyboard(inv))
}

func peerInfo(svc *tgbot.Service, bot *tgbot.Bot, cb *tgbot.Callback, peerName string) error {
	if !svc.Config.IsAdmin(cb.From.ID) {
		_ = bot.AnswerCallback(cb.ID, "Нет доступа")
		return nil
	}
	peer, err := svc.DB.GetPeer(peerName)
	if err != nil {
		_ = bot.AnswerCallback(cb.ID, "Не найден")
		return nil
	}
	on := "выкл"
	if peer.Enabled {
		on = "вкл"
	}
	info := fmt.Sprintf("%s · %s · %s", peer.Name, peer.Address, on)
	if m, err := svc.DB.GetMemberByPeerName(peerName); err == nil {
		info = fmt.Sprintf("%s · %s · %s · %s", memberTelegramLabelLocal(m), peer.Name, peer.Address, on)
		if m.PaidUntil != "" {
			info += " · до " + m.PaidUntil
		}
	}
	// answerCallbackQuery text is capped at ~200 chars.
	if len(info) > 190 {
		info = info[:190] + "…"
	}
	_ = bot.AnswerCallback(cb.ID, info)
	return nil
}

func memberTelegramLabelLocal(m *db.Member) string {
	if m == nil {
		return "?"
	}
	if u := strings.TrimSpace(m.TelegramUsername); u != "" {
		return "@" + strings.TrimPrefix(u, "@")
	}
	if m.DisplayName != "" {
		return m.DisplayName
	}
	return m.PeerName
}

func giveFreeConfig(svc *tgbot.Service, bot *tgbot.Bot, fromID, chatID int64) error {
	if !svc.Config.IsAdmin(fromID) {
		return bot.SendText(chatID, "Нет доступа")
	}
	peer, err := svc.NextFreePeer()
	if err != nil {
		return bot.SendText(chatID, err.Error())
	}
	cfg, peer, err := svc.PeerConfig(peer.Name)
	if err != nil {
		return bot.SendText(chatID, err.Error())
	}
	_ = bot.SendText(chatID, fmt.Sprintf(
		"Свободный слот <code>%s</code> (%s).\nПерешли этот файл человеку. Один конфиг = одно устройство.",
		peer.Name, peer.Address))
	return bot.SendDocument(chatID, peer.Name+".conf", "Свободный VPN-конфиг", cfg)
}

func adminConfirmCmd(svc *tgbot.Service, bot *tgbot.Bot, fromID, chatID int64, code string) error {
	if !svc.Config.IsAdmin(fromID) {
		return bot.SendText(chatID, "Нет доступа")
	}
	code = strings.TrimPrefix(strings.ToUpper(code), "VPN-")
	member, err := svc.ConfirmSBPPayment(code)
	if err != nil {
		return bot.SendText(chatID, err.Error())
	}
	if tgID, err := svc.DB.MemberTelegramUserID(member.ID); err == nil {
		_ = bot.SendMenu(tgID, fmt.Sprintf("<b>%s</b>\n────────────\nОплата по СБП получена.\n%s\n\nОткройте «Мой конфиг».", tgbot.BrandName, tgbot.StatusText(member)))
	}
	return bot.SendText(chatID, fmt.Sprintf("✅ Подтверждено: %s до %s", member.DisplayName, member.PaidUntil))
}

func sendConfig(svc *tgbot.Service, bot *tgbot.Bot, member *db.Member, chatID int64) error {
	config, err := svc.ClientConfig(member)
	if err != nil {
		return bot.SendText(chatID, err.Error())
	}
	filename := member.PeerName + ".conf"
	caption := "Импортируйте в AmneziaVPN / FamilyVPN."
	return bot.SendDocument(chatID, filename, caption, config)
}

func sendAndroid(svc *tgbot.Service, bot *tgbot.Bot, chatID int64) error {
	path := strings.TrimSpace(svc.Config.AndroidAPKPath)
	url := strings.TrimSpace(svc.Config.AndroidAPKURL)
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			caption := tgbot.BrandName + " для Android\nУстановите APK → импортируйте конфиг из бота."
			if url != "" {
				caption += "\n" + url
			}
			if err := bot.SendDocumentFile(chatID, path, caption); err == nil {
				return nil
			} else {
				log.Printf("send apk file: %v", err)
			}
		}
	}
	if url == "" {
		return bot.SendText(chatID, "Android-приложение пока не загружено. Напишите в поддержку.")
	}
	return bot.SendInline(chatID,
		fmt.Sprintf("<b>%s · Android</b>\n────────────\nСкачайте приложение и импортируйте конфиг после оплаты.", tgbot.BrandName),
		[][]map[string]any{
			{{"text": "Скачать APK", "url": url}},
			{{"text": "Написать в поддержку", "url": svc.Config.SupportURL}},
		},
	)
}

func sendSupport(svc *tgbot.Service, bot *tgbot.Bot, chatID int64) error {
	url := strings.TrimSpace(svc.Config.SupportURL)
	if url == "" {
		url = "https://t.me/ecstasyssssssss"
	}
	return bot.SendInline(chatID,
		fmt.Sprintf("<b>%s · поддержка</b>\n────────────\nВопросы по оплате, конфигу или подключению — напишите сюда.", tgbot.BrandName),
		[][]map[string]any{
			{{"text": "Написать в поддержку", "url": url}},
		},
	)
}

func handleCryptoWebhook(svc *tgbot.Service, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		sig := r.Header.Get("crypto-pay-api-signature")
		if !cryptopay.VerifyWebhookSignature(token, raw, sig) {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}

		var update cryptopay.WebhookUpdate
		if err := json.Unmarshal(raw, &update); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if update.UpdateType != "invoice_paid" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err := svc.HandlePaidInvoice(update.Payload); err != nil {
			log.Printf("cryptopay webhook: %v", err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		stored, err := svc.DB.GetPayment(fmt.Sprintf("cp:%d", update.Payload.InvoiceID))
		if err == nil {
			member, err := svc.DB.GetMemberByID(stored.MemberID)
			if err == nil {
				if tgID, err := svc.DB.MemberTelegramUserID(member.ID); err == nil {
					bot := &tgbot.Bot{Token: os.Getenv("TELEGRAM_BOT_TOKEN")}
					text := fmt.Sprintf("<b>%s</b>\n────────────\nОплата получена.\n%s\n\nОткройте «Мой конфиг», чтобы скачать настройки.", tgbot.BrandName, tgbot.StatusText(member))
					_ = bot.SendMenu(tgID, text)
				}
			}
		}

		w.WriteHeader(http.StatusOK)
	}
}

func welcomeText(svc *tgbot.Service, member *db.Member) string {
	pay := []string{}
	if svc.CP != nil && svc.CP.Token != "" {
		pay = append(pay, "USDT")
	}
	if svc.Config.SBPEnabled() {
		pay = append(pay, "СБП")
	}
	payLine := strings.Join(pay, " · ")
	if payLine == "" {
		payLine = "скоро"
	}
	return fmt.Sprintf(
		"<b>%s</b>\n────────────\n"+
			"Привет, <b>%s</b>!\n\n"+
			"Семейный VPN без лишней возни.\n"+
			"• <b>%s ₽</b> в месяц\n"+
			"• Оплата: %s\n"+
			"• Конфиг и Android-приложение — прямо в боте\n\n"+
			"%s\n\n"+
			"Начните с кнопки <b>Оплатить</b> ↓",
		tgbot.BrandName, escapeHTML(member.DisplayName), tgbot.PriceRubHuman, payLine, tgbot.StatusText(member),
	)
}

func helpText(svc *tgbot.Service, member *db.Member) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s · справка</b>\n────────────\n", tgbot.BrandName)
	b.WriteString("<b>Оплата</b>\n")
	b.WriteString("1. Нажмите «Оплатить»\n")
	if svc.CP != nil && svc.CP.Token != "" {
		b.WriteString("2. <b>USDT</b> — счёт в @CryptoBot, зачисление автоматически\n")
	}
	if svc.Config.SBPEnabled() {
		b.WriteString("2. <b>СБП</b> — перевод с комментарием VPN-XXXX → «Я оплатил»\n")
	}
	b.WriteString("3. «Мой конфиг» — скачайте и импортируйте\n\n")
	b.WriteString("<b>Android</b>\n")
	b.WriteString("Кнопка «Android» — приложение FamilyVPN\n")
	if u := strings.TrimSpace(svc.Config.AndroidAPKURL); u != "" {
		b.WriteString(u + "\n")
	}
	b.WriteString("\n<b>Поддержка</b>\n")
	b.WriteString(svc.Config.SupportURL + "\n\n")
	b.WriteString("<b>Продление</b>\n")
	b.WriteString("Автосписания нет. За 3 дня до конца пришлю напоминание.\n\n")
	b.WriteString(tgbot.StatusText(member))
	return b.String()
}

func statusMessage(member *db.Member) string {
	return fmt.Sprintf(
		"<b>%s · статус</b>\n────────────\n"+
			"Имя: <b>%s</b>\n"+
			"Слот: <code>%s</code>\n\n"+
			"%s",
		tgbot.BrandName, escapeHTML(member.DisplayName), member.PeerName, tgbot.StatusText(member),
	)
}

func displayName(u tgbot.User) string {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	return strings.TrimSpace(name)
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func parseAdminIDs(raw string) []int64 {
	var out []int64
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadEnvFile(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, strings.TrimSpace(value))
	}
}
