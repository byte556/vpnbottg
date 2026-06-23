package callbacks

import (
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/handlers"
	"vpnbottg/internal/telegram/session"

	tele "gopkg.in/telebot.v3"
)

// PromoRedeemCallback — кнопка «🎟 Промокод»: ждём ввод кода следующим сообщением.
func PromoRedeemCallback(c tele.Context) error {
	sess := session.GetStore().Get(c.Sender().ID)
	sess.AwaitPromo = true
	session.GetStore().Save(c.Sender().ID, sess)
	return handlers.PromoPrompt(c)
}

// AdminPromoCallback — кнопка «🎟 Промокоды» в админке: список + ожидание ввода.
func AdminPromoCallback(promoSvc *service.PromoService) tele.HandlerFunc {
	return func(c tele.Context) error {
		sess := session.GetStore().Get(c.Sender().ID)
		sess.AdminAction = "promo"
		session.GetStore().Save(c.Sender().ID, sess)
		return handlers.AdminPromo(promoSvc)(c)
	}
}

// clearSessionPromo сбрасывает применённую промо-скидку в сессии.
func clearSessionPromo(userID int64) {
	sess := session.GetStore().Get(userID)
	sess.PromoCode = ""
	sess.PromoDiscountPct = 0
	session.GetStore().Save(userID, sess)
}
