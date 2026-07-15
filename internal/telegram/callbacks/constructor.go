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

	"github.com/rs/zerolog"
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
		return handlers.Constructor(c)
	}
}

func Buy(paymentServ *service.PaymentService, userSvc *service.User, sub *service.Subscription, promo *service.PromoService) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "Buy").Int64("user_id", c.Sender().ID).Logger()
		ctx := context.Background()

		sess := session.GetStore().Get(c.Sender().ID)
		if sess.PaymentID != "" {
			log.Debug().Str("payment_id", sess.PaymentID).Msg("Buy: payment already pending")
			return handlers.PaymentPending(c)
		}

		if err := userSvc.EnsureUser(ctx, c.Sender().ID, c.Sender().Username, c.Sender().FirstName, nil); err != nil {
			log.Error().Err(err).Msg("Buy: EnsureUser failed")
			return handlers.PaymentError(c)
		}

		cs := &sess.Constructor
		devices := cs.GetDevices()
		months := cs.GetMonths()

		// Режим управления: если ничего не меняется (не продлеваем и число
		// устройств прежнее) — платить/применять нечего.
		if cs.IsManage() && months == 0 && devices == cs.BaseDevices() {
			return c.Respond(&tele.CallbackResponse{Text: texts.T("manage.no_changes")})
		}

		price := sess.ApplyDiscount(cs.CalcPrice())

		balance, _ := userSvc.GetBalance(ctx, c.Sender().ID)
		balanceUsed := balance
		if balanceUsed > int64(price) {
			balanceUsed = int64(price)
		}
		toPay := int64(price) - balanceUsed

		var description string
		if months > 0 {
			description = fmt.Sprintf("VPN %d подкл. / %d мес", devices, months)
		} else {
			description = fmt.Sprintf("VPN %d подкл. (изменение тарифа)", devices)
		}
		log.Info().Bool("manage", cs.IsManage()).Int("devices", devices).Int("months", months).Int("price", price).Int64("balance", balance).Int64("balance_used", balanceUsed).Int64("to_pay", toPay).Msg("Buy: calculated")

		// Полностью покрыто балансом (или бесплатное изменение) — выдаём без YooKassa.
		if toPay <= 0 {
			log.Info().Msg("Buy: fully covered by balance / free change")
			if balanceUsed > 0 {
				if _, err := userSvc.DeductBalance(ctx, c.Sender().ID, balanceUsed); err != nil {
					log.Error().Err(err).Msg("Buy: DeductBalance failed")
					return handlers.PaymentError(c)
				}
			}

			if promo != nil && sess.PromoCode != "" {
				if err := promo.ConsumeDiscount(ctx, c.Sender().ID, sess.PromoCode); err != nil {
					log.Error().Err(err).Msg("Buy: ConsumeDiscount failed")
				}
				clearSessionPromo(c.Sender().ID)
			}

			_ = handlers.PaymentProcessing(c)

			meta := map[string]string{
				"tg_id":   fmt.Sprintf("%d", c.Sender().ID),
				"devices": fmt.Sprintf("%d", devices),
				"months":  fmt.Sprintf("%d", months),
			}
			return provision(c, ctx, meta, sub)
		}

		// Частичная или нулевая скидка балансом — создаём YK-платёж на остаток.
		metadata := map[string]string{
			"tg_id":      fmt.Sprintf("%d", c.Sender().ID),
			"devices":    fmt.Sprintf("%d", devices),
			"months":     fmt.Sprintf("%d", months),
			"promo_code": sess.PromoCode,
		}
		if balanceUsed > 0 {
			metadata["balance_used"] = strconv.FormatInt(balanceUsed, 10)
		}

		ykPayment, _, err := paymentServ.InitiatePayment(c.Sender().ID, yookassa.CreatePaymentReq{
			AmountRub:   int(toPay),
			Description: description,
			SaveMethod:  true,
			Metadata:    metadata,
		})
		if err != nil {
			log.Error().Err(err).Msg("Buy: InitiatePayment failed")
			return handlers.PaymentError(c)
		}

		sess.PaymentID = ykPayment.ID
		sess.PaymentURL = ykPayment.ConfirmationURL
		session.GetStore().Save(c.Sender().ID, sess)
		log.Info().Str("yk_id", ykPayment.ID).Int64("to_pay", toPay).Msg("Buy: payment created")

		return handlers.PaymentPending(c)
	}
}

