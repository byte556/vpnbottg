package middleware

import (
	"vpnbottg/internal/config"
	"vpnbottg/internal/infra/logger"

	tele "gopkg.in/telebot.v3"
)

// IsAdmin возвращает true если отправитель — настроенный администратор.
func IsAdmin(c tele.Context) bool {
	adminID := config.Cfg.Bot.AdminID
	return adminID != 0 && c.Sender().ID == adminID
}

// AdminOnly пропускает только запросы от admin_id из конфига.
func AdminOnly(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		adminID := config.Cfg.Bot.AdminID
		senderID := c.Sender().ID
		if adminID == 0 {
			logger.L().Warn().Msg("AdminOnly: admin_id not configured (= 0), denying all")
			return nil
		}
		if senderID != adminID {
			logger.L().Warn().
				Int64("sender_id", senderID).
				Int64("admin_id", adminID).
				Msg("AdminOnly: access denied")
			return nil
		}
		return next(c)
	}
}
