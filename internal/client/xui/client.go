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

// AddClient создаёт нового клиента в панели 3x-ui и привязывает его к указанным inbound'ам.
// subID — если непустой, клиент получает этот SubID (используется чтобы direct и relay делили одну ссылку).
// totalGB=0 — безлимит; expiryTime zero — не истекает; limitIP=0 — без лимита.
func (c *Client) AddClient(
	ctx context.Context,
	email string,
	totalGB int,
	expiryTime time.Time,
	limitIP int,
	inboundIDs []int,
	subID string,
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

	client := map[string]any{
		"email":      email,
		"totalGB":    gbToBytes(totalGB),
		"expiryTime": tsToMs(expiryTime.Unix()),
		"tgId":       0,
		"limitIp":    limitIP,
		"flow":       "xtls-rprx-vision",
		"enable":     true,
	}
	if subID != "" {
		client["subId"] = subID
	}

	body := map[string]any{
		"client":     client,
		"inboundIds": inboundIDs,
	}

	r, err := c.do(ctx, http.MethodPost, "panel/api/clients/add", body)
	if err != nil {
		log.Error().Err(err).Msg("addClient: request failed")
		return err
	}
	if !r.Success {
		if strings.Contains(r.Msg, "email already in use") {
			log.Info().Str("email", email).Msg("addClient: client already exists — treating as success")
			return nil
		}
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

// UpdateClientByEmail обновляет трафик и срок подписки, сохраняя limitIp.
func (c *Client) UpdateClientByEmail(ctx context.Context, email string, totalGB int, expiryTime time.Time) error {
	cl, err := c.GetClient(ctx, email)
	if err != nil {
		return fmt.Errorf("updateClient %s: %w", email, err)
	}
	return c.updateClientFull(ctx, cl, totalGB, expiryTime, cl.LimitIP)
}

// AddClientCapacity добавляет GB и устройства к существующему клиенту.
// addGB и addDevices — дельты (например, +10 GB, +1 устройство).
func (c *Client) AddClientCapacity(ctx context.Context, email string, addGB, addDevices int) error {
	cl, err := c.GetClient(ctx, email)
	if err != nil {
		return fmt.Errorf("addCapacity %s: %w", email, err)
	}
	currentGB := int(cl.TotalGB / giB)
	currentExpiry := time.UnixMilli(cl.ExpiryTime)
	return c.updateClientFull(ctx, cl, currentGB+addGB, currentExpiry, cl.LimitIP+addDevices)
}

// SetClientSubID присваивает клиенту указанный subId, сохраняя все остальные поля.
// Используется чтобы direct и relay клиенты делили один subId — тогда одна
// подписка (subscription-сервер группирует по subId) отдаёт серверы обоих.
// totalGB/expiryTime берутся как есть (уже в bytes/ms из GetClient).
func (c *Client) SetClientSubID(ctx context.Context, email, subID string) error {
	log := logger.L().With().Str("email", email).Str("subId", subID).Logger()

	cl, err := c.GetClient(ctx, email)
	if err != nil {
		return fmt.Errorf("setClientSubID %s: %w", email, err)
	}
	if cl.SubID == subID {
		log.Debug().Msg("setClientSubID: already set, skipping")
		return nil
	}

	body := map[string]any{
		"email":      cl.Email,
		"id":         cl.UUID,
		"subId":      subID,
		"flow":       cl.Flow,
		"tgId":       cl.TgID,
		"limitIp":    cl.LimitIP,
		"totalGB":    cl.TotalGB,
		"expiryTime": cl.ExpiryTime,
		"enable":     true,
	}
	r, err := c.do(ctx, http.MethodPost, "panel/api/clients/update/"+email, body)
	if err != nil {
		log.Error().Err(err).Msg("setClientSubID: request failed")
		return fmt.Errorf("setClientSubID %s: %w", email, err)
	}
	if !r.Success {
		log.Warn().Str("apiMsg", r.Msg).Msg("setClientSubID: api returned failure")
		return fmt.Errorf("setClientSubID %s: %s", email, r.Msg)
	}

	log.Info().Msg("setClientSubID: ok")
	return nil
}

// DeleteClient удаляет клиента из панели по email.
// Если клиент не найден — не возвращает ошибку (идемпотентно).
func (c *Client) DeleteClient(ctx context.Context, email string) error {
	log := logger.L().With().Str("email", email).Logger()

	r, err := c.do(ctx, http.MethodPost, "panel/api/clients/del/"+email, nil)
	if err != nil {
		log.Error().Err(err).Msg("deleteClient: request failed")
		return fmt.Errorf("deleteClient %s: %w", email, err)
	}
	if r != nil && !r.Success && r.Msg != "" {
		log.Warn().Str("msg", r.Msg).Msg("deleteClient: panel error")
		return fmt.Errorf("deleteClient %s: %s", email, r.Msg)
	}
	log.Info().Msg("deleteClient: ok")
	return nil
}

// DisableClient деактивирует клиента в панели (enable: false).
func (c *Client) DisableClient(ctx context.Context, email string) error {
	cl, err := c.GetClient(ctx, email)
	if err != nil {
		return fmt.Errorf("disableClient %s: %w", email, err)
	}
	log := logger.L().With().Str("email", email).Logger()
	body := map[string]any{
		"email":      cl.Email,
		"id":         cl.UUID,
		"subId":      cl.SubID,
		"flow":       cl.Flow,
		"tgId":       cl.TgID,
		"limitIp":    cl.LimitIP,
		"totalGB":    cl.TotalGB,
		"expiryTime": cl.ExpiryTime,
		"enable":     false,
	}
	r, err := c.do(ctx, http.MethodPost, "panel/api/clients/update/"+email, body)
	if err != nil {
		return fmt.Errorf("disableClient %s: %w", email, err)
	}
	if !r.Success {
		return fmt.Errorf("disableClient %s: %s", email, r.Msg)
	}
	log.Info().Msg("disableClient: ok")
	return nil
}

func (c *Client) updateClientFull(ctx context.Context, cl *XUIClient, totalGB int, expiryTime time.Time, limitIP int) error {
	log := logger.L().With().Str("email", cl.Email).Logger()

	log.Debug().Int("totalGB", totalGB).Int("limitIP", limitIP).Msg("updateClientFull: sending request")

	body := map[string]any{
		"email":      cl.Email,
		"id":         cl.UUID,
		"subId":      cl.SubID,
		"flow":       cl.Flow,
		"tgId":       cl.TgID,
		"limitIp":    limitIP,
		"totalGB":    gbToBytes(totalGB),
		"expiryTime": tsToMs(expiryTime.Unix()),
		"enable":     true,
	}

	r, err := c.do(ctx, http.MethodPost, "panel/api/clients/update/"+cl.Email, body)
	if err != nil {
		log.Error().Err(err).Msg("updateClientFull: request failed")
		return fmt.Errorf("updateClient %s: %w", cl.Email, err)
	}
	if !r.Success {
		log.Warn().Str("apiMsg", r.Msg).Msg("updateClientFull: api returned failure")
		return fmt.Errorf("updateClient %s: %s", cl.Email, r.Msg)
	}

	log.Info().Int("totalGB", totalGB).Int("limitIP", limitIP).Msg("updateClientFull: ok")
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
