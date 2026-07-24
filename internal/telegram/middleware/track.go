package middleware

import (
	"context"

	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/repository"

	tele "gopkg.in/telebot.v3"
)

// noopScreens — служебные кнопки, которые не являются навигацией и только
// зашумляют воронку (счётчик ➕/➖ без значения).
var noopScreens = map[string]bool{
	"noop_dev": true,
}

// TrackNav возвращает middleware, пишущее переход пользователя по экранам в
// nav_events. Ловит нажатия inline-кнопок (по Callback().Unique) и /start.
// Служебные (noop) и админские (admin_*) события пропускаются — воронка про
// путь обычного пользователя. Запись асинхронная и best-effort: ошибка БД
// логируется, но не мешает обработке апдейта.
func TrackNav(nav repository.Nav) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			if screen := navScreen(c); screen != "" {
				sender := c.Sender()
				if sender != nil {
					go func(userID int64, screen string) {
						if err := nav.TrackNav(context.Background(), userID, screen); err != nil {
							logger.L().Warn().Err(err).
								Int64("user_id", userID).
								Str("screen", screen).
								Msg("trackNav: insert failed")
						}
					}(sender.ID, screen)
				}
			}
			return next(c)
		}
	}
}

// navScreen извлекает имя экрана из апдейта или "" если событие трекать не нужно.
func navScreen(c tele.Context) string {
	if cb := c.Callback(); cb != nil {
		unique := trimUnique(cb.Unique)
		if unique == "" || noopScreens[unique] || isAdminScreen(unique) {
			return ""
		}
		return unique
	}
	// /start — точка входа воронки. Прочие текстовые сообщения (ввод промокода,
	// админский ввод) — не навигация.
	if msg := c.Message(); msg != nil && msg.Text != "" {
		if msg.Text == "/start" || hasStartPrefix(msg.Text) {
			return "start"
		}
	}
	return ""
}

// trimUnique нормализует Unique кнопки: telebot хранит его как есть (тот же
// стринг, что задан в keyboard.*), поэтому доп. обработка не нужна — оставляем
// хук на случай префиксов.
func trimUnique(u string) string { return u }

func hasStartPrefix(text string) bool {
	const p = "/start"
	return len(text) >= len(p) && text[:len(p)] == p
}

func isAdminScreen(screen string) bool {
	const p = "admin_"
	return len(screen) >= len(p) && screen[:len(p)] == p
}
