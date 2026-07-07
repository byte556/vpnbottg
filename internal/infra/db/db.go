package db

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

//go:embed *.sql
var migrationFS embed.FS

// Connect — открыть SQLite и применить миграции.
func Connect(path string) (*sqlx.DB, error) {
	dsn := "file:" + path + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"
	db, err := sqlx.Connect("sqlite", dsn)
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
	files, err := migrationFS.ReadDir(".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		if !f.IsDir() {
			names = append(names, f.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		applied, err := migrationApplied(db, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := migrationFS.ReadFile(name)
		if err != nil {
			return err
		}

		tx, err := db.Beginx()
		if err != nil {
			return fmt.Errorf("begin %s: %w", name, err)
		}
		if _, err := tx.Exec(string(body)); err != nil {
			_ = tx.Rollback()
			if isAlreadyAppliedSQLiteError(err) {
				if _, markErr := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); markErr != nil {
					return fmt.Errorf("mark already applied %s: %w", name, markErr)
				}
				continue
			}
			return fmt.Errorf("exec %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("mark %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
	}
	return nil
}

func migrationApplied(db *sqlx.DB, name string) (bool, error) {
	ok, err := tableExists(db, "schema_migrations")
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, name); err != nil {
		return false, fmt.Errorf("check migration %s: %w", name, err)
	}
	return count > 0, nil
}

func isAlreadyAppliedSQLiteError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column name")
}

func tableExists(db *sqlx.DB, table string) (bool, error) {
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table); err != nil {
		return false, fmt.Errorf("check table %s: %w", table, err)
	}
	return count > 0, nil
}
