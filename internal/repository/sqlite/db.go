package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"vpnbottg/internal/infra/db"
)

type DB struct {
	sql *sql.DB
}

func New(path string) (*DB, error) {
	sqlxdb, err := db.Connect(path)
	if err != nil {
		return nil, fmt.Errorf("repository connect: %w", err)
	}
	return &DB{sql: sqlxdb.DB}, nil
}

// PurgeUserData удаляет все данные пользователя в одной транзакции.
// Сначала читает XUI-email'ы из подписок (нужны для удаления в панели),
// затем чистит: subscriptions → payments → referrals → audit_log (NULL) → users.
func (d *DB) PurgeUserData(ctx context.Context, tgID int64) (xuiEmailDirect, xuiEmailRelay string, err error) {
	row := d.sql.QueryRowContext(
		ctx,
		`SELECT xui_email_direct, xui_email_relay FROM subscriptions WHERE user_id = ? ORDER BY id DESC LIMIT 1`,
		tgID,
	)
	_ = row.Scan(&xuiEmailDirect, &xuiEmailRelay)

	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("purgeUserData: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmts := []string{
		`DELETE FROM subscriptions WHERE user_id = ?`,
		`DELETE FROM payments     WHERE user_id = ?`,
		`DELETE FROM referrals    WHERE referrer_id = ? OR referee_id = ?`,
		`UPDATE audit_log SET user_id = NULL WHERE user_id = ?`,
		`DELETE FROM users WHERE tg_id = ?`,
	}
	args := [][]any{
		{tgID},
		{tgID},
		{tgID, tgID},
		{tgID},
		{tgID},
	}

	for i, stmt := range stmts {
		if _, execErr := tx.ExecContext(ctx, stmt, args[i]...); execErr != nil {
			err = fmt.Errorf("purgeUserData step %d: %w", i, execErr)
			return "", "", err
		}
	}

	if err = tx.Commit(); err != nil {
		return "", "", fmt.Errorf("purgeUserData: commit: %w", err)
	}
	return xuiEmailDirect, xuiEmailRelay, nil
}

func (d *DB) Close() error {
	return d.sql.Close()
}
