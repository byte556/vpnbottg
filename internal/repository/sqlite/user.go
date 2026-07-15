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
		SELECT tg_id, username, first_name, referred_by, balance_rub, created_at
		FROM users WHERE tg_id = ?
	`, tgID)

	var u models.User
	if err := row.Scan(&u.TgID, &u.Username, &u.FirstName, &u.ReferredBy, &u.BalanceRub, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("getUser: %w", err)
	}
	return &u, nil
}

func (d *DB) SearchUserByUsername(ctx context.Context, username string) (*models.User, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT tg_id, username, first_name, referred_by, balance_rub, created_at
		FROM users WHERE username = ?
	`, username)

	var u models.User
	if err := row.Scan(&u.TgID, &u.Username, &u.FirstName, &u.ReferredBy, &u.BalanceRub, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("searchUserByUsername: %w", err)
	}
	return &u, nil
}

func (d *DB) GetAllUserIDs(ctx context.Context) ([]int64, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT tg_id FROM users`)
	if err != nil {
		return nil, fmt.Errorf("getAllUserIDs: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("getAllUserIDs scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (d *DB) GetBalance(ctx context.Context, userID int64) (int64, error) {
	var balance int64
	err := d.sql.QueryRowContext(ctx, `SELECT balance_rub FROM users WHERE tg_id = ?`, userID).Scan(&balance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("getBalance: %w", err)
	}
	return balance, nil
}

func (d *DB) AddBalance(ctx context.Context, userID int64, amount int64) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE users SET balance_rub = balance_rub + ? WHERE tg_id = ?`, amount, userID)
	if err != nil {
		return fmt.Errorf("addBalance: %w", err)
	}
	return nil
}

func (d *DB) DeductBalance(ctx context.Context, userID int64, amount int64) (int64, error) {
	var before int64
	if err := d.sql.QueryRowContext(ctx, `SELECT balance_rub FROM users WHERE tg_id = ?`, userID).Scan(&before); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("deductBalance select: %w", err)
	}
	deducted := before
	if deducted > amount {
		deducted = amount
	}
	if deducted <= 0 {
		return 0, nil
	}
	_, err := d.sql.ExecContext(ctx,
		`UPDATE users SET balance_rub = balance_rub - ? WHERE tg_id = ?`, deducted, userID)
	if err != nil {
		return 0, fmt.Errorf("deductBalance update: %w", err)
	}
	return deducted, nil
}
