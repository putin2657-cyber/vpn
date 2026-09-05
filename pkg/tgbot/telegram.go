package tgbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const telegramAPI = "https://api.telegram.org"

// Bot sends messages through the Telegram Bot API.
type Bot struct {
	Token  string
	HTTP   *http.Client
	Offset int64
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      Chat   `json:"chat"`
	From      User   `json:"from"`
}

type Callback struct {
	ID   string  `json:"id"`
	From User    `json:"from"`
	Data string  `json:"data"`
	Msg  Message `json:"message"`
}

type Update struct {
	UpdateID      int64     `json:"update_id"`
	Message       *Message  `json:"message"`
	CallbackQuery *Callback `json:"callback_query"`
}

type tgResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

func (b *Bot) client() *http.Client {
	if b.HTTP != nil {
		return b.HTTP
	}
	// VPS has no public IPv6; prefer IPv4 so api.telegram.org does not hang on AAAA.
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Timeout: 70 * time.Second, Transport: transport}
}

// PollUpdates long-polls Telegram for new messages.
func (b *Bot) PollUpdates() ([]Update, error) {
	q := fmt.Sprintf("offset=%d&timeout=50", b.Offset)
	var updates []Update
	if err := b.get("getUpdates", q, &updates); err != nil {
		return nil, err
	}
	for _, u := range updates {
		if u.UpdateID >= b.Offset {
			b.Offset = u.UpdateID + 1
		}
	}
	return updates, nil
}

func (b *Bot) SendText(chatID int64, text string) error {
	body := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	return b.postJSON("sendMessage", body, nil)
}

func (b *Bot) SendMenu(chatID int64, text string) error {
	return b.SendMenuWith(chatID, text, false)
}

func (b *Bot) SendMenuWith(chatID int64, text string, admin bool) error {
	body := map[string]any{
		"chat_id":      chatID,
		"text":         text,
		"parse_mode":   "HTML",
		"reply_markup": mainKeyboard(admin),
	}
	return b.postJSON("sendMessage", body, nil)
}

func (b *Bot) SendPayButton(chatID int64, text, payURL string) error {
	return b.SendInline(chatID, text, [][]map[string]any{
		{{"text": "Оплатить в USDT", "url": payURL}},
		{{"text": "Мой конфиг", "callback_data": "config"}},
	})
}

// SendInline sends an HTML message with an inline keyboard.
func (b *Bot) SendInline(chatID int64, text string, keyboard [][]map[string]any) error {
	body := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
		"reply_markup": map[string]any{
			"inline_keyboard": keyboard,
		},
	}
	return b.postJSON("sendMessage", body, nil)
}

func (b *Bot) EditInline(chatID, messageID int64, text string, keyboard [][]map[string]any) error {
	body := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if keyboard != nil {
		body["reply_markup"] = map[string]any{"inline_keyboard": keyboard}
	}
	return b.postJSON("editMessageText", body, nil)
}

func (b *Bot) SendDocument(chatID int64, filename, caption, content string) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	if caption != "" {
		_ = w.WriteField("caption", caption)
	}
	part, err := w.CreateFormFile("document", filename)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(part, content); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, b.apiURL("sendDocument"), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return b.do(req, nil)
}

func (b *Bot) AnswerCallback(id, text string) error {
	body := map[string]any{"callback_query_id": id}
	if text != "" {
		body["text"] = text
	}
	return b.postJSON("answerCallbackQuery", body, nil)
}

func (b *Bot) Username() (string, error) {
	var me struct {
		Username string `json:"username"`
	}
	if err := b.get("getMe", "", &me); err != nil {
		return "", err
	}
	return me.Username, nil
}

func mainKeyboard(admin bool) map[string]any {
	rows := [][]map[string]string{
		{{"text": "Оплатить"}, {"text": "Статус"}},
		{{"text": "Мой конфиг"}, {"text": "Помощь"}},
		{{"text": "Android"}, {"text": "Поддержка"}},
	}
	if admin {
		rows = append(rows, []map[string]string{
			{"text": "Менеджер"}, {"text": "Выдать конфиг"},
		})
	}
	return map[string]any{
		"keyboard":          rows,
		"resize_keyboard":   true,
		"input_field_placeholder": "ecstasysVPN · выберите действие",
	}
}

// SendDocumentFile uploads a local file path as a Telegram document.
func (b *Bot) SendDocumentFile(chatID int64, path, caption string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	if caption != "" {
		_ = w.WriteField("caption", caption)
	}
	part, err := w.CreateFormFile("document", filepath.Base(path))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, b.apiURL("sendDocument"), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return b.do(req, nil)
}

func (b *Bot) apiURL(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", telegramAPI, b.Token, method)
}

func (b *Bot) get(method, query string, out any) error {
	url := b.apiURL(method)
	if query != "" {
		url += "?" + query
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return b.do(req, out)
}

func (b *Bot) postJSON(method string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, b.apiURL(method), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return b.do(req, out)
}

func (b *Bot) do(req *http.Request, out any) error {
	resp, err := b.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var envelope tgResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("telegram decode: %w", err)
	}
	if !envelope.OK {
		desc := strings.TrimSpace(envelope.Description)
		if desc == "" {
			desc = string(raw)
		}
		return fmt.Errorf("telegram: %s", desc)
	}
	if out != nil && len(envelope.Result) > 0 {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}
