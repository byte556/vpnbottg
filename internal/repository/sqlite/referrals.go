package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

func (d *DB) CreateReferral(ctx context.Context, r *models.Referral) error {
	_, err := d.sql.ExecContext(ctx, `
		INSERT OR IGNORE INTO referrals (referrer_id, referee_id)
		VALUES (?, ?)
	`, r.ReferrerID, r.RefereeID)
	if err != nil {
		return fmt.Errorf("createReferral: %w", err)
	}
	return nil
}

func (d *DB) GetReferrerByReferee(ctx context.Context, refereeID int64) (int64, error) {
	var referrerID int64
	err := d.sql.QueryRowContext(ctx,
		`SELECT referrer_id FROM referrals WHERE referee_id = ? LIMIT 1`, refereeID).Scan(&referrerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, repository.ErrNotFound
		}
		return 0, fmt.Errorf("getReferrerByReferee: %w", err)
	}
	return referrerID, nil
}

func (d *DB) GetReferralCount(ctx context.Context, referrerID int64) (int, error) {
	var count int
	err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM referrals WHERE referrer_id = ?`, referrerID).Scan(&count)
	return count, err
}
