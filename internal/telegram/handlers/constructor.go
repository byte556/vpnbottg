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

	toPay := final
	if balance > 0 {
		toPay = int(max(0, int64(final)-balance))
	}

	balanceLine := ""
	if balance > 0 && final > 0 {
		balanceLine = "\n" + texts.T("constructor.balance_line", map[string]any{"Balance": balance})
	}

	// Режим управления активной подпиской — «Мой тариф».
	if cs.IsManage() {
		text := manageText(cs, final, balanceLine)
		return screen(c, "buy", text, keyboard.Constructor(cs, toPay),
			&tele.SendOptions{ParseMode: tele.ModeHTML})
	}

	// Режим новой покупки.
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

	text := texts.T("constructor.text", map[string]any{
		"Devices":     cs.GetDevices(),
		"Months":      cs.GetMonths(),
		"PricingLine": pricingLine,
		"SavingsLine": savingsLine,
		"BalanceLine": balanceLine,
	})

	return screen(c, "buy", text, keyboard.Constructor(cs, toPay),
		&tele.SendOptions{ParseMode: tele.ModeHTML})
}

// manageText — экран управления активной подпиской: текущий статус + что
// изменится (устройства/срок) и итоговая цена. Пропорциональная доплата за
// добавленные устройства считается по оставшимся дням.
func manageText(cs *session.ConstructorState, final int, balanceLine string) string {
	var priceLine string
	switch {
	case final <= 0 && cs.GetDevices() < cs.BaseDevices():
		priceLine = "🗑 Уменьшение устройств — бесплатно (без возврата)"
	case final <= 0:
		priceLine = "Изменений к оплате нет"
	default:
		// final (=CalcPrice) — источник истины для суммы к оплате. Разбивку строим
		// так, чтобы слагаемые всегда давали в сумме final (без расхождения ±1₽
		// от независимого округления): продление показываем как есть, апгрейд —
		// как остаток.
		var parts string
		rp := cs.RenewPrice()
		if rp > final {
			rp = final
		}
		if rp > 0 {
			parts += fmt.Sprintf("• продление %d мес: %d ₽\n", cs.GetMonths(), rp)
		}
		if up := final - rp; up > 0 && cs.GetDevices() > cs.BaseDevices() {
			parts += fmt.Sprintf("• +%d устр. на %d дн.: %d ₽\n", cs.GetDevices()-cs.BaseDevices(), cs.DaysLeft(), up)
		}
		priceLine = parts + fmt.Sprintf("💳 <b>Итого: %d ₽</b>", final)
	}

	return texts.T("constructor.manage_text", map[string]any{
		"BaseDevices": cs.BaseDevices(),
		"Devices":     cs.GetDevices(),
		"DaysLeft":    cs.DaysLeft(),
		"PriceLine":   priceLine,
		"BalanceLine": balanceLine,
	})
}
