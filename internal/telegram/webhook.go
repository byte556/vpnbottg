package telegram

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/config"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	"github.com/rs/zerolog"
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
}

func NewWebhookHandler(
	bot *tele.Bot,
	payment *service.PaymentService,
	sub *service.Subscription,
	user *service.User,
	ref *service.ReferralService,
) *WebhookHandler {
	return &WebhookHandler{bot: bot, payment: payment, sub: sub, user: user, ref: ref}
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
func (h *WebhookHandler) StartPoller(ctx context.Context) {
	log := logger.L().With().Str("service", "payment_poller").Logger()
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	log.Info().Msg("poller: started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("poller: stopped")
			return
		case <-ticker.C:
			h.poll(ctx)
		}
	}
}

func (h *WebhookHandler) poll(ctx context.Context) {
	log := logger.L().With().Str("func", "poll").Logger()

	pending, err := h.payment.GetPendingPayments(ctx)
	if err != nil {
		log.Error().Err(err).Msg("poll: GetPendingPayments failed")
		return
	}
	if len(pending) == 0 {
		return
	}
	log.Info().Int("count", len(pending)).Msg("poll: checking pending payments")

	for _, p := range pending {
		plog := log.With().Str("yk_id", p.ProviderPaymentID).Int64("user_id", p.UserID).Logger()

		ykPayment, err := h.payment.GetYkClient().FetchPayment(p.ProviderPaymentID)
		if err != nil {
			plog.Error().Err(err).Msg("poll: FetchPayment failed")
			continue
		}

		switch ykPayment.Status {
		case "succeeded":
			plog.Info().Msg("poll: payment succeeded — provisioning")
			h.process(ctx, ykPayment)
		case "canceled":
			plog.Info().Msg("poll: payment canceled — updating db")
			if err := h.payment.Cancel(ctx, p.ProviderPaymentID); err != nil {
				plog.Error().Err(err).Msg("poll: Cancel failed")
			}
		default:
			plog.Debug().Str("status", ykPayment.Status).Msg("poll: still pending")
		}
	}
}

// process — общая логика подтверждения и выдачи VPN.
// Вызывается и из webhook Handle, и из поллера.
func (h *WebhookHandler) process(ctx context.Context, ykPayment *yookassa.Payment) {
	log := logger.L().With().Str("func", "process").Str("yk_id", ykPayment.ID).Logger()

	tgIDStr := ykPayment.Metadata["tg_id"]
	if tgIDStr == "" {
		log.Warn().Msg("process: no tg_id in metadata")
		return
	}
	tgID, err := strconv.ParseInt(tgIDStr, 10, 64)
	if err != nil {
		log.Error().Err(err).Str("tg_id_str", tgIDStr).Msg("process: invalid tg_id")
		return
	}
	log = log.With().Int64("tg_id", tgID).Logger()

	changed, err := h.payment.Confirm(ctx, ykPayment.ID)
	if err != nil {
		log.Error().Err(err).Msg("process: Confirm failed")
		h.send(tgID, texts.T("error.payment_process"))
		return
	}
	if !changed {
		log.Info().Msg("process: already confirmed, skip")
		return
	}

	if err := h.user.EnsureUser(ctx, tgID, "", "", nil); err != nil {
		log.Error().Err(err).Msg("process: EnsureUser failed")
		h.send(tgID, texts.T("error.user_create"))
		return
	}

	meta := ykPayment.Metadata
	if meta["addon_type"] == "device" {
		amount, _ := strconv.Atoi(meta["amount"])
		log.Info().Int("devices", amount).Msg("process: device addon")
		if err := h.sub.AddDevice(ctx, tgID, amount); err != nil {
			log.Error().Err(err).Msg("process: AddDevice failed")
			h.send(tgID, texts.T("error.provision"))
			return
		}
		h.send(tgID, texts.T("addon.device_success", map[string]any{"Devices": amount}))
		h.sendSubscriberMenu(ctx, tgID)
		return
	}

	planGBStr := meta["total_gb"]
	devicesStr := meta["devices"]
	monthsStr := meta["months"]
	if planGBStr == "" || devicesStr == "" || monthsStr == "" {
		log.Warn().Msg("process: missing subscription metadata")
		h.send(tgID, texts.T("error.incomplete_metadata"))
		return
	}
	planGB, _ := strconv.Atoi(planGBStr)
	devices, _ := strconv.Atoi(devicesStr)
	months, _ := strconv.Atoi(monthsStr)

	log.Info().Int("gb", planGB).Int("devices", devices).Int("months", months).Msg("process: provisioning subscription")
	subURL, err := h.sub.ProvisionFromPayment(ctx, tgID, planGB, devices, months)
	if err != nil {
		log.Error().Err(err).Msg("process: ProvisionFromPayment failed")
		h.send(tgID, texts.T("error.provision"))
		return
	}
	h.sendHTML(tgID, texts.T("provision.success"))
	h.sendSubscriberMenu(ctx, tgID)
	log.Info().Str("sub_url", subURL).Msg("process: ok")

	h.rewardReferrer(ctx, tgID, log)
}

func (h *WebhookHandler) rewardReferrer(ctx context.Context, refereeID int64, log zerolog.Logger) {
	if h.ref == nil {
		return
	}
	referrerID, err := h.ref.Reward(ctx, refereeID)
	if err != nil {
		log.Error().Err(err).Msg("rewardReferrer: failed")
		return
	}
	if referrerID == 0 {
		return
	}
	refUser, _ := h.user.GetUser(ctx, refereeID)
	name := "Друг"
	if refUser != nil {
		if refUser.Username != "" {
			name = "@" + refUser.Username
		} else if refUser.FirstName != "" {
			name = refUser.FirstName
		}
	}
	days := config.Cfg.Bot.ReferralRewardDays
	h.send(referrerID, texts.T("referral.reward", map[string]any{"Name": name, "Days": days}))
	log.Info().Int64("referrer_id", referrerID).Int("days", days).Msg("rewardReferrer: notified")
}

func (h *WebhookHandler) send(userID int64, text string) {
	if _, err := h.bot.Send(&tele.User{ID: userID}, text); err != nil {
		logger.L().Error().Err(err).Int64("user_id", userID).Msg("webhook: bot send failed")
	}
}

func (h *WebhookHandler) sendHTML(userID int64, text string) {
	if _, err := h.bot.Send(&tele.User{ID: userID}, text, &tele.SendOptions{ParseMode: tele.ModeHTML}); err != nil {
		logger.L().Error().Err(err).Int64("user_id", userID).Msg("webhook: bot sendHTML failed")
	}
}

// sendSubscriberMenu отправляет subscriber меню с обновлённой клавиатурой.
func (h *WebhookHandler) sendSubscriberMenu(ctx context.Context, userID int64) {
	sub, err := h.sub.GetActive(ctx, userID)
	if err != nil {
		logger.L().Warn().Err(err).Int64("user_id", userID).Msg("sendSubscriberMenu: GetActive failed")
		return
	}
	daysLeft := max(0, int(time.Until(time.Unix(sub.ExpiresAt, 0)).Hours()/24))
	text := texts.T("menu.subscriber.text", map[string]any{
		"TrafficGB": sub.TrafficGB,
		"DaysLeft":  daysLeft,
	})
	if _, err := h.bot.Send(&tele.User{ID: userID}, text, keyboard.SubscriberMenu()); err != nil {
		logger.L().Error().Err(err).Int64("user_id", userID).Msg("sendSubscriberMenu: send failed")
	}
}
