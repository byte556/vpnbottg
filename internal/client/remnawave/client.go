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
