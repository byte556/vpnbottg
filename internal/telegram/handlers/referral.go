package handlers

import (
	"context"

	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/service"

	tele "gopkg.in/telebot.v3"
)

// RewardReferrer начисляет реферреру кешбэк за оплату его реферала и уведомляет
// его экраном referral.reward.
//
// Уведомление реферреру — всегда push (свежее сообщение третьему лицу; чужое
// сообщение отредактировать нельзя), поэтому оба пути выдачи — ручная проверка
// оплаты (есть tele.Context) и вебхук (Context отсутствует) — используют эту
// единую функцию, работающую по bot + userID.
//
// Ничего не делает, если ref == nil, реферрера нет или расчёт кешбэка вернул 0.
func RewardReferrer(ctx context.Context, bot *tele.Bot, ref *service.ReferralService, user *service.User, refereeID, paymentRub int64) {
	if ref == nil {
		return
	}
	log := logger.L().With().Str("func", "RewardReferrer").Int64("referee_id", refereeID).Logger()

	referrerID, rewardRub, err := ref.RewardBalance(ctx, refereeID, paymentRub)
	if err != nil {
		log.Error().Err(err).Msg("RewardBalance failed")
		return
	}
	if referrerID == 0 {
		return
	}

	name := refereeName(ctx, user, refereeID)
	if err := PushReferralReward(bot, referrerID, name, rewardRub); err != nil {
		log.Error().Err(err).Int64("referrer_id", referrerID).Msg("push reward failed")
	}
	log.Info().Int64("referrer_id", referrerID).Int64("reward_rub", rewardRub).Msg("notified")
}

// refereeName возвращает отображаемое имя реферала: @username, затем имя,
// иначе «Друг» (в т.ч. если пользователь не найден).
func refereeName(ctx context.Context, user *service.User, refereeID int64) string {
	u, _ := user.GetUser(ctx, refereeID)
	if u != nil {
		if u.Username != "" {
			return "@" + u.Username
		}
		if u.FirstName != "" {
			return u.FirstName
		}
	}
	return "Друг"
}
