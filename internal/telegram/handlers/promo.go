package handlers

import (
	"context"
	"fmt"
	"strings"
	"vpnbottg/internal/models"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// PromoPrompt — экран ввода промокода (после кнопки 🎟 Промокод).
func PromoPrompt(c tele.Context) error {
	return screen(c, "promo", texts.T("promo.prompt"), keyboard.PromoPromptKeyboard())
}

// PromoDaysSuccess — промокод выдал дни подписки: показываем успех и меню подписчика.
func PromoDaysSuccess(c tele.Context, res *service.RedeemResult, subSvc *service.Subscription) error {
	if err := editOrFresh(c, texts.T("promo.days_success", map[string]any{
		"Days":    res.Days,
		"Devices": res.Devices,
	}), &tele.SendOptions{ParseMode: tele.ModeHTML}); err != nil {
		return err
	}
	return Menu(c, subSvc)
}

// PromoDiscountApplied — discount-код применён к сессии: сообщаем и открываем конструктор.
func PromoDiscountApplied(c tele.Context, res *service.RedeemResult) error {
	if err := editOrFresh(c, texts.T("promo.discount_applied", map[string]any{
		"Pct":  res.DiscountPct,
		"Code": res.Code,
	}), &tele.SendOptions{ParseMode: tele.ModeHTML}); err != nil {
		return err
	}
	return Constructor(c)
}

// AdminPromo рендерит список промокодов + подсказку формата (экран создания).
func AdminPromo(promoSvc *service.PromoService) tele.HandlerFunc {
	return func(c tele.Context) error {
		codes, err := promoSvc.List(context.Background())
		if err != nil {
			return c.EditOrSend(texts.T("admin.err_generic"), keyboard.AdminCancelKeyboard())
		}
		// SendOptions первым — иначе telebot затирает клавиатуру (см. sendOptsFirst).
		return c.EditOrSend(
			texts.T("admin.promo.text", map[string]any{"List": formatPromoList(codes)}),
			&tele.SendOptions{ParseMode: tele.ModeHTML},
			keyboard.AdminCancelKeyboard(),
		)
	}
}

func formatPromoList(codes []*models.PromoCode) string {
	if len(codes) == 0 {
		return texts.T("admin.promo.empty")
	}
	lines := make([]string, 0, len(codes))
	for _, p := range codes {
		var reward string
		if p.RewardType == models.PromoRewardDays {
			reward = fmt.Sprintf("%d дн./%dГБ/%dустр", p.Days, p.GB, p.Devices)
		} else {
			reward = fmt.Sprintf("−%d%%", p.DiscountPct)
		}
		status := "✅"
		if !p.Active {
			status = "🚫"
		}
		lines = append(lines, fmt.Sprintf("%s <code>%s</code> · %s · %d/%d",
			status, p.Code, reward, p.UsedCount, p.MaxUses))
	}
	return strings.Join(lines, "\n")
}
