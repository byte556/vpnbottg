// internal/repository/subscriptions.go
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

func (d *DB) CreateSubscription(ctx context.Context, s *models.Subscription) (int64, error) {
	res, err := d.sql.ExecContext(ctx, `
		INSERT INTO subscriptions
			(user_id, xui_email_direct, xui_email_relay, xui_sub_id, bypass, traffic_gb, device_limit, started_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.UserID, s.XUIEmailDirect, s.XUIEmailRelay, s.XUISubID, s.Bypass, s.TrafficGB, s.DeviceLimit, s.StartedAt, s.ExpiresAt)
	if err != nil {
		return 0, fmt.Errorf("createSubscription: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (d *DB) GetActiveSubscription(ctx context.Context, userID int64) (*models.Subscription, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT id, user_id, xui_email_direct, xui_email_relay, xui_sub_id, bypass, traffic_gb, device_limit,
		       started_at, expires_at, created_at, reminded_at, expired_notified
		FROM subscriptions
		WHERE user_id = ? AND expires_at > unixepoch()
		ORDER BY expires_at DESC LIMIT 1
	`, userID)
	return scanSub(row)
}

func scanSub(row *sql.Row) (*models.Subscription, error) {
	var s models.Subscription
	if err := row.Scan(
		&s.ID, &s.UserID, &s.XUIEmailDirect, &s.XUIEmailRelay, &s.XUISubID,
		&s.Bypass, &s.TrafficGB, &s.DeviceLimit,
		&s.StartedAt, &s.ExpiresAt, &s.CreatedAt, &s.RemindedAt, &s.ExpiredNotified,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scanSub: %w", err)
	}
	return &s, nil
}

func (d *DB) UpdateSubscriptionExpiry(ctx context.Context, subID int64, expiresAt int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE subscriptions SET expires_at = ? WHERE id = ?`, expiresAt, subID)
	if err != nil {
		return fmt.Errorf("updateSubscriptionExpiry: %w", err)
	}
	return nil
}

func (d *DB) UpdateSubscriptionDevices(ctx context.Context, subID int64, deviceLimit, trafficGB int) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE subscriptions SET device_limit = ?, traffic_gb = ? WHERE id = ?`, deviceLimit, trafficGB, subID)
	if err != nil {
		return fmt.Errorf("updateSubscriptionDevices: %w", err)
	}
	return nil
}

func (d *DB) UpdateSubscriptionXUISubID(ctx context.Context, subID int64, xuiSubID string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE subscriptions SET xui_sub_id = ? WHERE id = ?`, xuiSubID, subID)
	if err != nil {
		return fmt.Errorf("updateSubscriptionXUISubID: %w", err)
	}
	return nil
}

func (d *DB) GetExpiringUnreminded(ctx context.Context, from, to int64) ([]*models.Subscription, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, user_id, xui_email_direct, xui_email_relay, xui_sub_id, bypass, traffic_gb, device_limit,
		       started_at, expires_at, created_at, reminded_at, expired_notified
		FROM subscriptions
		WHERE expires_at BETWEEN ? AND ? AND reminded_at IS NULL
	`, from, to)
	if err != nil {
		return nil, fmt.Errorf("getExpiringUnreminded: %w", err)
	}
	defer rows.Close()
	return scanSubs(rows)
}

func (d *DB) GetRecentlyExpiredUnnotified(ctx context.Context, since int64) ([]*models.Subscription, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, user_id, xui_email_direct, xui_email_relay, xui_sub_id, bypass, traffic_gb, device_limit,
		       started_at, expires_at, created_at, reminded_at, expired_notified
		FROM subscriptions
		WHERE expires_at < unixepoch() AND expires_at > ? AND expired_notified = 0
	`, since)
	if err != nil {
		return nil, fmt.Errorf("getRecentlyExpiredUnnotified: %w", err)
	}
	defer rows.Close()
	return scanSubs(rows)
}

func (d *DB) MarkReminded(ctx context.Context, subID int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE subscriptions SET reminded_at = unixepoch() WHERE id = ?`, subID)
	return err
}

func (d *DB) MarkExpiredNotified(ctx context.Context, subID int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE subscriptions SET expired_notified = 1 WHERE id = ?`, subID)
	return err
}

func scanSubs(rows *sql.Rows) ([]*models.Subscription, error) {
	var result []*models.Subscription
	for rows.Next() {
		var s models.Subscription
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.XUIEmailDirect, &s.XUIEmailRelay, &s.XUISubID,
			&s.Bypass, &s.TrafficGB, &s.DeviceLimit,
			&s.StartedAt, &s.ExpiresAt, &s.CreatedAt, &s.RemindedAt, &s.ExpiredNotified,
		); err != nil {
			return nil, fmt.Errorf("scanSubs: %w", err)
		}
		result = append(result, &s)
	}
	return result, rows.Err()
}
