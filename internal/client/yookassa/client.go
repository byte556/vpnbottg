package yookassa

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client — минимальный синхронный клиент.
type Client struct {
	ShopID    string
	SecretKey string
	ReturnURL string
	HTTP      *http.Client
}

func NewClient(shopID, secretKey, returnURL string) *Client {
	return &Client{
		ShopID:    shopID,
		SecretKey: secretKey,
		ReturnURL: returnURL,
		HTTP:      &http.Client{Timeout: 20 * time.Second},
	}
}

// Enabled — нет ли пустых ключей.
func (c *Client) Enabled() bool {
	return c != nil && c.ShopID != "" && c.SecretKey != ""
}

// CreatePaymentReq — запрос создания платежа.
type CreatePaymentReq struct {
	AmountRub       int
	Description     string
	SaveMethod      bool   // запрашиваем токен карты у YK
	PaymentMethodID string // !="" → автосписание по сохранённой карте
	IdempotenceKey  string // если пусто — сгенерим
	// Metadata — произвольные пары (до 16 ключей), которые Webhook вернёт
	// и в FetchPayment, и в webhook-уведомлении. Кладём сюда что куплено
	// (preset/period/kind/tg), чтобы webhook мог активировать подписку,
	// зная только payment id.
	Metadata map[string]string
}

// Payment — единый ответ YK (POST create или GET status).
type Payment struct {
	ID              string            `json:"id"`
	Status          string            `json:"status"` // pending|succeeded|canceled|waiting_for_capture
	Paid            bool              `json:"paid"`
	ConfirmationURL string            // вытаскиваем из confirmation.confirmation_url
	PaymentMethodID string            // вытаскиваем из payment_method.id (если saved)
	Last4           string            // payment_method.card.last4
	Metadata        map[string]string // то, что мы передали при создании
}

type rawConfirmation struct {
	ConfirmationURL string `json:"confirmation_url"`
}
type rawCard struct {
	Last4 string `json:"last4"`
}
type rawMethod struct {
	ID    string  `json:"id"`
	Saved bool    `json:"saved"`
	Card  rawCard `json:"card"`
}
type rawPayment struct {
	ID            string            `json:"id"`
	Status        string            `json:"status"`
	Paid          bool              `json:"paid"`
	Confirmation  rawConfirmation   `json:"confirmation"`
	PaymentMethod rawMethod         `json:"payment_method"`
	Metadata      map[string]string `json:"metadata"`
}

func (r rawPayment) toPayment() Payment {
	return Payment{
		ID:              r.ID,
		Status:          r.Status,
		Paid:            r.Paid,
		ConfirmationURL: r.Confirmation.ConfirmationURL,
		PaymentMethodID: r.PaymentMethod.ID,
		Last4:           r.PaymentMethod.Card.Last4,
		Metadata:        r.Metadata,
	}
}

// CreatePayment — POST /payments
func (c *Client) CreatePayment(req CreatePaymentReq) (*Payment, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("yookassa not configured")
	}

	body := map[string]any{
		"amount": map[string]any{
			"value":    fmt.Sprintf("%d.00", req.AmountRub),
			"currency": "RUB",
		},
		"capture":     true,
		"description": req.Description,
	}

	// Если это списание по сохранённой карте — нет редиректа, просто списываем.
	if req.PaymentMethodID != "" {
		body["payment_method_id"] = req.PaymentMethodID
	} else {
		// Иначе — нужен confirmation.return_url, юзер пойдёт оплачивать.
		body["confirmation"] = map[string]any{
			"type":       "redirect",
			"return_url": c.ReturnURL,
		}
	}

	if req.SaveMethod {
		body["save_payment_method"] = true
	}

	if len(req.Metadata) > 0 {
		body["metadata"] = req.Metadata
	}

	if req.IdempotenceKey == "" {
		req.IdempotenceKey = randomHex(16)
	}

	raw, err := c.do(http.MethodPost, "https://api.yookassa.ru/v3/payments",
		body, req.IdempotenceKey)
	if err != nil {
		return nil, err
	}

	var rp rawPayment
	if err := json.Unmarshal(raw, &rp); err != nil {
		return nil, fmt.Errorf("decode create payment: %w; body: %s", err, raw)
	}
	p := rp.toPayment()
	return &p, nil
}

// FetchPayment — GET /payments/{id}
func (c *Client) FetchPayment(providerID string) (*Payment, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("yookassa not configured")
	}
	raw, err := c.do(http.MethodGet,
		"https://api.yookassa.ru/v3/payments/"+providerID, nil, "")
	if err != nil {
		return nil, err
	}
	var rp rawPayment
	if err := json.Unmarshal(raw, &rp); err != nil {
		return nil, fmt.Errorf("decode fetch payment: %w; body: %s", err, raw)
	}
	p := rp.toPayment()
	return &p, nil
}

// do — общий HTTP запрос с Basic Auth и Idempotence-Key.
func (c *Client) do(method, url string, body any, idemp string) ([]byte, error) {
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.ShopID, c.SecretKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idemp != "" {
		req.Header.Set("Idempotence-Key", idemp)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody := readAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("yookassa %s %s: HTTP %d: %s",
			method, url, resp.StatusCode, respBody)
	}
	return respBody, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// readAll — чтение всего тела (sloppy: для коротких ответов YK достаточно).
func readAll(r interface{ Read(p []byte) (int, error) }) []byte {
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			break
		}
	}
	return buf.Bytes()
}
