// Package xui — асинхронный клиент 3X-UI v2.6+.
package xui

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

func tsToMs(ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	return ts * 1000
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client

	mu       sync.Mutex
	csrf     string
	authedAt time.Time
}

func NewClient(host, xuiPath, token string) *Client {
	base := strings.TrimRight(host, "/")
	if xuiPath != "" {
		base = base + "/" + strings.Trim(xuiPath, "/")
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	c := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	return &Client{
		baseURL: base,
		token:   token,
		http:    c,
	}
}

type apiResp struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*apiResp, error) {
	log := logger.L().With().Str("method", method).Str("path", path).Logger()

	url := c.baseURL + strings.TrimLeft(path, "/")

	var rb io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			log.Error().Err(err).Msg("xui marshal error")
			return nil, err
		}
		rb = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, rb)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	log.Debug().Str("url", url).Msg("xui request")

	resp, err := c.http.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("xui http error")
		return nil, err
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	log.Debug().Int("status", resp.StatusCode).Msg("xui response")

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		log.Error().Int("status", resp.StatusCode).Msg("xui unauthorized, check token")
		return nil, fmt.Errorf("xui: unauthorized (%d)", resp.StatusCode)
	}
	if resp.StatusCode == 404 {
		log.Warn().Str("path", path).Msg("xui 404")
		return &apiResp{Success: false}, nil
	}

	var r apiResp
	if err := json.Unmarshal(respBody, &r); err != nil {
		log.Error().Int("status", resp.StatusCode).Str("body", string(respBody[:min(200, len(respBody))])).Msg("xui non-JSON response")
		return nil, fmt.Errorf("xui %s %s: non-JSON (%d): %.200s", method, path, resp.StatusCode, respBody)
	}

	if !r.Success {
		log.Warn().Str("msg", r.Msg).Msg("xui api error")
	}

	return &r, nil
}

// ============ public API (panel/api/clients/*) ============

/*
	AddClient создаёт нового клиента в панели 3x-ui и привязывает его

к указанным inbound'ам за один запрос.

totalGB — лимит трафика в гигабайтах, 0 = безлимит.

expiryTime — время истечения подписки, zero value = не истекает.

limitIP — максимум одновременных подключений, 0 = без лимита.

inboundIDs — обязательны; UUID и субскрипция генерируются панелью.
*/
func (c *Client) AddClient(
	ctx context.Context,
	email string,
	totalGB int,
	expiryTime time.Time,
	limitIP int,
	inboundIDs []int,
) error {
	log := logger.L().With().Str("email", email).Logger()

	if len(inboundIDs) == 0 {
		log.Error().Msg("addClient: no inbound ids")
		return fmt.Errorf("xui addClient: no inbound ids")
	}

	log.Debug().
		Int("totalGB", totalGB).
		Int64("expiryTime", tsToMs(expiryTime.Unix())).
		Ints("inboundIds", inboundIDs).
		Int("limitIP", limitIP).
		Msg("addClient: sending request")

	body := map[string]any{
		"client": map[string]any{
			"email":      email,
			"totalGB":    gbToBytes(totalGB),
			"expiryTime": tsToMs(expiryTime.Unix()),
			"tgId":       0,
			"limitIp":    limitIP,
			"flow":       "xtls-rprx-vision",
			"enable":     true,
		},
		"inboundIds": inboundIDs,
	}

	r, err := c.do(ctx, http.MethodPost, "panel/api/clients/add", body)
	if err != nil {
		log.Error().Err(err).Msg("addClient: request failed")
		return err
	}
	if !r.Success {
		log.Warn().Str("apiMsg", r.Msg).Msg("addClient: api returned failure")
		return fmt.Errorf("xui addClient: %s", r.Msg)
	}

	log.Info().Ints("inboundIds", inboundIDs).Msg("addClient: client created")
	return nil
}

type XUIClient struct {
	ID         int    `json:"id"`
	Email      string `json:"email"`
	UUID       string `json:"uuid"`
	SubID      string `json:"subId"`
	Enable     bool   `json:"enable"`
	TotalGB    int64  `json:"totalGB"`
	ExpiryTime int64  `json:"expiryTime"`
	Flow       string `json:"flow"`
	LimitIP    int    `json:"limitIp"`
	TgID       int64  `json:"tgId"`
	InboundIDs []int  `json:"-"`
}

var ErrClientNotFound = errors.New("xui: client not found")

