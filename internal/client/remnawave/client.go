// Package remnawave — асинхронный клиент панели Remnawave (https://docs.rw).
//
// Клиент повторяет поверхность прежнего пакета xui (AddClient/GetClient/...),
// поэтому сервисы почти не меняются. Отличия модели Remnawave от 3x-ui:
//
//   - Пользователь — сущность верхнего уровня с UUID; PATCH/DELETE идут по UUID,
//     а не по email/username. GetUser по username даёт нам UUID для правок.
//   - Доступ к серверам задаётся через internal squads (UUID из конфига),
//     а не через inbound-id. Поэтому inboundIDs/subID из xui-сигнатур больше
//     не нужны — direct/relay-костыли исчезают.
//   - Ссылку подписки (subscriptionUrl) и shortUuid панель отдаёт сама.
//   - Лимит устройств — встроенный hwidDeviceLimit; трафик — trafficLimitBytes
//     (0 = безлимит); статус — ACTIVE/DISABLED.
package remnawave

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"vpnbottg/internal/infra/logger"
)

const giB = 1024 * 1024 * 1024

func gbToBytes(gb int) int64 {
	if gb <= 0 {
		return 0
	}
	return int64(gb) * giB
}

type Client struct {
	baseURL string
	token   string
	squads  []string // activeInternalSquads (UUID) для новых пользователей
	http    *http.Client

	mu sync.Mutex
}

// NewClient создаёт клиент Remnawave.
// host — базовый URL панели (например https://panel.example.com); token — API-токен
// из раздела панели «API Tokens»; squads — список UUID internal squad'ов, к которым
// привязываются создаваемые пользователи (аналог inbound-id в 3x-ui).
func NewClient(host, token string, squads []string) *Client {
	base := strings.TrimRight(host, "/")
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	c := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// Копируем срез, чтобы внешние изменения конфига не влияли на клиент.
	sq := make([]string, 0, len(squads))
	for _, s := range squads {
		if s = strings.TrimSpace(s); s != "" {
			sq = append(sq, s)
		}
	}

	return &Client{
		baseURL: base,
		token:   token,
		squads:  sq,
		http:    c,
	}
}

// resp — обёртка ответа Remnawave: полезные данные всегда в поле "response".
type resp struct {
	Response json.RawMessage `json:"response"`
}

// apiError — тело ошибки Remnawave (message/errorCode) для диагностики.
type apiError struct {
	Message   string `json:"message"`
	ErrorCode string `json:"errorCode"`
}

var ErrClientNotFound = errors.New("remnawave: user not found")

// do выполняет запрос к API. Возвращает сырой response-объект.
// notFound=true — 404 (пользователь не найден), обрабатывается вызывающим.
func (c *Client) do(ctx context.Context, method, path string, body any) (json.RawMessage, bool, error) {
	log := logger.L().With().Str("method", method).Str("path", path).Logger()

	url := c.baseURL + strings.TrimLeft(path, "/")

	var rb io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			log.Error().Err(err).Msg("remnawave marshal error")
			return nil, false, err
		}
		rb = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, rb)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	// Remnawave proxy-check middleware рвёт соединение без этих заголовков
	// (ожидает запрос из-за reverse proxy по HTTPS). Бот ходит на localhost,
	// поэтому проставляем их сами.
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	log.Debug().Str("url", url).Msg("remnawave request")

	httpResp, err := c.http.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("remnawave http error")
		return nil, false, err
	}
	respBody, _ := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()

	log.Debug().Int("status", httpResp.StatusCode).Msg("remnawave response")

	switch {
	case httpResp.StatusCode == 401 || httpResp.StatusCode == 403:
		log.Error().Int("status", httpResp.StatusCode).Msg("remnawave unauthorized, check token")
		return nil, false, fmt.Errorf("remnawave: unauthorized (%d)", httpResp.StatusCode)
	case httpResp.StatusCode == 404:
		log.Warn().Str("path", path).Msg("remnawave 404")
		return nil, true, nil
	case httpResp.StatusCode >= 400:
		var ae apiError
		_ = json.Unmarshal(respBody, &ae)
		msg := ae.Message
		if msg == "" {
			msg = fmt.Sprintf("%.200s", respBody)
		}
		log.Warn().Int("status", httpResp.StatusCode).Str("msg", msg).Str("code", ae.ErrorCode).Msg("remnawave api error")
		return nil, false, fmt.Errorf("remnawave %s %s: %s (%d)", method, path, msg, httpResp.StatusCode)
	}

	var r resp
	if err := json.Unmarshal(respBody, &r); err != nil {
		log.Error().Int("status", httpResp.StatusCode).Str("body", fmt.Sprintf("%.200s", respBody)).Msg("remnawave non-JSON response")
		return nil, false, fmt.Errorf("remnawave %s %s: non-JSON (%d): %.200s", method, path, httpResp.StatusCode, respBody)
	}

	return r.Response, false, nil
}

