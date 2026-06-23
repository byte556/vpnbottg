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

	var pricingLine string
	if cs.GetMonths() == 1 {
		pricingLine = fmt.Sprintf("💳 %d ₽  (~%d ₽/день)", cs.CalcPrice(), cs.CalcPricePerDay())
	} else {
		pricingLine = fmt.Sprintf("💳 %d ₽/мес → итого %d ₽  (~%d ₽/день)",
			cs.CalcPricePerMonth(), cs.CalcPrice(), cs.CalcPricePerDay())
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

	return editOrFresh(c, text, keyboard.Constructor(&sess.Constructor))
}