// GetClient возвращает клиента по email, включая список inbound ID'ов.
//
// Возвращает ErrClientNotFound если клиент не найден.
func (c *Client) GetClient(ctx context.Context, email string) (*XUIClient, error) {
	log := logger.L().With().Str("email", email).Logger()

	r, err := c.do(ctx, http.MethodGet, "panel/api/clients/get/"+email, nil)
	if err != nil {
		log.Error().Err(err).Msg("getClient: request failed")
		return nil, fmt.Errorf("getClient %s: %w", email, err)
	}
	if r == nil || !r.Success || len(r.Obj) == 0 || string(r.Obj) == "null" {
		log.Warn().Msg("getClient: client not found")
		return nil, ErrClientNotFound
	}

	log.Debug().RawJSON("obj", r.Obj).Msg("getClient: raw response")

	var wrap struct {
		Client     XUIClient `json:"client"`
		InboundIDs []int     `json:"inboundIds"`
	}
	if err := json.Unmarshal(r.Obj, &wrap); err != nil {
		log.Error().Err(err).Msg("getClient: unmarshal failed")
		return nil, fmt.Errorf("getClient %s: unmarshal: %w", email, err)
	}

	wrap.Client.InboundIDs = wrap.InboundIDs

	log.Info().Str("uuid", wrap.Client.UUID).Ints("inboundIds", wrap.InboundIDs).Msg("getClient: ok")
	return &wrap.Client, nil
}

// UpdateClientByEmail обновляет лимит трафика и срок подписки клиента.
// Перед обновлением получает текущее состояние клиента чтобы сохранить
// uuid, flow и остальные поля — панель делает replace, не patch.
//
// totalGB — новый лимит в гигабайтах, 0 = безлимит.
//
// expiryTime — новый срок истечения, zero value = не истекает.
func (c *Client) UpdateClientByEmail(
	ctx context.Context,
	email string,
	totalGB int,
	expiryTime time.Time,
) error {
	log := logger.L().With().Str("email", email).Logger()

	cl, err := c.GetClient(ctx, email)
	if err != nil {
		log.Error().Err(err).Msg("updateClient: getClient failed")
		return fmt.Errorf("updateClient %s: %w", email, err)
	}

	log.Debug().
		Int("totalGB", totalGB).
		Int64("expiryTime", tsToMs(expiryTime.Unix())).
		Msg("updateClient: sending request")

	body := map[string]any{
		"email":      email,
		"id":         cl.UUID,
		"subId":      cl.SubID,
		"flow":       cl.Flow,
		"tgId":       cl.TgID,
		"limitIp":    cl.LimitIP,
		"totalGB":    gbToBytes(totalGB),
		"expiryTime": tsToMs(expiryTime.Unix()),
		"enable":     true,
	}

	r, err := c.do(ctx, http.MethodPost, "panel/api/clients/update/"+email, body)
	if err != nil {
		log.Error().Err(err).Msg("updateClient: request failed")
		return fmt.Errorf("updateClient %s: %w", email, err)
	}
	if !r.Success {
		log.Warn().Str("apiMsg", r.Msg).Msg("updateClient: api returned failure")
		return fmt.Errorf("updateClient %s: %s", email, r.Msg)
	}

	log.Info().
		Str("uuid", cl.UUID).
		Int("totalGB", totalGB).
		Int64("expiryTime", tsToMs(expiryTime.Unix())).
		Msg("updateClient: ok")
	return nil
}

type ClientTraffic struct {
	Email string `json:"email"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
	Total int64  `json:"total"`
}

func (c *Client) GetClientTraffic(ctx context.Context, email string) (*ClientTraffic, error) {
	log := logger.L().With().Str("email", email).Logger()

	r, err := c.do(ctx, http.MethodGet, "panel/api/clients/traffic/"+email, nil)
	if err != nil {
		log.Error().Err(err).Msg("getClientTraffic: request failed")
		return nil, fmt.Errorf("getClientTraffic %s: %w", email, err)
	}
	if r == nil || !r.Success || len(r.Obj) == 0 || string(r.Obj) == "null" {
		log.Warn().Msg("getClientTraffic: no data")
		return nil, ErrClientNotFound
	}

	var t ClientTraffic
	if err := json.Unmarshal(r.Obj, &t); err != nil {
		log.Error().Err(err).Msg("getClientTraffic: unmarshal failed")
		return nil, fmt.Errorf("getClientTraffic %s: unmarshal: %w", email, err)
	}

	log.Info().Str("email", email).Int64("up", t.Up).Int64("down", t.Down).Msg("getClientTraffic: ok")
	return &t, nil
}
