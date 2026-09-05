package tgbot

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// BrandName is the public product name shown in the bot.
const BrandName = "ecstasysVPN"

// ApplyBranding sets the Telegram profile name, descriptions and command menu.
func (b *Bot) ApplyBranding() error {
	if err := b.postJSON("setMyName", map[string]any{"name": BrandName}, nil); err != nil {
		return fmt.Errorf("setMyName: %w", err)
	}
	if err := b.postJSON("setMyDescription", map[string]any{
		"description": "ecstasysVPN — личный VPN.\n" +
			"Оплата " + PriceRubHuman + " ₽/мес (USDT или СБП), конфиг в один тап, Android-приложение и поддержка.",
	}, nil); err != nil {
		return fmt.Errorf("setMyDescription: %w", err)
	}
	if err := b.postJSON("setMyShortDescription", map[string]any{
		"short_description": "ecstasysVPN · " + PriceRubHuman + " ₽/мес · USDT / СБП",
	}, nil); err != nil {
		return fmt.Errorf("setMyShortDescription: %w", err)
	}

	commands := []map[string]string{
		{"command": "start", "description": "Главное меню ecstasysVPN"},
		{"command": "pay", "description": "Оплатить / продлить подписку"},
		{"command": "status", "description": "Статус подписки"},
		{"command": "config", "description": "Скачать VPN-конфиг"},
		{"command": "android", "description": "Скачать Android-приложение"},
		{"command": "support", "description": "Написать в поддержку"},
		{"command": "help", "description": "Как пользоваться"},
	}
	if err := b.postJSON("setMyCommands", map[string]any{"commands": commands}, nil); err != nil {
		return fmt.Errorf("setMyCommands: %w", err)
	}
	return nil
}

// SetProfilePhoto uploads a local PNG/JPEG as the bot avatar.
func (b *Bot) SetProfilePhoto(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("photo", `{"type":"static","photo":"attach://avatar"}`)
	part, err := w.CreateFormFile("avatar", filepath.Base(path))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, b.apiURL("setMyProfilePhoto"), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return b.do(req, nil)
}

// GetMeName returns the current bot display name and username.
func (b *Bot) GetMeName() (name, username string, err error) {
	var me struct {
		FirstName string `json:"first_name"`
		Username  string `json:"username"`
	}
	if err := b.get("getMe", "", &me); err != nil {
		return "", "", err
	}
	return me.FirstName, me.Username, nil
}
