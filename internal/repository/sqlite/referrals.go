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

func (d *DB) GetUnrewardedByReferee(ctx context.Context, refereeID int64) (*models.Referral, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT id, referrer_id, referee_id, rewarded_at, created_at
		FROM referrals
		WHERE referee_id = ? AND rewarded_at IS NULL
	`, refereeID)

	var r models.Referral
	if err := row.Scan(&r.ID, &r.ReferrerID, &r.RefereeID, &r.RewardedAt, &r.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("getUnrewardedByReferee: %w", err)
	}
	return &r, nil
}

func (d *DB) MarkReferralRewarded(ctx context.Context, referralID int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE referrals SET rewarded_at = unixepoch() WHERE id = ?`, referralID)
	if err != nil {
		return fmt.Errorf("markReferralRewarded: %w", err)
	}
	return nil
}

func (d *DB) GetReferralCount(ctx context.Context, referrerID int64) (int, error) {
	var count int
	err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM referrals WHERE referrer_id = ?`, referrerID).Scan(&count)
	return count, err
}
