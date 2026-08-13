// Package cryptopay is a thin client for the Crypto Pay API (@CryptoBot).
package cryptopay

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const mainnetBase = "https://pay.crypt.bot/api"

// Client talks to Crypto Pay on behalf of one app.
type Client struct {
	Token   string
	BaseURL string
	HTTP    *http.Client
}

// Invoice is a Crypto Pay invoice.
type Invoice struct {
	InvoiceID      int64  `json:"invoice_id"`
	Hash           string `json:"hash"`
	CurrencyType   string `json:"currency_type"`
	Asset          string `json:"asset"`
	Fiat           string `json:"fiat"`
	Amount         string `json:"amount"`
	PaidAsset      string `json:"paid_asset"`
	PaidAmount     string `json:"paid_amount"`
	Status         string `json:"status"`
	BotInvoiceURL  string `json:"bot_invoice_url"`
	MiniAppURL     string `json:"mini_app_invoice_url"`
	WebAppURL      string `json:"web_app_invoice_url"`
	Description    string `json:"description"`
	Payload        string `json:"payload"`
	CreatedAt      string `json:"created_at"`
	PaidAt         string `json:"paid_at"`
	ExpirationDate string `json:"expiration_date"`
}

// WebhookUpdate is sent to the app's webhook URL.
type WebhookUpdate struct {
	UpdateID   int64   `json:"update_id"`
	UpdateType string  `json:"update_type"`
	RequestAt  string  `json:"request_date"`
	Payload    Invoice `json:"payload"`
}

// CreateInvoiceParams configures a new invoice.
type CreateInvoiceParams struct {
	CurrencyType   string
	Asset          string
	Fiat           string
	Amount         string
	AcceptedAssets string
	Description    string
	HiddenMessage  string
	Payload        string
	PaidBtnName    string
	PaidBtnURL     string
	ExpiresIn      int
}

func (c *Client) apiBase() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return mainnetBase
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

type apiResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

// GetMe checks the API token.
func (c *Client) GetMe() (map[string]any, error) {
	var out map[string]any
	if err := c.get("getMe", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateInvoice opens a payment link in @CryptoBot.
func (c *Client) CreateInvoice(p CreateInvoiceParams) (*Invoice, error) {
	form := url.Values{}
	if p.CurrencyType != "" {
		form.Set("currency_type", p.CurrencyType)
	}
	if p.Asset != "" {
		form.Set("asset", p.Asset)
	}
	if p.Fiat != "" {
		form.Set("fiat", p.Fiat)
	}
	form.Set("amount", p.Amount)
	if p.AcceptedAssets != "" {
		form.Set("accepted_assets", p.AcceptedAssets)
	}
	if p.Description != "" {
		form.Set("description", p.Description)
	}
	if p.HiddenMessage != "" {
		form.Set("hidden_message", p.HiddenMessage)
	}
	if p.Payload != "" {
		form.Set("payload", p.Payload)
	}
	if p.PaidBtnName != "" {
		form.Set("paid_btn_name", p.PaidBtnName)
	}
	if p.PaidBtnURL != "" {
		form.Set("paid_btn_url", p.PaidBtnURL)
	}
	if p.ExpiresIn > 0 {
		form.Set("expires_in", fmt.Sprintf("%d", p.ExpiresIn))
	}

	var invoice Invoice
	if err := c.postForm("createInvoice", form, &invoice); err != nil {
		return nil, err
	}
	return &invoice, nil
}

// GetInvoices fetches invoices by id.
func (c *Client) GetInvoices(ids ...int64) ([]Invoice, error) {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	q := url.Values{}
	q.Set("invoice_ids", strings.Join(parts, ","))
	var list []Invoice
	if err := c.get("getInvoices", q, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// VerifyWebhookSignature checks crypto-pay-api-signature against the raw body.
func VerifyWebhookSignature(token string, rawBody []byte, signature string) bool {
	secret := sha256.Sum256([]byte(token))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write(rawBody)
	expected := mac.Sum(nil)
	got, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return false
	}
	return hmac.Equal(expected, got)
}

func (c *Client) get(method string, query url.Values, out any) error {
	u := c.apiBase() + "/" + method
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Crypto-Pay-API-Token", c.Token)
	return c.do(req, out)
}

func (c *Client) postForm(method string, form url.Values, out any) error {
	req, err := http.NewRequest(http.MethodPost, c.apiBase()+"/"+method, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Crypto-Pay-API-Token", c.Token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cryptopay http %s: %s", resp.Status, bytes.TrimSpace(raw))
	}

	var envelope apiResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("cryptopay decode: %w", err)
	}
	if !envelope.OK {
		if envelope.Error == "" {
			envelope.Error = "unknown error"
		}
		return fmt.Errorf("cryptopay: %s", envelope.Error)
	}
	if out == nil {
		return nil
	}
	if len(envelope.Result) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}
