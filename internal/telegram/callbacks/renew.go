package callbacks

import (
	"context"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/handlers"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// OpenRenew открывает конструктор тарифа для продления активной подписки,
// предзаполняя его текущим числом устройств. Дальше пользователь как обычно
// выбирает срок/устройства и оплачивает — оплата продлевает подписку и
// приводит лимит устройств к выбранному значению.
func OpenRenew(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "OpenRenew").Int64("user_id", c.Sender().ID).Logger()
		ctx := context.Background()

		active, err := subSvc.GetActive(ctx, c.Sender().ID)
		if err != nil {
			log.Info().Err(err).Msg("OpenRenew: no active subscription")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("renew.no_active")})
		}

		devices := active.DeviceLimit
		if devices < 1 {
			devices = 1
		}

		sess := session.GetStore().Get(c.Sender().ID)
		sess.PaymentID = ""
		sess.Constructor.SetDevices(devices)
		sess.Constructor.SetMonths(1)
		session.GetStore().Save(c.Sender().ID, sess)

		return handlers.Constructor(c)
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
