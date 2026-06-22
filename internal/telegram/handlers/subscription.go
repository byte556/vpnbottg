package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
	"vpnbottg/internal/models"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func ProvisionSuccess(c tele.Context, subURL string, subSvc *service.Subscription) error {
	if err := c.Send(texts.T("provision.success", map[string]any{"URL": subURL})); err != nil {
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
		url, err := subSvc.GetConfigURL(context.Background(), c.Sender().ID)
		if err != nil {
			return c.Send(texts.T("error.provision"))
		}
		return c.Send(texts.T("provision.my_config", map[string]any{"URL": url}))
	}
}

func StatusSub(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		ctx := context.Background()
		sub, err := subSvc.GetActive(ctx, c.Sender().ID)
		if err != nil {
			return GuestMenu(c)
		}
		daysLeft := max(0, int(time.Until(time.Unix(sub.ExpiresAt, 0)).Hours()/24))
		expiresAt := time.Unix(sub.ExpiresAt, 0).Format("02.01.2006")
		usedGB := subSvc.GetTrafficUsedGB(ctx, c.Sender().ID)
		return c.Send(texts.T("subscription.status", map[string]any{
			"UsedGB":    fmt.Sprintf("%.1f", usedGB),
			"TrafficGB": sub.TrafficGB,
			"Devices":   sub.DeviceLimit,
			"DaysLeft":  daysLeft,
			"ExpiresAt": expiresAt,
		}))
	}
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
		)
	}

	return c.EditOrSend(
		texts.T("subscription.devices", map[string]any{
			"Used":  len(devices),
			"Limit": sub.DeviceLimit,
			"List":  formatDeviceList(devices),
		}),
		keyboard.DevicesKeyboard(devices),
	)
}

func formatDeviceList(devices []*models.DeviceConnection) string {
	lines := make([]string, 0, len(devices))
	for i, device := range devices {
		lastSeen := time.Unix(device.LastSeen, 0).Format("02.01.2006 15:04")
		lines = append(lines, fmt.Sprintf("#%d %s · %s · %s", i+1, device.Platform, shortDeviceID(device.DeviceID), lastSeen))
	}
	return strings.Join(lines, "\n")
}

func shortDeviceID(deviceID string) string {
	if len(deviceID) <= 10 {
		return deviceID
	}
	return deviceID[:10]
}
