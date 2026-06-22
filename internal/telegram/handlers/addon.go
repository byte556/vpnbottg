package handlers

import (
	"context"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func AddDeviceScreen(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		sess := session.GetStore().Get(c.Sender().ID)

		currentDevices := 0
		if sub, err := subSvc.GetActive(context.Background(), c.Sender().ID); err == nil {
			currentDevices = sub.DeviceLimit
		}

		text := texts.T("addon.device", map[string]any{
			"AddDevices":     sess.AddOnDevices,
			"Price":          sess.AddonDevicesPrice(),
			"CurrentDevices": currentDevices,
			"NewDevices":     currentDevices + sess.AddOnDevices,
		})
		return c.EditOrSend(text, keyboard.AddDeviceKeyboard(sess.AddOnDevices, sess.AddonDevicesPrice()))
	}
}
