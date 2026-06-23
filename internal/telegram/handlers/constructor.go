package handlers

import (
	"fmt"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func Constructor(c tele.Context) error {
	sess := session.GetStore().Get(c.Sender().ID)
	cs := &sess.Constructor

	base := cs.CalcPrice()
	final := sess.ApplyDiscount(base)

	var pricingLine string
	switch {
	case sess.PromoDiscountPct > 0:
		// С активной промо-скидкой показываем зачёркнутую исходную и итоговую цену.
		pricingLine = fmt.Sprintf("💳 <s>%d ₽</s> → <b>%d ₽</b>  (−%d%% по промокоду %s)",
			base, final, sess.PromoDiscountPct, sess.PromoCode)
	case cs.GetMonths() == 1:
		pricingLine = fmt.Sprintf("💳 %d ₽  (~%d ₽/день)", base, cs.CalcPricePerDay())
	default:
		pricingLine = fmt.Sprintf("💳 %d ₽/мес → итого %d ₽  (~%d ₽/день)",
			cs.CalcPricePerMonth(), base, cs.CalcPricePerDay())
	}

	savingsLine := ""
	if savings := cs.CalcSavings(); savings > 0 {
		savingsLine = fmt.Sprintf("\n💰 Экономия %d ₽ по сравнению с помесячной оплатой", savings)
	}

	text := texts.T("constructor.text", map[string]any{
		"GB":          cs.GetGB(),
		"Devices":     cs.GetDevices(),
		"Months":      cs.GetMonths(),
		"PricingLine": pricingLine,
		"SavingsLine": savingsLine,
	})

	return editOrFresh(c, text, keyboard.Constructor(&sess.Constructor, final),
		&tele.SendOptions{ParseMode: tele.ModeHTML})
}
