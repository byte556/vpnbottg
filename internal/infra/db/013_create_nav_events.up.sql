-- Трекинг навигации по экранам бота (воронка «докуда дошёл пользователь»).
-- Пишется из middleware на каждый callback-переход и /start; отдельно от
-- audit_log, где живут бизнес-события (подписки, платежи, промокоды).
CREATE TABLE IF NOT EXISTS nav_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL,
    screen     TEXT    NOT NULL,   -- имя экрана/кнопки: start, nav_buy, month_3, buy, ...
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_nav_events_user_id ON nav_events(user_id);
CREATE INDEX IF NOT EXISTS idx_nav_events_screen ON nav_events(screen);
