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

func (d *DB) UpdatePaymentStatus(ctx context.Context, providerPaymentID, status string) error {
	_, err := d.sql.ExecContext(ctx, `
		UPDATE payments SET status = ? WHERE provider_payment_id = ?
	`, status, providerPaymentID)
	if err != nil {
		return fmt.Errorf("updatePaymentStatus: %w", err)
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
