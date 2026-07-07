package callbacks

import (
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/handlers"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// AddonSuccess — экран успешного добавления устройств. Используется провижном
// для legacy-платежей с addon_type=device (созданных до перехода на единый
// конструктор управления тарифом).
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