// user — модель пользователя Remnawave (поля из UserItemInfo).
type user struct {
	UUID              string  `json:"uuid"`
	ShortUUID         string  `json:"shortUuid"`
	Username          string  `json:"username"`
	Status            string  `json:"status"`
	TrafficLimitBytes float64 `json:"trafficLimitBytes"`
	ExpireAt          string  `json:"expireAt"`
	HwidDeviceLimit   *int    `json:"hwidDeviceLimit"`
	SubscriptionURL   string  `json:"subscriptionUrl"`
	TelegramID        *int64  `json:"telegramId"`
	UsedTrafficBytes  float64 `json:"usedTrafficBytes"`
}

// User — публичное представление клиента для сервисов (зеркалит xui.XUIClient).
// SubID = shortUuid (используется в шаблоне ссылки подписки).
type User struct {
	UUID            string
	Email           string // = username
	SubID           string // = shortUuid
	Enable          bool
	TotalGB         int64 // trafficLimitBytes
	ExpiryTime      int64 // expireAt в unix-ms (совместимость с прежним кодом)
	LimitIP         int   // hwidDeviceLimit
	SubscriptionURL string
}

func toUser(u *user) *User {
	limit := 0
	if u.HwidDeviceLimit != nil {
		limit = *u.HwidDeviceLimit
	}
	var expMs int64
	if t, err := time.Parse(time.RFC3339, u.ExpireAt); err == nil {
		expMs = t.UnixMilli()
	}
	return &User{
		UUID:            u.UUID,
		Email:           u.Username,
		SubID:           u.ShortUUID,
		Enable:          u.Status == "ACTIVE",
		TotalGB:         int64(u.TrafficLimitBytes),
		ExpiryTime:      expMs,
		LimitIP:         limit,
		SubscriptionURL: u.SubscriptionURL,
	}
}

// ============ public API ============

// AddClient создаёт пользователя в Remnawave и привязывает к squad'ам из конфига.
// totalGB=0 — безлимит; limitIP=0 — без лимита устройств.
// Идемпотентно: если username уже занят — это не ошибка (клиент уже существует).
func (c *Client) AddClient(ctx context.Context, email string, totalGB int, expiryTime time.Time, limitIP int) error {
	log := logger.L().With().Str("email", email).Logger()

	body := map[string]any{
		"username":             email,
		"status":               "ACTIVE",
		"expireAt":             expiryTime.UTC().Format(time.RFC3339),
		"trafficLimitBytes":    gbToBytes(totalGB),
		"hwidDeviceLimit":      limitIP,
		"activeInternalSquads": c.squads,
	}

	log.Debug().Int("totalGB", totalGB).Int("limitIP", limitIP).Strs("squads", c.squads).Msg("addClient: sending request")

	_, _, err := c.do(ctx, http.MethodPost, "api/users", body)
	if err != nil {
		// username уже занят — трактуем как успех (клиент от прошлой подписки).
		if strings.Contains(strings.ToLower(err.Error()), "already") || strings.Contains(err.Error(), "USER_USERNAME_ALREADY_EXISTS") {
			log.Info().Msg("addClient: user already exists — treating as success")
			return nil
		}
		log.Error().Err(err).Msg("addClient: request failed")
		return err
	}

	log.Info().Msg("addClient: user created")
	return nil
}

// GetClient возвращает пользователя по email (username). ErrClientNotFound если нет.
func (c *Client) GetClient(ctx context.Context, email string) (*User, error) {
	log := logger.L().With().Str("email", email).Logger()

	raw, notFound, err := c.do(ctx, http.MethodGet, "api/users/by-username/"+email, nil)
	if err != nil {
		log.Error().Err(err).Msg("getClient: request failed")
		return nil, fmt.Errorf("getClient %s: %w", email, err)
	}
	if notFound || len(raw) == 0 || string(raw) == "null" {
		log.Warn().Msg("getClient: user not found")
		return nil, ErrClientNotFound
	}

	// by-username может вернуть массив пользователей или одиночный объект.
	var u user
	if err := json.Unmarshal(raw, &u); err != nil {
		var arr []user
		if err2 := json.Unmarshal(raw, &arr); err2 != nil || len(arr) == 0 {
			if len(arr) == 0 && err2 == nil {
				return nil, ErrClientNotFound
			}
			log.Error().Err(err).Msg("getClient: unmarshal failed")
			return nil, fmt.Errorf("getClient %s: unmarshal: %w", email, err)
		}
		u = arr[0]
	}
	if u.UUID == "" {
		return nil, ErrClientNotFound
	}

	log.Info().Str("uuid", u.UUID).Str("shortUuid", u.ShortUUID).Msg("getClient: ok")
	return toUser(&u), nil
}

// patch применяет частичное обновление пользователя по UUID.
func (c *Client) patch(ctx context.Context, body map[string]any) error {
	_, _, err := c.do(ctx, http.MethodPatch, "api/users", body)
	return err
}

// UpdateClientByEmail обновляет трафик и срок подписки, сохраняя лимит устройств.
func (c *Client) UpdateClientByEmail(ctx context.Context, email string, totalGB int, expiryTime time.Time) error {
	cl, err := c.GetClient(ctx, email)
	if err != nil {
		return fmt.Errorf("updateClient %s: %w", email, err)
	}
	return c.updateFull(ctx, cl, totalGB, expiryTime, cl.LimitIP)
}

