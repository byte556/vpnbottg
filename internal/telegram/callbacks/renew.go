package callbacks

import (
	"context"
	"fmt"
	"strconv"
	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/handlers"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// RenewSub продлевает активную подписку на months месяцев. Цена считается по
// текущему числу устройств (та же формула, что и в конструкторе). Сначала
// тратится реферальный баланс, остаток — через YooKassa.
func RenewSub(months int, paymentServ *service.PaymentService, userSvc *service.User, sub *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "RenewSub").Int64("user_id", c.Sender().ID).Int("months", months).Logger()
		ctx := context.Background()

		active, err := sub.GetActive(ctx, c.Sender().ID)
		if err != nil {
			log.Info().Err(err).Msg("RenewSub: no active subscription")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("renew.no_active")})
		}

		devices := active.DeviceLimit
		if devices < 1 {
			devices = 1
		}
		price := session.RenewPrice(devices, months)

		balance, _ := userSvc.GetBalance(ctx, c.Sender().ID)
		balanceUsed := balance
		if balanceUsed > int64(price) {
			balanceUsed = int64(price)
		}
		toPay := int64(price) - balanceUsed

		log.Info().Int("devices", devices).Int("price", price).Int64("balance", balance).Int64("balance_used", balanceUsed).Int64("to_pay", toPay).Msg("RenewSub: calculated")

		meta := map[string]string{
			"tg_id":   fmt.Sprintf("%d", c.Sender().ID),
			"devices": fmt.Sprintf("%d", devices),
			"months":  fmt.Sprintf("%d", months),
		}

		// Полностью покрыто балансом — продлеваем без YooKassa.
		if toPay <= 0 {
			log.Info().Msg("RenewSub: fully covered by balance")
			if _, err := userSvc.DeductBalance(ctx, c.Sender().ID, balanceUsed); err != nil {
				log.Error().Err(err).Msg("RenewSub: DeductBalance failed")
				return handlers.PaymentError(c)
			}
			_ = handlers.PaymentProcessing(c)
			return provision(c, ctx, meta, sub)
		}

		if balanceUsed > 0 {
			meta["balance_used"] = strconv.FormatInt(balanceUsed, 10)
		}

		ykPayment, _, err := paymentServ.InitiatePayment(c.Sender().ID, yookassa.CreatePaymentReq{
			AmountRub:   int(toPay),
			Description: fmt.Sprintf("Продление VPN %d устр. / %d мес", devices, months),
			SaveMethod:  true,
			Metadata:    meta,
		})
		if err != nil {
			log.Error().Err(err).Msg("RenewSub: InitiatePayment failed")
			return handlers.PaymentError(c)
		}

		sess := session.GetStore().Get(c.Sender().ID)
		sess.PaymentID = ykPayment.ID
		sess.PaymentURL = ykPayment.ConfirmationURL
		session.GetStore().Save(c.Sender().ID, sess)
		log.Info().Str("yk_id", ykPayment.ID).Int64("to_pay", toPay).Msg("RenewSub: payment created")

		return handlers.PaymentPending(c)
	}
}

// RemoveDeviceBtn уменьшает лимит устройств подписки на одно (без возврата денег).
func RemoveDeviceBtn(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "RemoveDeviceBtn").Int64("user_id", c.Sender().ID).Logger()
		newLimit, err := subSvc.RemoveDevice(context.Background(), c.Sender().ID, 1)
		if err != nil {
			log.Error().Err(err).Msg("RemoveDeviceBtn: failed")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("error.provision")})
		}
		_ = c.Respond(&tele.CallbackResponse{Text: texts.T("remove_device.done", map[string]any{"Limit": newLimit})})
		return handlers.SendSettings(c, subSvc)
	}
}
