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
	if err := screen(c, "success",
		texts.T("provision.success"),
		keyboard.ProvisionSuccessKeyboard(),
		&tele.SendOptions{ParseMode: tele.ModeHTML},
	); err != nil {
		return err
	}
	return Menu(c, subSvc)
}

func ProvisionError(c tele.Context) error {
	return editOrFresh(c, texts.T("error.provision"))
}

func AlreadyProvisioned(c tele.Context, subSvc *service.Subscription) error {
	if err := editOrFresh(c, texts.T("subscription.already_active")); err != nil {
		return err
	}
	return Menu(c, subSvc)
}

func EnsureUserError(c tele.Context) error {
	return editOrFresh(c, texts.T("error.user_create"))
}

func MyConfig(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		ctx := context.Background()
		sub, err := subSvc.GetActive(ctx, c.Sender().ID)
		if err != nil {
			return editOrFresh(c, texts.T("error.provision"))
		}
		url, err := subSvc.GetConfigURL(ctx, c.Sender().ID)
		if err != nil {
			return editOrFresh(c, texts.T("error.provision"))
		}

		daysLeft := max(0, int(time.Until(time.Unix(sub.ExpiresAt, 0)).Hours()/24))
		usedGB := subSvc.GetTrafficUsedGB(ctx, c.Sender().ID)

		caption := texts.T("provision.my_config", map[string]any{
			"URL":      url,
			"UsedGB":   fmt.Sprintf("%.1f", usedGB),
			"Devices":  sub.DeviceLimit,
			"DaysLeft": daysLeft,
		})

		// Экран конфига — фото (QR). Его нельзя получить редактированием текстового
		// сообщения меню, поэтому удаляем исходное сообщение и шлём фото заново.
		if c.Callback() != nil {
			_ = c.Delete()
		}

		kb := keyboard.MyConfigKeyboard(url)

		qrBytes, err := qrcode.Encode(url, qrcode.Medium, 256)
		if err != nil {
			return editOrFresh(c, caption, kb, &tele.SendOptions{ParseMode: tele.ModeHTML})
		}

		photo := &tele.Photo{
			File:    tele.FromReader(bytes.NewReader(qrBytes)),
			Caption: caption,
		}
		// SendOptions передаём первым — иначе telebot затирает клавиатуру (см. sendOptsFirst).
		return c.Send(photo, &tele.SendOptions{ParseMode: tele.ModeHTML}, kb)
	}
}

func StatusSub(subSvc *service.Subscription) tele.HandlerFunc {
	return MyConfig(subSvc)
}

// Settings открывает конструктор в режиме управления активной подпиской
// («⚙️ Мой тариф»): предзаполняет текущим числом устройств, фиксирует остаток
// дней для пропорциональной доплаты за устройства. Отдельной страницы-настроек
// больше нет — всё управление тарифом в одном конструкторе.
func Settings(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		return SendSettings(c, subSvc)
	}
}

func SendSettings(c tele.Context, subSvc *service.Subscription) error {
	ctx := context.Background()
	sess := session.GetStore().Get(c.Sender().ID)
	sess.PaymentID = ""

	sub, err := subSvc.GetActive(ctx, c.Sender().ID)
	if err != nil {
		return GuestMenu(c)
	}

	daysLeft := max(0, int(time.Until(time.Unix(sub.ExpiresAt, 0)).Hours()/24))
	devices := sub.DeviceLimit
	if devices < 1 {
		devices = 1
	}

	sess.Constructor.SetManage(devices, daysLeft)
	sess.Constructor.SetDevices(devices)
	sess.Constructor.SetMonths(0) // по умолчанию не продлеваем — только меняем устройства
	session.GetStore().Save(c.Sender().ID, sess)

	return Constructor(c)
}

// StartBuy открывает конструктор в режиме новой покупки (сбрасывает режим
// управления подпиской). Используется кнопкой «Купить VPN» и командой.
func StartBuy(c tele.Context) error {
	sess := session.GetStore().Get(c.Sender().ID)
	sess.PaymentID = ""
	sess.Constructor.ResetManage()
	sess.Constructor.SetMonths(1)
	session.GetStore().Save(c.Sender().ID, sess)
	return Constructor(c)
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
		return editOrFresh(c, texts.T("error.provision"))
	}

	slotBar := progressBar(len(devices), sub.DeviceLimit, 10)

	if len(devices) == 0 {
		return screen(c, "devices",
			texts.T("subscription.devices_empty", map[string]any{
				"Limit":   sub.DeviceLimit,
				"SlotBar": slotBar,
			}),
			keyboard.DevicesKeyboard(devices),
			&tele.SendOptions{ParseMode: tele.ModeHTML},
		)
	}

	return screen(c, "devices",
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
