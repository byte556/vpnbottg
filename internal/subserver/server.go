// Package subserver — отдельный HTTP-сервер выдачи подписок.
//
//	GET /              — корневой лендинг VEXA VPN с кнопкой «Открыть в Telegram».
//	GET /sub/{sub_id}  — единый endpoint: VPN-клиент (Happ и т.п.) получает сырой конфиг,
//	  браузер — фирменную HTML-страницу подписки с QR и кнопками.
//	GET /p/{sub_id}    — алиас: всегда отдаёт HTML-страницу (обратная совместимость).
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

// upstreamHeaders — заголовки подписки, которые клиенты ждут от sub-сервиса.
var upstreamHeaders = []string{
	"Content-Type",
	"Profile-Update-Interval",
	"Subscription-Userinfo",
	"Profile-Title",
	"Profile-Web-Page-Url",
	"Content-Disposition",
}

// hwidHeaders — заголовки идентификации устройства, которые шлёт Happ и читает
// панель Remnawave для нативного HWID-лимита (обязателен только x-hwid).
var hwidHeaders = []string{"x-hwid", "x-device-os", "x-ver-os", "x-device-model"}

type Server struct {
	upstreamTemplate string
	publicBaseURL    string
	botURL           string
	support          string
	http             *http.Client
}

func New(upstreamTemplate, publicBaseURL, botURL, support string) *Server {
	return &Server{
		upstreamTemplate: upstreamTemplate,
		publicBaseURL:    strings.TrimRight(publicBaseURL, "/"),
		botURL:           strings.TrimSpace(botURL),
		support:          strings.TrimSpace(support),
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
	mux.HandleFunc("GET /sub/{sub_id}", s.handleUnified)
	mux.HandleFunc("GET /p/{sub_id}", s.handlePage)
	return mux
}

// handleUnified — единый endpoint: Happ получает конфиг, браузер — HTML-страницу.
func (s *Server) handleUnified(w http.ResponseWriter, r *http.Request) {
	if isVPNClient(r.UserAgent()) {
		s.handleSub(w, r)
		return
	}
	s.handlePage(w, r)
}

func isVPNClient(ua string) bool {
	ua = strings.ToLower(ua)
	for _, sig := range []string{"happ", "v2ray", "clash", "sing-box", "stash", "shadowrocket", "quantumult", "surge", "loon"} {
		if strings.Contains(ua, sig) {
			return true
		}
	}
	return false
}

// subURL собирает внешнюю ссылку на сырой конфиг (её забирает VPN-клиент).
func (s *Server) subURL(subID string) string {
	return fmt.Sprintf("%s/sub/%s", s.publicBaseURL, subID)
}

func (s *Server) handleSub(w http.ResponseWriter, r *http.Request) {
	subID := r.PathValue("sub_id")
	ua := r.UserAgent()

	log := logger.L().With().
		Str("handler", "sub").
		Str("sub_id", subID).
		Logger()

	if subID == "" {
		http.NotFound(w, r)
		return
	}

	// Лимит устройств по HWID теперь обеспечивает панель Remnawave: она читает
	// x-hwid/x-device-os и сама режет по hwidDeviceLimit. Наша задача — прокинуть
	// эти заголовки наверх (см. fetchUpstream).
	body, headers, status, err := s.fetchUpstream(r.Context(), subID, ua, r.Header.Get("Accept"), r.Header)
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
// (панель отдаёт разный формат в зависимости от клиента) и заголовки HWID
// (панель по ним ведёт нативный учёт устройств).
func (s *Server) fetchUpstream(ctx context.Context, subID, ua, accept string, clientHeaders http.Header) ([]byte, http.Header, int, error) {
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
	// Прокидываем заголовки идентификации устройства — по ним панель Remnawave
	// регистрирует HWID и применяет лимит устройств.
	for _, h := range hwidHeaders {
		if v := clientHeaders.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	// Remnawave proxy-check требует эти заголовки (ждёт запрос из-за reverse
	// proxy по HTTPS). Безвредны для любого upstream'а.
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "127.0.0.1")

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
