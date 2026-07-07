// Package subserver — отдельный HTTP-сервер выдачи подписок.
//
//	GET /              — корневой лендинг VEXA VPN с кнопкой «Открыть в Telegram».
//	GET /sub/{sub_id} — сырой конфиг для VPN-клиента (happ). Проксирует подписку
//	  из панели 3X-UI. Устройство идентифицируется по HWID (x-hwid от Happ) и
//	  фиксируется в device_connections. Если HWID не прислан или новое устройство
//	  превышает device_limit — конфиг не отдаётся (403). device_id хранит HWID.
//	GET /p/{sub_id} — фирменная страница подписки для пользователя:
//	  QR-код, кнопка «Подключить в Happ» и сама ссылка.
package subserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"vpnbottg/internal/infra/logger"
)

// DeviceAuthorizer — проверка лимита устройств + учёт по HWID.
// allowed=false → новое устройство сверх лимита, конфиг не отдаётся.
// isNew=true → устройство зарегистрировано впервые.
type DeviceAuthorizer interface {
	AuthorizeDeviceConnection(ctx context.Context, subID, deviceID, userAgent, platform string) (allowed, isNew bool, err error)
}

// upstreamHeaders — заголовки подписки, которые клиенты ждут от sub-сервиса.
var upstreamHeaders = []string{
	"Content-Type",
	"Profile-Update-Interval",
	"Subscription-Userinfo",
	"Profile-Title",
	"Profile-Web-Page-Url",
	"Content-Disposition",
}

type Server struct {
	upstreamTemplate string
	publicBaseURL    string
	botURL           string
	support          string
	repo             DeviceAuthorizer
	http             *http.Client
	OnNewDevice     func(subID, platform string)
	OnDeviceBlocked func(subID, platform string)
}

func New(upstreamTemplate, publicBaseURL, botURL, support string, repo DeviceAuthorizer) *Server {
	return &Server{
		upstreamTemplate: upstreamTemplate,
		publicBaseURL:    strings.TrimRight(publicBaseURL, "/"),
		botURL:           strings.TrimSpace(botURL),
		support:          strings.TrimSpace(support),
		repo:             repo,
		http: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// Handler возвращает маршрутизатор sub-сервера.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleLanding)
	mux.HandleFunc("GET /sub/{sub_id}", s.handleSub)
	mux.HandleFunc("GET /p/{sub_id}", s.handlePage)
	return mux
}

// subURL собирает внешнюю ссылку на сырой конфиг (её забирает VPN-клиент).
func (s *Server) subURL(subID string) string {
	return fmt.Sprintf("%s/sub/%s", s.publicBaseURL, subID)
}

func (s *Server) handleSub(w http.ResponseWriter, r *http.Request) {
	subID := r.PathValue("sub_id")
	ua := r.UserAgent()
	hw := hwid(r)
	plat := platform(r)

	log := logger.L().With().
		Str("handler", "sub").
		Str("sub_id", subID).
		Str("platform", plat).
		Logger()

	if subID == "" {
		http.NotFound(w, r)
		return
	}

	// Лимит устройств по HWID (x-hwid от Happ).
	if s.repo != nil {
		// Без HWID идентифицировать устройство нечем — строго отказываем.
		if hw == "" {
			log.Warn().Msg("sub: missing x-hwid, refusing")
			http.Error(w, "device id required", http.StatusForbidden)
			return
		}
		allowed, isNew, err := s.repo.AuthorizeDeviceConnection(r.Context(), subID, hw, ua, plat)
		switch {
		case err != nil:
			// fail-open: разовый сбой БД не должен блокировать всех пользователей.
			log.Error().Err(err).Str("hwid", hw).Msg("sub: authorize failed, serving anyway (fail-open)")
		case !allowed:
			log.Warn().Str("hwid", hw).Msg("sub: device limit reached, refusing")
			if s.OnDeviceBlocked != nil {
				go s.OnDeviceBlocked(subID, plat)
			}
			http.Error(w, "device limit reached", http.StatusForbidden)
			return
		case isNew:
			log.Info().Str("hwid", hw).Str("platform", plat).Msg("sub: new device registered")
			if s.OnNewDevice != nil {
				go s.OnNewDevice(subID, plat)
			}
		}
	}

	body, headers, status, err := s.fetchUpstream(r.Context(), subID, ua, r.Header.Get("Accept"))
	if err != nil {
		log.Error().Err(err).Msg("sub: upstream fetch failed")
		http.Error(w, "subscription unavailable", http.StatusBadGateway)
		return
	}

	for _, h := range upstreamHeaders {
		if v := headers.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	w.WriteHeader(status)
	_, _ = w.Write(body)
	if status != http.StatusOK || len(body) == 0 {
		// Панель не знает такой subId или клиент выключен/удалён —
		// пользователь получает пустой конфиг. Это всегда проблема выдачи.
		log.Warn().Int("status", status).Int("bytes", len(body)).Msg("sub: upstream returned empty/error config")
		return
	}
	log.Info().Int("status", status).Int("bytes", len(body)).Msg("sub: served")
}

// fetchUpstream забирает контент подписки из панели, проксируя User-Agent
// (панель отдаёт разный формат в зависимости от клиента).
func (s *Server) fetchUpstream(ctx context.Context, subID, ua, accept string) ([]byte, http.Header, int, error) {
	url := fmt.Sprintf(s.upstreamTemplate, subID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("read upstream body: %w", err)
	}
	return body, resp.Header, resp.StatusCode, nil
}