func CheckPayment(yk *service.PaymentService, sub *service.Subscription, user *service.User, promo *service.PromoService, ref *service.ReferralService) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "CheckPayment").Int64("user_id", c.Sender().ID).Logger()

		paymentID := c.Data()
		log.Info().Str("payment_id", paymentID).Msg("CheckPayment: fetching")

		payment, err := yk.GetYkClient().FetchPayment(paymentID)
		if err != nil {
			log.Error().Err(err).Str("payment_id", paymentID).Msg("CheckPayment: FetchPayment failed")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("check_payment.error_fetch")})
		}
		if payment.Status != "succeeded" {
			log.Info().Str("status", payment.Status).Msg("CheckPayment: not yet succeeded")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("check_payment.not_found")})
		}

		_ = c.Respond(&tele.CallbackResponse{})
		_ = handlers.PaymentProcessing(c)

		ctx := context.Background()

		// Confirm ПЕРВЫМ — он идемпотентен (changed=true только при первом подтверждении).
		// Списание баланса/промо и реферальный кешбэк делаем строго при changed==true,
		// иначе webhook/поллер и ручная проверка обработали бы платёж дважды
		// (двойное списание баланса, повторное начисление кешбэка).
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
			log.Info().Msg("CheckPayment: orphaned payment — retrying provision")
			_ = handlers.PaymentProcessing(c)
			return provision(c, ctx, payment.Metadata, sub)
		}

		if err := user.EnsureUser(ctx, c.Sender().ID, c.Sender().Username, c.Sender().FirstName, nil); err != nil {
			log.Error().Err(err).Msg("CheckPayment: EnsureUser failed")
			return handlers.EnsureUserError(c)
		}

		// Списываем промо-скидку (если была).
		if promo != nil {
			if code := payment.Metadata["promo_code"]; code != "" {
				if err := promo.ConsumeDiscount(ctx, c.Sender().ID, code); err != nil {
					log.Error().Err(err).Str("code", code).Msg("CheckPayment: ConsumeDiscount failed")
				}
				clearSessionPromo(c.Sender().ID)
			}
		}

		// Списываем баланс (если покупатель использовал бонусы).
		if balStr := payment.Metadata["balance_used"]; balStr != "" {
			if bal, _ := strconv.ParseInt(balStr, 10, 64); bal > 0 {
				if deducted, err := user.DeductBalance(ctx, c.Sender().ID, bal); err != nil {
					log.Error().Err(err).Int64("balance_used", bal).Msg("CheckPayment: DeductBalance failed")
				} else {
					log.Info().Int64("deducted", deducted).Msg("CheckPayment: balance deducted")
				}
			}
		}

		// Реферальный кешбэк — от суммы реальной оплаты (не от баланса).
		if ref != nil {
			if dbPayment, err := yk.GetPaymentByProviderID(ctx, payment.ID); err != nil {
				log.Error().Err(err).Msg("CheckPayment: GetPaymentByProviderID for referral failed")
			} else {
				rewardReferrerTG(c, ctx, ref, user, c.Sender().ID, dbPayment.Amount, log)
			}
		}

		return provision(c, ctx, payment.Metadata, sub)
	}
}

// rewardReferrerTG начисляет реферреру кешбэк за оплату реферала и уведомляет его.
// Используется в путях подтверждения, где есть tele.Context (ручная проверка оплаты).
func rewardReferrerTG(c tele.Context, ctx context.Context, ref *service.ReferralService, user *service.User, refereeID, paymentRub int64, log zerolog.Logger) {
	referrerID, rewardRub, err := ref.RewardBalance(ctx, refereeID, paymentRub)
	if err != nil {
		log.Error().Err(err).Msg("rewardReferrerTG: failed")
		return
	}
	if referrerID == 0 {
		return
	}
	refUser, _ := user.GetUser(ctx, refereeID)
	name := "Друг"
	if refUser != nil {
		if refUser.Username != "" {
			name = "@" + refUser.Username
		} else if refUser.FirstName != "" {
			name = refUser.FirstName
		}
	}
	text := texts.T("referral.reward", map[string]any{"Name": name, "Amount": rewardRub})
	if _, err := c.Bot().Send(&tele.User{ID: referrerID}, text, &tele.SendOptions{ParseMode: tele.ModeHTML}); err != nil {
		log.Error().Err(err).Int64("referrer_id", referrerID).Msg("rewardReferrerTG: notify failed")
	}
	log.Info().Int64("referrer_id", referrerID).Int64("reward_rub", rewardRub).Msg("rewardReferrerTG: notified")
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
	// months=0 — управление активной подпиской без продления (только изменение
	// числа устройств). ProvisionFromPayment в этом случае срок не трогает.
	log.Info().Int("devices", devices).Int("months", months).Msg("provision: subscription")
	subURL, err := sub.ProvisionFromPayment(ctx, c.Sender().ID, devices, months)
	if err != nil {
		log.Error().Err(err).Msg("provision: ProvisionFromPayment failed")
		return handlers.ProvisionError(c)
	}
	log.Info().Str("sub_url", subURL).Msg("provision: ok")
	return handlers.ProvisionSuccess(c, subURL, sub)
}
