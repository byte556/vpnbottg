package handlers

import (
	"context"
	"fmt"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// Renew — точка входа в режим продления: берёт устройства из активной подписки,
// фиксирует их и показывает экран выбора срока. Если активной подписки нет —
// продлевать нечего, откатываемся на полный конструктор.
func Renew(c tele.Context, subSvc *service.Subscription) error {
	ctx := context.Background()
	sub, err := subSvc.GetActive(ctx, c.Sender().ID)
	if err != nil {
		return Constructor(c)
	}

	sess := session.GetStore().Get(c.Sender().ID)
	cs := &sess.Constructor
	// Устройства фиксируются по текущей подписке — их нельзя менять при продлении.
	cs.SetDevices(sub.DeviceLimit)
	if cs.GetMonths() <= 0 {
		cs.SetMonths(1)
	}
	sess.RenewMode = true
	// Продление идёт чистой ценой — сбрасываем промо и прошлый платёж.
	sess.PromoCode = ""
	sess.PromoDiscountPct = 0
	sess.PaymentID = ""
	sess.PaymentURL = ""
	session.GetStore().Save(c.Sender().ID, sess)

	return RenewRender(c)
}

// RenewRender перерисовывает экран продления из текущей сессии (без обращения к БД) —
// вызывается при смене срока кнопками 1/3/6/12 мес.
func RenewRender(c tele.Context) error {
	sess := session.GetStore().Get(c.Sender().ID)
	cs := &sess.Constructor

	base := cs.CalcPrice()

	text := texts.T("renew.text", map[string]any{
		"Devices":     cs.GetDevices(),
		"Months":      cs.GetMonths(),
		"PricingLine": renewPricingLine(cs, base),
	})

	return screen(c, "settings", text, keyboard.RenewConstructor(cs, base),
		&tele.SendOptions{ParseMode: tele.ModeHTML})
}

func renewPricingLine(cs *session.ConstructorState, base int) string {
	switch cs.GetMonths() {
	case 1:
		return fmt.Sprintf("💳 %d ₽  (~%d ₽/день)", base, cs.CalcPricePerDay())
	default:
		return fmt.Sprintf("💳 %d ₽/мес → итого %d ₽  (~%d ₽/день)",
			cs.CalcPricePerMonth(), base, cs.CalcPricePerDay())
	}
}

// ChangePlan выходит из режима продления в полный конструктор (выбор устройств + срока).
func ChangePlan(c tele.Context) error {
	sess := session.GetStore().Get(c.Sender().ID)
	sess.RenewMode = false
	session.GetStore().Save(c.Sender().ID, sess)
	return Constructor(c)
}