// AddClientCapacity добавляет GB и устройства к существующему клиенту (дельты).
func (c *Client) AddClientCapacity(ctx context.Context, email string, addGB, addDevices int) error {
	cl, err := c.GetClient(ctx, email)
	if err != nil {
		return fmt.Errorf("addCapacity %s: %w", email, err)
	}
	currentGB := int(cl.TotalGB / giB)
	currentExpiry := time.UnixMilli(cl.ExpiryTime)
	return c.updateFull(ctx, cl, currentGB+addGB, currentExpiry, cl.LimitIP+addDevices)
}

// ResetClient включает клиента (ACTIVE) и выставляет новые totalGB, срок и лимит устройств.
// Нужен при повторной покупке после истечения: клиент мог остаться DISABLED со старым сроком.
func (c *Client) ResetClient(ctx context.Context, email string, totalGB int, expiryTime time.Time, limitIP int) error {
	cl, err := c.GetClient(ctx, email)
	if err != nil {
		return fmt.Errorf("resetClient %s: %w", email, err)
	}
	return c.updateFull(ctx, cl, totalGB, expiryTime, limitIP)
}

// DisableClient деактивирует клиента (status=DISABLED).
func (c *Client) DisableClient(ctx context.Context, email string) error {
	cl, err := c.GetClient(ctx, email)
	if err != nil {
		return fmt.Errorf("disableClient %s: %w", email, err)
	}
	log := logger.L().With().Str("email", email).Logger()
	if err := c.patch(ctx, map[string]any{"uuid": cl.UUID, "status": "DISABLED"}); err != nil {
		return fmt.Errorf("disableClient %s: %w", email, err)
	}
	log.Info().Msg("disableClient: ok")
	return nil
}

// DeleteClient удаляет клиента по email. Идемпотентно (нет клиента — не ошибка).
func (c *Client) DeleteClient(ctx context.Context, email string) error {
	log := logger.L().With().Str("email", email).Logger()

	cl, err := c.GetClient(ctx, email)
	if err != nil {
		if errors.Is(err, ErrClientNotFound) {
			log.Info().Msg("deleteClient: already absent")
			return nil
		}
		return fmt.Errorf("deleteClient %s: %w", email, err)
	}

	_, notFound, err := c.do(ctx, http.MethodDelete, "api/users/"+cl.UUID, nil)
	if err != nil {
		log.Error().Err(err).Msg("deleteClient: request failed")
		return fmt.Errorf("deleteClient %s: %w", email, err)
	}
	if notFound {
		log.Info().Msg("deleteClient: already absent")
		return nil
	}
	log.Info().Msg("deleteClient: ok")
	return nil
}

func (c *Client) updateFull(ctx context.Context, cl *User, totalGB int, expiryTime time.Time, limitIP int) error {
	log := logger.L().With().Str("email", cl.Email).Logger()

	log.Debug().Int("totalGB", totalGB).Int("limitIP", limitIP).Msg("updateFull: sending request")

	body := map[string]any{
		"uuid":              cl.UUID,
		"status":            "ACTIVE",
		"expireAt":          expiryTime.UTC().Format(time.RFC3339),
		"trafficLimitBytes": gbToBytes(totalGB),
		"hwidDeviceLimit":   limitIP,
	}
	if err := c.patch(ctx, body); err != nil {
		log.Error().Err(err).Msg("updateFull: request failed")
		return fmt.Errorf("updateClient %s: %w", cl.Email, err)
	}

	log.Info().Int("totalGB", totalGB).Int("limitIP", limitIP).Msg("updateFull: ok")
	return nil
}

// ClientTraffic — использованный трафик (зеркалит xui.ClientTraffic).
type ClientTraffic struct {
	Email string
	Up    int64
	Down  int64
	Total int64
}

// GetClientTraffic возвращает использованный трафик клиента.
// Remnawave отдаёт суммарный usedTrafficBytes — кладём его в Down (Up=0),
// вызывающий код считает Up+Down.
func (c *Client) GetClientTraffic(ctx context.Context, email string) (*ClientTraffic, error) {
	log := logger.L().With().Str("email", email).Logger()

	raw, notFound, err := c.do(ctx, http.MethodGet, "api/users/by-username/"+email, nil)
	if err != nil {
		log.Error().Err(err).Msg("getClientTraffic: request failed")
		return nil, fmt.Errorf("getClientTraffic %s: %w", email, err)
	}
	if notFound || len(raw) == 0 || string(raw) == "null" {
		return nil, ErrClientNotFound
	}

	var u user
	if err := json.Unmarshal(raw, &u); err != nil {
		var arr []user
		if err2 := json.Unmarshal(raw, &arr); err2 != nil || len(arr) == 0 {
			return nil, ErrClientNotFound
		}
		u = arr[0]
	}

	t := &ClientTraffic{
		Email: email,
		Down:  int64(u.UsedTrafficBytes),
		Total: int64(u.UsedTrafficBytes),
	}
	log.Info().Int64("used", t.Total).Msg("getClientTraffic: ok")
	return t, nil
}