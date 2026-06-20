CREATE TABLE users (
                       tg_id        INTEGER PRIMARY KEY,
                       username     TEXT,
                       first_name   TEXT,
                       referred_by  INTEGER REFERENCES users(tg_id),
                       created_at   INTEGER NOT NULL DEFAULT (unixepoch())
);



CREATE TABLE subscriptions (
      id           INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id      INTEGER NOT NULL REFERENCES users(tg_id),
      xui_email_direct    TEXT NOT NULL,
      xui_email_relay    TEXT NOT NULL,
      bypass INTEGER NOT NULL,
      traffic_gb   INTEGER NOT NULL,
      started_at   INTEGER NOT NULL,
      expires_at   INTEGER NOT NULL,
      created_at   INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE payments (
                          id                  INTEGER PRIMARY KEY AUTOINCREMENT,
                          user_id             INTEGER NOT NULL REFERENCES users(tg_id),
                          amount              INTEGER NOT NULL,
                          provider            TEXT NOT NULL DEFAULT 'yookassa',
                          provider_payment_id TEXT,
                          status              TEXT NOT NULL DEFAULT 'pending',
                          created_at          INTEGER NOT NULL DEFAULT (unixepoch())
);


CREATE TABLE audit_log (
                           id         INTEGER PRIMARY KEY AUTOINCREMENT,
                           user_id    INTEGER REFERENCES users(tg_id),
                           action     TEXT NOT NULL,
                           payload    TEXT,
                           created_at INTEGER NOT NULL DEFAULT (unixepoch())
);