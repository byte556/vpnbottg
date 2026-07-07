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

func AddonDevDec(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		sess := session.GetStore().Get(c.Sender().ID)
		sess.AddOnDevices--
		if sess.AddOnDevices < 1 {
			sess.AddOnDevices = 1
		}
		session.GetStore().Save(c.Sender().ID, sess)
		return handlers.SendSettings(c, subSvc)
	}
}

func AddonDevInc(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		sess := session.GetStore().Get(c.Sender().ID)
		sess.AddOnDevices++
		session.GetStore().Save(c.Sender().ID, sess)
		return handlers.SendSettings(c, subSvc)
	}
}

func BuyAddonDevice(paymentServ *service.PaymentService, userSvc *service.User, subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "BuyAddonDevice").Int64("user_id", c.Sender().ID).Logger()
		ctx := context.Background()

		sess := session.GetStore().Get(c.Sender().ID)
		if err := userSvc.EnsureUser(ctx, c.Sender().ID, c.Sender().Username, c.Sender().FirstName, nil); err != nil {
			log.Error().Err(err).Msg("BuyAddonDevice: EnsureUser failed")
			return handlers.PaymentError(c)
		}

		price := sess.AddonDevicesPrice()

		balance, _ := userSvc.GetBalance(ctx, c.Sender().ID)
		balanceUsed := balance
		if balanceUsed > int64(price) {
			balanceUsed = int64(price)
		}
		toPay := int64(price) - balanceUsed

		log.Info().Int("devices", sess.AddOnDevices).Int("price", price).Int64("balance", balance).Int64("balance_used", balanceUsed).Int64("to_pay", toPay).Msg("BuyAddonDevice: calculated")

		if toPay <= 0 {
			log.Info().Msg("BuyAddonDevice: fully covered by balance")
			if _, err := userSvc.DeductBalance(ctx, c.Sender().ID, balanceUsed); err != nil {
				log.Error().Err(err).Msg("BuyAddonDevice: DeductBalance failed")
				return handlers.PaymentError(c)
			}
			_ = handlers.PaymentProcessing(c)
			if err := subSvc.AddDevice(ctx, c.Sender().ID, sess.AddOnDevices); err != nil {
				log.Error().Err(err).Msg("BuyAddonDevice: AddDevice failed")
				return handlers.ProvisionError(c)
			}
			return AddonSuccess(c, sess.AddOnDevices, subSvc)
		}

		metadata := map[string]string{
			"tg_id":      fmt.Sprintf("%d", c.Sender().ID),
			"addon_type": "device",
			"amount":     fmt.Sprintf("%d", sess.AddOnDevices),
		}
		if balanceUsed > 0 {
			metadata["balance_used"] = strconv.FormatInt(balanceUsed, 10)
		}

		ykPayment, _, err := paymentServ.InitiatePayment(c.Sender().ID, yookassa.CreatePaymentReq{
			AmountRub:   int(toPay),
			Description: fmt.Sprintf("Добавление %d устр.", sess.AddOnDevices),
			Metadata:    metadata,
		})
		if err != nil {
			log.Error().Err(err).Msg("BuyAddonDevice: InitiatePayment failed")
			return handlers.PaymentError(c)
		}

		sess.PaymentID = ykPayment.ID
		sess.PaymentURL = ykPayment.ConfirmationURL
		session.GetStore().Save(c.Sender().ID, sess)
		log.Info().Str("yk_id", ykPayment.ID).Int64("to_pay", toPay).Msg("BuyAddonDevice: payment created")

		return handlers.PaymentPending(c)
	}
}

func AddonSuccess(c tele.Context, amount int, subSvc *service.Subscription) error {
	if err := editOrFresh(c, texts.T("addon.device_success", map[string]any{"Devices": amount})); err != nil {
		return err
	}
	return handlers.Menu(c, subSvc)
}

func BackToMenu(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		// Сбрасываем ожидание ввода промокода, если пользователь передумал.
		sess := session.GetStore().Get(c.Sender().ID)
		if sess.AwaitPromo {
			sess.AwaitPromo = false
			session.GetStore().Save(c.Sender().ID, sess)
		}
		return handlers.Menu(c, subSvc)
	}
}

func ShowConfigCallback(subSvc *service.Subscription) tele.HandlerFunc {
	return handlers.MyConfig(subSvc)
}
