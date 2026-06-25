package callbacks

import (
	"context"
	"fmt"
	"strconv"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/handlers"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
	yookassa "vpnbottg/internal/client/yookassa"
)

func DevicesDec(c tele.Context) error {
	sess := session.GetStore().Get(c.Sender().ID)
	sess.PaymentID = ""
	sess.Constructor.AddDevices(-1)
	session.GetStore().Save(c.Sender().ID, sess)
	return handlers.Constructor(c)
}

func DevicesInc(c tele.Context) error {
	sess := session.GetStore().Get(c.Sender().ID)
	sess.PaymentID = ""
	sess.Constructor.AddDevices(1)
	session.GetStore().Save(c.Sender().ID, sess)
	return handlers.Constructor(c)
}

func Month(months int) tele.HandlerFunc {
	return func(c tele.Context) error {
		sess := session.GetStore().Get(c.Sender().ID)
		sess.PaymentID = ""
		sess.Constructor.SetMonths(months)
		session.GetStore().Save(c.Sender().ID, sess)
		if sess.RenewMode {
			return handlers.RenewRender(c)
		}
		return handlers.Constructor(c)
	}
}

func Buy(paymentServ *service.PaymentService, userSvc *service.User) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "Buy").Int64("user_id", c.Sender().ID).Logger()

		sess := session.GetStore().Get(c.Sender().ID)
		if sess.PaymentID != "" {
			log.Debug().Str("payment_id", sess.PaymentID).Msg("Buy: payment already pending")
			return handlers.PaymentPending(c)
		}

		if err := userSvc.EnsureUser(context.Background(), c.Sender().ID, c.Sender().Username, c.Sender().FirstName, nil); err != nil {
			log.Error().Err(err).Msg("Buy: EnsureUser failed")
			return handlers.PaymentError(c)
		}

		price := sess.ApplyDiscount(sess.Constructor.CalcPrice())
		description := fmt.Sprintf(
			"VPN %d подкл. / %d мес",
			sess.Constructor.GetDevices(),
			sess.Constructor.GetMonths(),
		)
		log.Info().Int("price", price).Int("promo_pct", sess.PromoDiscountPct).Str("desc", description).Msg("Buy: initiating payment")

		ykPayment, _, err := paymentServ.InitiatePayment(c.Sender().ID, yookassa.CreatePaymentReq{
			AmountRub:   price,
			Description: description,
			SaveMethod:  true,
			Metadata: map[string]string{
				"tg_id":      fmt.Sprintf("%d", c.Sender().ID),
				"devices":    fmt.Sprintf("%d", sess.Constructor.GetDevices()),
				"months":     fmt.Sprintf("%d", sess.Constructor.GetMonths()),
				"promo_code": sess.PromoCode,
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("Buy: InitiatePayment failed")
			return handlers.PaymentError(c)
		}

		sess.PaymentID = ykPayment.ID
		sess.PaymentURL = ykPayment.ConfirmationURL
		session.GetStore().Save(c.Sender().ID, sess)
		log.Info().Str("yk_id", ykPayment.ID).Msg("Buy: payment created")

		return handlers.PaymentPending(c)
	}
}

