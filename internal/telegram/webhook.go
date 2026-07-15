package telegram

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/config"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/service"

	tele "gopkg.in/telebot.v3"
)

// ykCIDRs — официальные IP-диапазоны YooKassa для вебхуков.
// Источник: https://yookassa.ru/developers/using-api/webhooks
var ykCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"185.71.76.0/27",
		"185.71.77.0/27",
		"77.75.153.0/25",
		"77.75.156.11/32",
		"77.75.156.35/32",
		"77.75.154.128/25",
		"2a02:5180::/32",
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipnet, _ := net.ParseCIDR(cidr)
		nets = append(nets, ipnet)
	}
	return nets
}()

func isYooKassaIP(r *http.Request) bool {
	ipStr := r.Header.Get("X-Real-IP")
	if ipStr == "" {
		// X-Forwarded-For may contain a comma-separated list; take the first (client) IP.
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ipStr = strings.SplitN(fwd, ",", 2)[0]
			ipStr = strings.TrimSpace(ipStr)
		}
	}
	if ipStr == "" {
		ipStr, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, ipnet := range ykCIDRs {
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}

type WebhookHandler struct {
	bot     *tele.Bot
	payment *service.PaymentService
	sub     *service.Subscription
	user    *service.User
	ref     *service.ReferralService
	promo   *service.PromoService
}

func NewWebhookHandler(
	bot *tele.Bot,
	payment *service.PaymentService,
	sub *service.Subscription,
	user *service.User,
	ref *service.ReferralService,
	promo *service.PromoService,
) *WebhookHandler {
	return &WebhookHandler{bot: bot, payment: payment, sub: sub, user: user, ref: ref, promo: promo}
}

// Handle — HTTP хендлер для YooKassa webhook.
// Парсит уведомление, верифицирует через re-fetch, вызывает process.
func (h *WebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	log := logger.L().With().Str("handler", "yookassa_webhook").Logger()

	if config.Cfg.YooKassa.ValidateIPs && !isYooKassaIP(r) {
		log.Warn().Str("remote_addr", r.RemoteAddr).Str("x_real_ip", r.Header.Get("X-Real-IP")).
			Msg("Handle: IP not in YooKassa allowlist — rejected")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		log.Error().Err(err).Msg("Handle: read body failed")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	n, err := yookassa.ParseNotification(body)
	if err != nil {
		log.Error().Err(err).Msg("Handle: parse notification failed")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if n.Event != "payment.succeeded" {
		log.Debug().Str("event", n.Event).Msg("Handle: ignoring event")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Верифицируем через YooKassa API — не доверяем телу запроса напрямую.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	verified, err := h.payment.GetYkClient().FetchPayment(n.Object.ID)
	if err != nil {
		log.Error().Err(err).Str("yk_id", n.Object.ID).Msg("Handle: verify fetch failed")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if verified.Status != "succeeded" {
		log.Warn().Str("yk_id", n.Object.ID).Str("status", verified.Status).Msg("Handle: notification status mismatch — possible spoof")
		w.WriteHeader(http.StatusOK)
		return
	}

	h.process(ctx, verified)
	w.WriteHeader(http.StatusOK)
}

// StartPoller запускает фоновый поллинг pending платежей каждые 2 минуты.
// Резервный механизм на случай недоставки webhook.
