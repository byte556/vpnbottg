package callbacks

import (
	"context"
	"fmt"
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

func AddonGBDec(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		sess := session.GetStore().Get(c.Sender().ID)
		sess.AddOnGB -= 10
		if sess.AddOnGB < 10 {
			sess.AddOnGB = 10
		}
		session.GetStore().Save(c.Sender().ID, sess)
		return handlers.SendSettings(c, subSvc)
	}
}

func AddonGBInc(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		sess := session.GetStore().Get(c.Sender().ID)
		sess.AddOnGB += 10
		session.GetStore().Save(c.Sender().ID, sess)
		return handlers.SendSettings(c, subSvc)
	}
}

func BuyAddonDevice(paymentServ *service.PaymentService, userSvc *service.User) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "BuyAddonDevice").Int64("user_id", c.Sender().ID).Logger()

		sess := session.GetStore().Get(c.Sender().ID)
		if err := userSvc.EnsureUser(context.Background(), c.Sender().ID, c.Sender().Username, c.Sender().FirstName, nil); err != nil {
			log.Error().Err(err).Msg("BuyAddonDevice: EnsureUser failed")
			return handlers.PaymentError(c)
		}

		price := sess.AddonDevicesPrice()
		log.Info().Int("devices", sess.AddOnDevices).Int("price", price).Msg("BuyAddonDevice: initiating payment")

		ykPayment, _, err := paymentServ.InitiatePayment(c.Sender().ID, yookassa.CreatePaymentReq{
			AmountRub:   price,
			Description: fmt.Sprintf("Добавление %d устр.", sess.AddOnDevices),
			Metadata: map[string]string{
				"tg_id":      fmt.Sprintf("%d", c.Sender().ID),
				"addon_type": "device",
				"amount":     fmt.Sprintf("%d", sess.AddOnDevices),
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("BuyAddonDevice: InitiatePayment failed")
			return handlers.PaymentError(c)
		}

		sess.PaymentID = ykPayment.ID
		sess.PaymentURL = ykPayment.ConfirmationURL
		session.GetStore().Save(c.Sender().ID, sess)
		log.Info().Str("yk_id", ykPayment.ID).Msg("BuyAddonDevice: payment created")

		return handlers.PaymentPending(c)
	}
}

func BuyAddonGB(paymentServ *service.PaymentService, userSvc *service.User) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "BuyAddonGB").Int64("user_id", c.Sender().ID).Logger()

		sess := session.GetStore().Get(c.Sender().ID)
		if err := userSvc.EnsureUser(context.Background(), c.Sender().ID, c.Sender().Username, c.Sender().FirstName, nil); err != nil {
			log.Error().Err(err).Msg("BuyAddonGB: EnsureUser failed")
			return handlers.PaymentError(c)
		}

		price := sess.AddonGBPrice()
		log.Info().Int("gb", sess.AddOnGB).Int("price", price).Msg("BuyAddonGB: initiating payment")

		ykPayment, _, err := paymentServ.InitiatePayment(c.Sender().ID, yookassa.CreatePaymentReq{
			AmountRub:   price,
			Description: fmt.Sprintf("Добавление %d ГБ трафика", sess.AddOnGB),
			Metadata: map[string]string{
				"tg_id":      fmt.Sprintf("%d", c.Sender().ID),
				"addon_type": "gb",
				"amount":     fmt.Sprintf("%d", sess.AddOnGB),
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("BuyAddonGB: InitiatePayment failed")
			return handlers.PaymentError(c)
		}

		sess.PaymentID = ykPayment.ID
		sess.PaymentURL = ykPayment.ConfirmationURL
		session.GetStore().Save(c.Sender().ID, sess)
		log.Info().Str("yk_id", ykPayment.ID).Msg("BuyAddonGB: payment created")

		return handlers.PaymentPending(c)
	}
}

func AddonSuccess(c tele.Context, amount int, subSvc *service.Subscription) error {
	if err := c.Send(texts.T("addon.device_success", map[string]any{"Devices": amount})); err != nil {
		return err
	}
	return handlers.Menu(c, subSvc)
}

func AddonGBSuccess(c tele.Context, gb int, subSvc *service.Subscription) error {
	if err := c.Send(texts.T("addon.gb_success", map[string]any{"GB": gb})); err != nil {
		return err
	}
	return handlers.Menu(c, subSvc)
}

func BackToMenu(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		return handlers.Menu(c, subSvc)
	}
}
