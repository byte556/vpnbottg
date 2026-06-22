package callbacks

import (
	"context"
	"fmt"
	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/config"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/handlers"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func TrialBuy(paymentSvc *service.PaymentService, userSvc *service.User) tele.HandlerFunc {
	return func(c tele.Context) error {
		ctx := context.Background()
		log := logger.L().With().Str("func", "TrialBuy").Int64("user_id", c.Sender().ID).Logger()

		// Повторная проверка — на случай если пользователь купил пока смотрел страницу.
		hasPaid, err := paymentSvc.HasEverPaid(ctx, c.Sender().ID)
		if err != nil {
			log.Error().Err(err).Msg("TrialBuy: HasEverPaid failed")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("create_payment.error")})
		}
		if hasPaid {
			return c.Respond(&tele.CallbackResponse{Text: texts.T("trial.used")})
		}

		if err := userSvc.EnsureUser(ctx, c.Sender().ID, c.Sender().Username, c.Sender().FirstName, nil); err != nil {
			log.Error().Err(err).Msg("TrialBuy: EnsureUser failed")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("error.user_create")})
		}

		cfg := config.Cfg.Bot.Constructor.Trial
		ykPayment, _, err := paymentSvc.InitiatePayment(c.Sender().ID, yookassa.CreatePaymentReq{
			AmountRub:   cfg.PriceRub,
			Description: fmt.Sprintf("Пробный VPN %d ГБ / %d уст. / %d дней", cfg.GB, cfg.Devices, cfg.Days),
			SaveMethod:  false,
			Metadata: map[string]string{
				"tg_id":    fmt.Sprintf("%d", c.Sender().ID),
				"total_gb": fmt.Sprintf("%d", cfg.GB),
				"devices":  fmt.Sprintf("%d", cfg.Devices),
				"days":     fmt.Sprintf("%d", cfg.Days),
				"trial":    "true",
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("TrialBuy: InitiatePayment failed")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("create_payment.error")})
		}

		sess := session.GetStore().Get(c.Sender().ID)
		sess.PaymentID = ykPayment.ID
		sess.PaymentURL = ykPayment.ConfirmationURL
		session.GetStore().Save(c.Sender().ID, sess)

		log.Info().Str("yk_id", ykPayment.ID).Msg("TrialBuy: payment created")
		return handlers.PaymentPending(c)
	}
}
