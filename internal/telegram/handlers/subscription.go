package handlers

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
	"vpnbottg/internal/models"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	qrcode "github.com/skip2/go-qrcode"
	tele "gopkg.in/telebot.v3"
)

func ProvisionSuccess(c tele.Context, subURL string, subSvc *service.Subscription) error {
	if err := c.Send(texts.T("provision.success"), &tele.SendOptions{ParseMode: tele.ModeHTML}); err != nil {
		return err
	}
	return Menu(c, subSvc)
}

func ProvisionError(c tele.Context) error {
	return c.Send(texts.T("error.provision"))
}

func AlreadyProvisioned(c tele.Context, subSvc *service.Subscription) error {
	if err := c.Send(texts.T("subscription.already_active")); err != nil {
		return err
	}
	return Menu(c, subSvc)
}

func EnsureUserError(c tele.Context) error {
	return c.Send(texts.T("error.user_create"))
}

func MyConfig(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		ctx := context.Background()
		sub, err := subSvc.GetActive(ctx, c.Sender().ID)
		if err != nil {
			return c.Send(texts.T("error.provision"))
		}
		url, err := subSvc.GetConfigURL(ctx, c.Sender().ID)
		if err != nil {
			return c.Send(texts.T("error.provision"))
		}

		daysLeft := max(0, int(time.Until(time.Unix(sub.ExpiresAt, 0)).Hours()/24))
		usedGB := subSvc.GetTrafficUsedGB(ctx, c.Sender().ID)

		caption := texts.T("provision.my_config", map[string]any{
			"URL":       url,
			"UsedGB":    fmt.Sprintf("%.1f", usedGB),
			"TrafficGB": sub.TrafficGB,
			"Devices":   sub.DeviceLimit,
			"DaysLeft":  daysLeft,
		})

		qrBytes, err := qrcode.Encode(url, qrcode.Medium, 256)
		if err != nil {
			return c.Send(caption, &tele.SendOptions{ParseMode: tele.ModeHTML})
		}

		photo := &tele.Photo{
			File:    tele.FromReader(bytes.NewReader(qrBytes)),
			Caption: caption,
		}
		return c.Send(photo, &tele.SendOptions{ParseMode: tele.ModeHTML})
	}
}

func StatusSub(subSvc *service.Subscription) tele.HandlerFunc {
	return MyConfig(subSvc)
}

func Settings(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		return SendSettings(c, subSvc)
	}
}

func SendSettings(c tele.Context, subSvc *service.Subscription) error {
	ctx := context.Background()
	sub, err := subSvc.GetActive(ctx, c.Sender().ID)
	if err != nil {
		return GuestMenu(c)
	}
	sess := session.GetStore().Get(c.Sender().ID)
	daysLeft := max(0, int(time.Until(time.Unix(sub.ExpiresAt, 0)).Hours()/24))
	expiresAt := time.Unix(sub.ExpiresAt, 0).Format("02.01.2006")

	text := texts.T("settings.text", map[string]any{
		"TrafficGB": sub.TrafficGB,
		"Devices":   sub.DeviceLimit,
		"DaysLeft":  daysLeft,
		"ExpiresAt": expiresAt,
	})
	return c.EditOrSend(text,
		keyboard.SettingsKeyboard(sess.AddOnDevices, sess.AddonDevicesPrice(), sess.AddOnGB, sess.AddonGBPrice()),
		&tele.SendOptions{ParseMode: tele.ModeHTML},
	)
}

func Devices(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		return SendDevices(c, subSvc)
	}
}

func SendDevices(c tele.Context, subSvc *service.Subscription) error {
	ctx := context.Background()
	sub, devices, err := subSvc.ListDevices(ctx, c.Sender().ID)
	if err != nil {
		return c.Send(texts.T("error.provision"))
	}

	if len(devices) == 0 {
		return c.EditOrSend(
			texts.T("subscription.devices_empty", map[string]any{"Limit": sub.DeviceLimit}),
			keyboard.DevicesKeyboard(devices),
			&tele.SendOptions{ParseMode: tele.ModeHTML},
		)
	}

	return c.EditOrSend(
		texts.T("subscription.devices", map[string]any{
			"Used":  len(devices),
			"Limit": sub.DeviceLimit,
			"List":  formatDeviceList(devices),
		}),
		keyboard.DevicesKeyboard(devices),
		&tele.SendOptions{ParseMode: tele.ModeHTML},
	)
}

func formatDeviceList(devices []*models.DeviceConnection) string {
	lines := make([]string, 0, len(devices))
	for i, device := range devices {
		lastSeen := time.Unix(device.LastSeen, 0).Format("02.01 15:04")
		lines = append(lines, fmt.Sprintf("#%d %s · %s", i+1, device.Platform, lastSeen))
	}
	return strings.Join(lines, "\n")
}
