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

var circleNums = []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩"}

func progressBar(used, total, width int) string {
	if total <= 0 || width <= 0 {
		return strings.Repeat("░", width)
	}
	filled := used * width / total
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("▓", filled) + strings.Repeat("░", width-filled)
}

func platformEmoji(platform string) string {
	switch strings.ToLower(platform) {
	case "ios", "iphone", "ipad":
		return "🍎"
	case "android":
		return "🤖"
	case "windows":
		return "🖥"
	case "macos", "mac", "osx":
		return "💻"
	case "linux":
		return "🐧"
	default:
		return "📱"
	}
}

func lastSeenIndicator(lastSeen int64) string {
	ago := time.Since(time.Unix(lastSeen, 0))
	switch {
	case ago < 30*time.Minute:
		return "🟢"
	case ago < 24*time.Hour:
		return "🟡"
	default:
		return "⚪"
	}
}

func circleNum(i int) string {
	if i >= 0 && i < len(circleNums) {
		return circleNums[i]
	}
	return fmt.Sprintf("%d.", i+1)
}

func ProvisionSuccess(c tele.Context, subURL string, subSvc *service.Subscription) error {
	if err := c.Send(
		texts.T("provision.success"),
		keyboard.ProvisionSuccessKeyboard(),
		&tele.SendOptions{ParseMode: tele.ModeHTML},
	); err != nil {
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

		// Экран конфига — фото (QR). Его нельзя получить редактированием текстового
		// сообщения меню, поэтому удаляем исходное сообщение и шлём фото заново.
		if c.Callback() != nil {
			_ = c.Delete()
		}

		qrBytes, err := qrcode.Encode(url, qrcode.Medium, 256)
		if err != nil {
			return c.Send(caption, keyboard.MyConfigKeyboard(), &tele.SendOptions{ParseMode: tele.ModeHTML})
		}

		photo := &tele.Photo{
			File:    tele.FromReader(bytes.NewReader(qrBytes)),
			Caption: caption,
		}
		return c.Send(photo, keyboard.MyConfigKeyboard(), &tele.SendOptions{ParseMode: tele.ModeHTML})
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

	usedGBFloat := subSvc.GetTrafficUsedGB(ctx, c.Sender().ID)
	usedGBInt := int(usedGBFloat)
	trafficBar := progressBar(usedGBInt, sub.TrafficGB, 10)

	totalDays := 30
	if sub.StartedAt > 0 && sub.ExpiresAt > sub.StartedAt {
		totalDays = int(time.Unix(sub.ExpiresAt, 0).Sub(time.Unix(sub.StartedAt, 0)).Hours() / 24)
		if totalDays < 1 {
			totalDays = 1
		}
	}
	daysBar := progressBar(daysLeft, totalDays, 10)

	text := texts.T("settings.text", map[string]any{
		"TrafficGB":  sub.TrafficGB,
		"Devices":    sub.DeviceLimit,
		"DaysLeft":   daysLeft,
		"ExpiresAt":  expiresAt,
		"UsedGB":     fmt.Sprintf("%.1f", usedGBFloat),
		"TrafficBar": trafficBar,
		"DaysBar":    daysBar,
	})
	return editOrFresh(c, text,
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

	slotBar := progressBar(len(devices), sub.DeviceLimit, 10)

	if len(devices) == 0 {
		return editOrFresh(c,
			texts.T("subscription.devices_empty", map[string]any{
				"Limit":   sub.DeviceLimit,
				"SlotBar": slotBar,
			}),
			keyboard.DevicesKeyboard(devices),
			&tele.SendOptions{ParseMode: tele.ModeHTML},
		)
	}

	return editOrFresh(c,
		texts.T("subscription.devices", map[string]any{
			"Used":    len(devices),
			"Limit":   sub.DeviceLimit,
			"List":    formatDeviceList(devices),
			"SlotBar": slotBar,
		}),
		keyboard.DevicesKeyboard(devices),
		&tele.SendOptions{ParseMode: tele.ModeHTML},
	)
}

func formatDeviceList(devices []*models.DeviceConnection) string {
	lines := make([]string, 0, len(devices))
	for i, d := range devices {
		platform := d.Platform
		if platform == "" {
			platform = "Неизвестно"
		}
		lastSeen := time.Unix(d.LastSeen, 0).Format("02.01 15:04")
		lines = append(lines, fmt.Sprintf(
			"%s %s %s  ·  %s %s",
			circleNum(i), platformEmoji(d.Platform), platform,
			lastSeenIndicator(d.LastSeen), lastSeen,
		))
	}
	return strings.Join(lines, "\n")
}
