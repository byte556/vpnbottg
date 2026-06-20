// internal/repository/subscriptions.go
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"vpnbottg/internal/models"
)

func (d *DB) CreateSubscription(ctx context.Context, s *models.Subscription) (int64, error) {
	res, err := d.sql.ExecContext(ctx, `
		INSERT INTO subscriptions
			(user_id, xui_email_direct, xui_email_relay, bypass, traffic_gb, started_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, s.UserID, s.XUIEmailDirect, s.XUIEmailRelay, s.Bypass, s.TrafficGB, s.StartedAt, s.ExpiresAt)
	if err != nil {
		return 0, fmt.Errorf("createSubscription: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (d *DB) GetActiveSubscription(ctx context.Context, userID int64) (*models.Subscription, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT id, user_id, xui_email_direct, xui_email_relay, bypass, traffic_gb, started_at, expires_at, created_at
		FROM subscriptions
		WHERE user_id = ? AND expires_at > unixepoch()
		ORDER BY expires_at DESC
		LIMIT 1
	`, userID)

	var s models.Subscription
	if err := row.Scan(&s.ID, &s.UserID, &s.XUIEmailDirect, &s.XUIEmailRelay, &s.Bypass, &s.TrafficGB, &s.StartedAt, &s.ExpiresAt, &s.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getActiveSubscription: %w", err)
	}
	return &s, nil
}
