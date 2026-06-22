package handlers

import (
	"context"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// TrialStart — хендлер текстовой кнопки «Попробовать 7 дней».
// Проверяет доступность и показывает инфо страницу с кнопкой оплаты.
func TrialStart(paymentSvc *service.PaymentService) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "TrialStart").Int64("user_id", c.Sender().ID).Logger()

		hasPaid, err := paymentSvc.HasEverPaid(context.Background(), c.Sender().ID)
		if err != nil {
			log.Error().Err(err).Msg("TrialStart: HasEverPaid failed")
			return c.Send(texts.T("create_payment.error"))
		}
		if hasPaid {
			return c.Send(texts.T("trial.used"))
		}

		log.Debug().Msg("TrialStart: showing info page")
		return c.Send(texts.T("trial.info"), keyboard.TrialInfo())
	}
}
