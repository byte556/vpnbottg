package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (d *DB) CreatePromoCode(ctx context.Context, p *models.PromoCode) (int64, error) {
	res, err := d.sql.ExecContext(ctx, `
		INSERT INTO promo_codes
			(code, reward_type, days, gb, devices, discount_pct, max_uses, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, normalizeCode(p.Code), p.RewardType, p.Days, p.GB, p.Devices, p.DiscountPct, p.MaxUses, p.ExpiresAt)
	if err != nil {
		return 0, fmt.Errorf("createPromoCode: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (d *DB) GetPromoByCode(ctx context.Context, code string) (*models.PromoCode, error) {
	var p models.PromoCode
	err := d.sql.QueryRowContext(ctx, `
		SELECT id, code, reward_type, days, gb, devices, discount_pct, max_uses, used_count, active, expires_at, created_at
		FROM promo_codes WHERE code = ?
	`, normalizeCode(code)).Scan(
		&p.ID, &p.Code, &p.RewardType, &p.Days, &p.GB, &p.Devices,
		&p.DiscountPct, &p.MaxUses, &p.UsedCount, &p.Active, &p.ExpiresAt, &p.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getPromoByCode: %w", err)
	}
	return &p, nil
}

func (d *DB) ListPromoCodes(ctx context.Context) ([]*models.PromoCode, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, code, reward_type, days, gb, devices, discount_pct, max_uses, used_count, active, expires_at, created_at
		FROM promo_codes ORDER BY active DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("listPromoCodes: %w", err)
	}
	defer rows.Close()

	var result []*models.PromoCode
	for rows.Next() {
		var p models.PromoCode
		if err := rows.Scan(
			&p.ID, &p.Code, &p.RewardType, &p.Days, &p.GB, &p.Devices,
			&p.DiscountPct, &p.MaxUses, &p.UsedCount, &p.Active, &p.ExpiresAt, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("listPromoCodes scan: %w", err)
		}
		result = append(result, &p)
	}
	return result, rows.Err()
}

func (d *DB) DeactivatePromoCode(ctx context.Context, code string) error {
	res, err := d.sql.ExecContext(ctx, `UPDATE promo_codes SET active = 0 WHERE code = ?`, normalizeCode(code))
	if err != nil {
		return fmt.Errorf("deactivatePromoCode: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (d *DB) HasRedeemed(ctx context.Context, promoID, userID int64) (bool, error) {
	var count int
	if err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM promo_redemptions WHERE promo_id = ? AND user_id = ?`, promoID, userID,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("hasRedeemed: %w", err)
	}
	return count > 0, nil
}

// RedeemPromo атомарно бронирует одну активацию промокода — см. интерфейс PromoCodes.
func (d *DB) RedeemPromo(ctx context.Context, promoID, userID int64) (err error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("redeemPromo: begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx,
		`INSERT INTO promo_redemptions (promo_id, user_id) VALUES (?, ?)`, promoID, userID,
	); err != nil {
		if isUniqueViolation(err) {
			err = repository.ErrPromoAlreadyUsed
			return err
		}
		return fmt.Errorf("redeemPromo: insert redemption: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE promo_codes SET used_count = used_count + 1 WHERE id = ? AND used_count < max_uses AND active = 1`, promoID,
	)
	if err != nil {
		return fmt.Errorf("redeemPromo: update count: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("redeemPromo: rows: %w", err)
	}
	if n == 0 {
		err = repository.ErrPromoLimitReached
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("redeemPromo: commit: %w", err)
	}
	return nil
}

// ReleasePromo откатывает бронь, если выдача награды не удалась.
func (d *DB) ReleasePromo(ctx context.Context, promoID, userID int64) (err error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("releasePromo: begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx,
		`DELETE FROM promo_redemptions WHERE promo_id = ? AND user_id = ?`, promoID, userID,
	); err != nil {
		return fmt.Errorf("releasePromo: delete redemption: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE promo_codes SET used_count = used_count - 1 WHERE id = ? AND used_count > 0`, promoID,
	); err != nil {
		return fmt.Errorf("releasePromo: decrement: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("releasePromo: commit: %w", err)
	}
	return nil
}
