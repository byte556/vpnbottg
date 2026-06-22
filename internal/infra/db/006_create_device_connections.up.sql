CREATE TABLE IF NOT EXISTS device_connections (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    sub_id     TEXT NOT NULL,
    device_id  TEXT NOT NULL,
    platform   TEXT NOT NULL,
    first_seen INTEGER NOT NULL DEFAULT (unixepoch()),
    last_seen  INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE(sub_id, device_id)
);
