package telegram

import (
	"context"
	"time"

	"vpnbottg/internal/infra/logger"
)

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
