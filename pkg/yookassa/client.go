// Package yookassa is a minimal client for YooKassa payments and renewals.
package yookassa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiBase = "https://api.yookassa.ru/v3"

// Client talks to the YooKassa Payments API.
type Client struct {
	ShopID    string
	SecretKey string
	HTTP      *http.Client
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Amount is a YooKassa money value.
type Amount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// Payment is the subset of fields we care about.
type Payment struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Paid   bool   `json:"paid"`
	Amount Amount `json:"amount"`
	Confirmation *struct {
		Type            string `json:"type"`
		ConfirmationURL string `json:"confirmation_url"`
	} `json:"confirmation"`
	PaymentMethod *struct {
		ID    string `json:"id"`
		Saved bool   `json:"saved"`
		Title string `json:"title"`
	} `json:"payment_method"`
	Metadata map[string]string `json:"metadata"`
}

type createPaymentRequest struct {
	Amount              Amount            `json:"amount"`
	Capture             bool              `json:"capture"`
	Description         string            `json:"description,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	SavePaymentMethod   bool              `json:"save_payment_method,omitempty"`
	PaymentMethodID     string            `json:"payment_method_id,omitempty"`
	Confirmation        *confirmation     `json:"confirmation,omitempty"`
}

type confirmation struct {
	Type      string `json:"type"`
	ReturnURL string `json:"return_url"`
}

// CreateRedirectPayment starts a first payment that can save the card for renewals.
func (c *Client) CreateRedirectPayment(idempotenceKey, value, description, returnURL string, metadata map[string]string) (*Payment, error) {
	body := createPaymentRequest{
		Amount:            Amount{Value: value, Currency: "RUB"},
		Capture:           true,
		Description:       description,
		Metadata:          metadata,
		SavePaymentMethod: true,
		Confirmation: &confirmation{
			Type:      "redirect",
			ReturnURL: returnURL,
		},
	}
	return c.createPayment(idempotenceKey, body)
}

// ChargeSavedMethod renews a subscription using a previously saved card.
func (c *Client) ChargeSavedMethod(idempotenceKey, value, description, paymentMethodID string, metadata map[string]string) (*Payment, error) {
	body := createPaymentRequest{
		Amount:          Amount{Value: value, Currency: "RUB"},
		Capture:         true,
		Description:     description,
		Metadata:        metadata,
		PaymentMethodID: paymentMethodID,
	}
	return c.createPayment(idempotenceKey, body)
}

// GetPayment fetches the current payment state (used after webhooks).
func (c *Client) GetPayment(id string) (*Payment, error) {
	req, err := http.NewRequest(http.MethodGet, apiBase+"/payments/"+id, nil)
	if err != nil {
		return nil, err
	}
	c.authorize(req)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodePayment(resp)
}

func (c *Client) createPayment(idempotenceKey string, body createPaymentRequest) (*Payment, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, apiBase+"/payments", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotence-Key", idempotenceKey)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodePayment(resp)
}

func (c *Client) authorize(req *http.Request) {
	req.SetBasicAuth(c.ShopID, c.SecretKey)
}

func decodePayment(resp *http.Response) (*Payment, error) {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("yookassa %s: %s", resp.Status, truncate(string(raw), 400))
	}
	var payment Payment
	if err := json.Unmarshal(raw, &payment); err != nil {
		return nil, err
	}
	return &payment, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
