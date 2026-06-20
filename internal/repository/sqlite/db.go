package sqlite

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	sql *sql.DB
}

func New(path string) (*DB, error) {
	sqldb, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("repository open: %w", err)
	}
	if err := sqldb.Ping(); err != nil {
		return nil, fmt.Errorf("repository ping: %w", err)
	}
	d := &DB{sql: sqldb}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("repository migrate: %w", err)
	}
	return d, nil
}

func (d *DB) migrate() error {
	_, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			tg_id        INTEGER PRIMARY KEY,
			username     TEXT,
			first_name   TEXT,
			referred_by  INTEGER REFERENCES users(tg_id),
			created_at   INTEGER NOT NULL DEFAULT (unixepoch())
		);
		CREATE TABLE IF NOT EXISTS subscriptions (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id             INTEGER NOT NULL REFERENCES users(tg_id),
			xui_email_direct    TEXT NOT NULL,
			xui_email_relay     TEXT NOT NULL,
			bypass              INTEGER NOT NULL DEFAULT 0,
			traffic_gb          INTEGER NOT NULL,
			started_at          INTEGER NOT NULL,
			expires_at          INTEGER NOT NULL,
			created_at          INTEGER NOT NULL DEFAULT (unixepoch())
		);
		CREATE TABLE IF NOT EXISTS payments (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id             INTEGER NOT NULL REFERENCES users(tg_id),
			amount              INTEGER NOT NULL,
			provider            TEXT NOT NULL DEFAULT 'yookassa',
			provider_payment_id TEXT,
			status              TEXT NOT NULL DEFAULT 'pending',
			created_at          INTEGER NOT NULL DEFAULT (unixepoch())
		);
		CREATE TABLE IF NOT EXISTS audit_log (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER REFERENCES users(tg_id),
			action     TEXT NOT NULL,
			payload    TEXT,
			created_at INTEGER NOT NULL DEFAULT (unixepoch())
		);
	`)
	return err
}

func (d *DB) Close() error {
	return d.sql.Close()
}
