CREATE TABLE IF NOT EXISTS referrals (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    referrer_id INTEGER NOT NULL REFERENCES users(tg_id),
    referee_id  INTEGER NOT NULL REFERENCES users(tg_id),
    rewarded_at INTEGER,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE(referee_id)
);
