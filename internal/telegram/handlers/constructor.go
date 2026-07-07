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

var userSvc *service.User

func Init(u *service.User) {
	userSvc = u
}

func Constructor(c tele.Context) error {
	sess := session.GetStore().Get(c.Sender().ID)
	cs := &sess.Constructor

	base := cs.CalcPrice()
	final := sess.ApplyDiscount(base)

	var balance int64
	if userSvc != nil {
		balance, _ = userSvc.GetBalance(context.Background(), c.Sender().ID)
	}

	var pricingLine string
	switch {
	case sess.PromoDiscountPct > 0:
		pricingLine = fmt.Sprintf("💳 <s>%d ₽</s> → <b>%d ₽</b>  (−%d%% по промокоду %s)",
			base, final, sess.PromoDiscountPct, sess.PromoCode)
	case cs.GetMonths() == 1:
		pricingLine = fmt.Sprintf("💳 %d ₽  (~%d ₽/день)", final, cs.CalcPricePerDay())
	default:
		pricingLine = fmt.Sprintf("💳 %d ₽/мес → итого %d ₽  (~%d ₽/день)",
			cs.CalcPricePerMonth(), final, cs.CalcPricePerDay())
	}

	savingsLine := ""
	if savings := cs.CalcSavings(); savings > 0 {
		savingsLine = fmt.Sprintf("\n💰 Экономия %d ₽ по сравнению с помесячной оплатой", savings)
	}

	balanceLine := ""
	if balance > 0 {
		balanceLine = "\n" + texts.T("constructor.balance_line", map[string]any{"Balance": balance})
	}

	toPay := final
	if balance > 0 {
		toPay = int(max(0, int64(final)-balance))
	}

	text := texts.T("constructor.text", map[string]any{
		"Devices":     cs.GetDevices(),
		"Months":      cs.GetMonths(),
		"PricingLine": pricingLine,
		"SavingsLine": savingsLine,
		"BalanceLine": balanceLine,
	})

	return screen(c, "buy", text, keyboard.Constructor(&sess.Constructor, toPay),
		&tele.SendOptions{ParseMode: tele.ModeHTML})
}
