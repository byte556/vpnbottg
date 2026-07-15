package remnawave

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"vpnbottg/internal/infra/logger"
)

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