func CheckPayment(yk *service.PaymentService, sub *service.Subscription, user *service.User, promo *service.PromoService) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "CheckPayment").Int64("user_id", c.Sender().ID).Logger()

		paymentID := c.Data()
		log.Info().Str("payment_id", paymentID).Msg("CheckPayment: fetching")

		payment, err := yk.GetYkClient().FetchPayment(paymentID)
		if err != nil {
			log.Error().Err(err).Str("payment_id", paymentID).Msg("CheckPayment: FetchPayment failed")
			// popup: кнопка перестаёт крутиться + показывает текст
			return c.Respond(&tele.CallbackResponse{Text: texts.T("check_payment.error_fetch")})
		}
		if payment.Status != "succeeded" {
			log.Info().Str("status", payment.Status).Msg("CheckPayment: not yet succeeded")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("check_payment.not_found")})
		}

		// Оплата подтверждена — сразу отвечаем на callback (убираем спиннер),
		// затем заменяем сообщение с кнопкой на статус "обрабатываем".
		_ = c.Respond(&tele.CallbackResponse{})
		_ = handlers.PaymentProcessing(c)

		ctx := context.Background()

		// Списываем промо-скидку (если была) — идемпотентно, безопасно при повторной проверке.
		if promo != nil {
			if code := payment.Metadata["promo_code"]; code != "" {
				if err := promo.ConsumeDiscount(ctx, c.Sender().ID, code); err != nil {
					log.Error().Err(err).Str("code", code).Msg("CheckPayment: ConsumeDiscount failed")
				}
				clearSessionPromo(c.Sender().ID)
			}
		}

		changed, err := yk.Confirm(ctx, payment.ID)
		if err != nil {
			log.Error().Err(err).Msg("CheckPayment: Confirm failed")
			return editOrFresh(c, texts.T("check_payment.error_confirm"))
		}
		if !changed {
			log.Info().Msg("CheckPayment: already confirmed — checking subscription")
			if _, subErr := sub.GetActive(ctx, c.Sender().ID); subErr == nil {
				return handlers.AlreadyProvisioned(c, sub)
			}
			// Payment confirmed but subscription never created — retry provision
			log.Info().Msg("CheckPayment: orphaned payment — retrying provision")
			_ = handlers.PaymentProcessing(c)
			return provision(c, ctx, payment.Metadata, sub)
		}

		if err := user.EnsureUser(ctx, c.Sender().ID, c.Sender().Username, c.Sender().FirstName, nil); err != nil {
			log.Error().Err(err).Msg("CheckPayment: EnsureUser failed")
			return handlers.EnsureUserError(c)
		}

		return provision(c, ctx, payment.Metadata, sub)
	}
}

// provision диспетчеризует выдачу по типу платежа из метаданных.
func provision(c tele.Context, ctx context.Context, meta map[string]string, sub *service.Subscription) error {
	log := logger.L().With().Str("func", "provision").Int64("user_id", c.Sender().ID).Logger()

	switch meta["addon_type"] {
	case "device":
		amount, _ := strconv.Atoi(meta["amount"])
		log.Info().Int("devices", amount).Msg("provision: device addon")
		if err := sub.AddDevice(ctx, c.Sender().ID, amount); err != nil {
			log.Error().Err(err).Msg("provision: AddDevice failed")
			return handlers.ProvisionError(c)
		}
		return AddonSuccess(c, amount, sub)
	}

	devices, _ := strconv.Atoi(meta["devices"])
	if devices == 0 {
		devices = 1
	}

	// Пробный период и любые случаи с явным числом дней (не months*30).
	if daysStr := meta["days"]; daysStr != "" {
		days, _ := strconv.Atoi(daysStr)
		if days <= 0 {
			days = 30
		}
		log.Info().Int("devices", devices).Int("days", days).Msg("provision: new subscription (days)")
		subURL, err := sub.ProvisionFromPaymentDays(ctx, c.Sender().ID, devices, days)
		if err != nil {
			log.Error().Err(err).Msg("provision: ProvisionFromPaymentDays failed")
			return handlers.ProvisionError(c)
		}
		log.Info().Str("sub_url", subURL).Msg("provision: ok")
		return handlers.ProvisionSuccess(c, subURL, sub)
	}

	months, _ := strconv.Atoi(meta["months"])
	if months == 0 {
		months = 1
	}
	log.Info().Int("devices", devices).Int("months", months).Msg("provision: new subscription")
	subURL, err := sub.ProvisionFromPayment(ctx, c.Sender().ID, devices, months)
	if err != nil {
		log.Error().Err(err).Msg("provision: ProvisionFromPayment failed")
		return handlers.ProvisionError(c)
	}
	log.Info().Str("sub_url", subURL).Msg("provision: ok")
	return handlers.ProvisionSuccess(c, subURL, sub)
}
