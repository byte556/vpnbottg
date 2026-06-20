// internal/repository/user.go
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

func (d *DB) UpsertUser(ctx context.Context, u *models.User) error {
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO users (tg_id, username, first_name, referred_by)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(tg_id) DO UPDATE SET
			username   = excluded.username,
			first_name = excluded.first_name
	`, u.TgID, u.Username, u.FirstName, u.ReferredBy)
	if err != nil {
		return fmt.Errorf("upsertUser: %w", err)
	}
	return nil
}

func (d *DB) GetUser(ctx context.Context, tgID int64) (*models.User, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT tg_id, username, first_name, referred_by, created_at
		FROM users WHERE tg_id = ?
	`, tgID)

	var u models.User
	if err := row.Scan(&u.TgID, &u.Username, &u.FirstName, &u.ReferredBy, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("getUser: %w", err)
	}
	return &u, nil
}
