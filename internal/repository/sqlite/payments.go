// internal/repository/payments.go
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

func (d *DB) CreatePayment(p *models.Payment) (int64, error) {
	res, err := d.sql.Exec(`
		INSERT INTO payments (user_id, amount, provider, provider_payment_id, status)
		VALUES (?, ?, ?, ?, ?)
	`, p.UserID, p.Amount, p.Provider, p.ProviderPaymentID, p.Status)
	if err != nil {
		return 0, fmt.Errorf("createPayment: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (d *DB) UpdatePaymentStatus(ctx context.Context, providerPaymentID, status string) (bool, error) {
	res, err := d.sql.ExecContext(ctx, `
		UPDATE payments SET status = ? WHERE provider_payment_id = ? AND status = 'pending'
	`, status, providerPaymentID)
	if err != nil {
		return false, fmt.Errorf("updatePaymentStatus: %w", err)
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

func (d *DB) GetPendingPayments(ctx context.Context) ([]*models.Payment, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, user_id, amount, provider, provider_payment_id, status, created_at
		FROM payments
		WHERE status = 'pending' AND created_at > unixepoch() - 7200
	`)
	if err != nil {
		return nil, fmt.Errorf("getPendingPayments: %w", err)
	}
	defer rows.Close()

	var out []*models.Payment
	for rows.Next() {
		var p models.Payment
		if err := rows.Scan(&p.ID, &p.UserID, &p.Amount, &p.Provider, &p.ProviderPaymentID, &p.Status, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("getPendingPayments scan: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (d *DB) HasSucceededPayment(ctx context.Context, userID int64) (bool, error) {
	var count int
	err := d.sql.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM payments WHERE user_id = ? AND status = 'succeeded'`,
		userID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("hasSucceededPayment: %w", err)
	}
	return count > 0, nil
}

func (d *DB) GetLastSucceededPayment(ctx context.Context, userID int64) (*models.Payment, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT id, user_id, amount, provider, provider_payment_id, status, created_at
		FROM payments WHERE user_id = ? AND status = 'succeeded'
		ORDER BY created_at DESC LIMIT 1
	`, userID)

	var p models.Payment
	if err := row.Scan(&p.ID, &p.UserID, &p.Amount, &p.Provider, &p.ProviderPaymentID, &p.Status, &p.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("getLastSucceededPayment: %w", err)
	}
	return &p, nil
}

func (d *DB) MarkRefunded(ctx context.Context, providerPaymentID string) error {
	_, err := d.sql.ExecContext(
		ctx,
		`UPDATE payments SET status = 'refunded' WHERE provider_payment_id = ?`,
		providerPaymentID,
	)
	if err != nil {
		return fmt.Errorf("markRefunded: %w", err)
	}
	return nil
}

func (d *DB) GetPaymentByProviderID(ctx context.Context, providerPaymentID string) (*models.Payment, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT id, user_id, amount, provider, provider_payment_id, status, created_at
		FROM payments WHERE provider_payment_id = ?
	`, providerPaymentID)

	var p models.Payment
	if err := row.Scan(&p.ID, &p.UserID, &p.Amount, &p.Provider, &p.ProviderPaymentID, &p.Status, &p.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("getPaymentByProviderID: %w", err)
	}
	return &p, nil
}
