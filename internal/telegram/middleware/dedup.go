package middleware

import (
	"sync"
	"time"

	"vpnbottg/internal/infra/logger"

	tele "gopkg.in/telebot.v3"
)

// dedup хранит недавно обработанные update ID.
// TTL = 60 секунд; защищает от Telegram resend и двойного запуска бота.
type dedup struct {
	mu  sync.Mutex
	ids map[int]time.Time
}

var globalDedup = &dedup{ids: make(map[int]time.Time)}

// Dedup пропускает апдейт только один раз. Повторный — тихо игнорируется.
func Dedup(next tele.HandlerFunc) tele.HandlerFunc {
	const ttl = 60 * time.Second

	return func(c tele.Context) error {
		id := c.Update().ID

		globalDedup.mu.Lock()
		now := time.Now()
		// чистим устаревшие записи
		for k, t := range globalDedup.ids {
			if now.Sub(t) > ttl {
				delete(globalDedup.ids, k)
			}
		}
		if _, exists := globalDedup.ids[id]; exists {
			globalDedup.mu.Unlock()
			logger.L().Warn().
				Int("update_id", id).
				Int64("user_id", c.Sender().ID).
				Msg("dedup: duplicate update dropped")
			return nil
		}
		globalDedup.ids[id] = now
		globalDedup.mu.Unlock()

		return next(c)
	}
}
