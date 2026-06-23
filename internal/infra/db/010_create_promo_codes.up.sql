CREATE TABLE IF NOT EXISTS promo_codes (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    code         TEXT    NOT NULL UNIQUE,           -- хранится в UPPER
    reward_type  TEXT    NOT NULL,                  -- 'days' | 'discount'
    days         INTEGER NOT NULL DEFAULT 0,
    gb           INTEGER NOT NULL DEFAULT 0,
    devices      INTEGER NOT NULL DEFAULT 0,
    discount_pct INTEGER NOT NULL DEFAULT 0,
    max_uses     INTEGER NOT NULL DEFAULT 1,
    used_count   INTEGER NOT NULL DEFAULT 0,
    active       INTEGER NOT NULL DEFAULT 1,
    expires_at   INTEGER,                           -- nullable, на будущее
    created_at   INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS promo_redemptions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    promo_id    INTEGER NOT NULL REFERENCES promo_codes(id),
    user_id     INTEGER NOT NULL,
    redeemed_at INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE(promo_id, user_id)
);
