package db

import (
	"embed"
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed *.sql
var migrationFS embed.FS

// Connect — открыть SQLite и применить миграции.
func Connect(path string) (*sqlx.DB, error) {
	dsn := "file:" + path + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"
	db, err := sqlx.Connect("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sqlx.DB) error {
	// Без миграций-фреймворка: все .sql из embedded папки выполняем подряд,
	// идемпотентно (CREATE TABLE IF NOT EXISTS). Для прод-нагрузки этого
	// достаточно; если когда-нибудь схема усложнится — подключим
	// golang-migrate.
	files, err := migrationFS.ReadDir(".")
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		body, err := migrationFS.ReadFile(f.Name())
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(body)); err != nil {
			return fmt.Errorf("exec %s: %w", f.Name(), err)
		}
	}
	return nil
}
