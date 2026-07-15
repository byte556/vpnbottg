package sqlite

import (
	"context"
	"time"

	"vpnbottg/internal/repository"
)

func (d *DB) GetBotStats(ctx context.Context) (*repository.BotStats, error) {
	s := &repository.BotStats{}

	if err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&s.TotalUsers); err != nil {
		return nil, err
	}
	if err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriptions WHERE expires_at > unixepoch()`).Scan(&s.ActiveSubs); err != nil {
		return nil, err
	}
	if err := d.sql.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount),0) FROM payments WHERE status='succeeded'`).Scan(&s.TotalRevenue); err != nil {
		return nil, err
	}
	if err := d.sql.QueryRowContext(
		ctx,
		`SELECT COALESCE(SUM(amount),0) FROM payments WHERE status='succeeded' AND created_at >= unixepoch('now','start of day')`,
	).Scan(&s.TodayRevenue); err != nil {
		return nil, err
	}
	if err := d.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM payments p
		WHERE p.status = 'succeeded'
		  AND NOT EXISTS (
			SELECT 1 FROM subscriptions s
			WHERE s.user_id = p.user_id AND s.expires_at > unixepoch()
		  )
	`).Scan(&s.OrphanedPayments); err != nil {
		return nil, err
	}
	return s, nil
}

func (d *DB) GetRecentPayments(ctx context.Context, limit int) ([]*repository.RecentPayment, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT p.amount, COALESCE(u.username,''), u.first_name, p.created_at
		FROM payments p
		JOIN users u ON u.tg_id = p.user_id
		WHERE p.status = 'succeeded'
		ORDER BY p.created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*repository.RecentPayment
	for rows.Next() {
		r := &repository.RecentPayment{}
		if err := rows.Scan(&r.Amount, &r.Username, &r.FirstName, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) GetOrphanedPayments(ctx context.Context, limit int) ([]*repository.OrphanedPaymentInfo, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT p.user_id, COALESCE(u.username,''), u.first_name, p.amount, p.provider_payment_id, p.created_at
		FROM payments p
		JOIN users u ON u.tg_id = p.user_id
		WHERE p.status = 'succeeded'
		  AND NOT EXISTS (
			SELECT 1 FROM subscriptions s WHERE s.user_id = p.user_id AND s.expires_at > unixepoch()
		  )
		ORDER BY p.created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*repository.OrphanedPaymentInfo
	for rows.Next() {
		r := &repository.OrphanedPaymentInfo{}
		if err := rows.Scan(&r.UserID, &r.Username, &r.FirstName, &r.Amount, &r.ProviderPaymentID, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) GetDailyRevenue(ctx context.Context, days int) ([]*repository.DailyRevenue, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	rows, err := d.sql.QueryContext(ctx, `
		SELECT strftime('%Y-%m-%d', created_at, 'unixepoch') as day, COALESCE(SUM(amount), 0)
		FROM payments
		WHERE status = 'succeeded' AND created_at >= ?
		GROUP BY day
		ORDER BY day ASC
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*repository.DailyRevenue
	for rows.Next() {
		r := &repository.DailyRevenue{}
		if err := rows.Scan(&r.Day, &r.Revenue); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) GetActiveSubs(ctx context.Context, limit int) ([]*repository.ActiveSubInfo, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT s.user_id, COALESCE(u.username,''), u.first_name, s.device_limit, s.traffic_gb, s.expires_at
		FROM subscriptions s
		JOIN users u ON u.tg_id = s.user_id
		WHERE s.expires_at > unixepoch()
		ORDER BY s.expires_at ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*repository.ActiveSubInfo
	for rows.Next() {
		r := &repository.ActiveSubInfo{}
		if err := rows.Scan(&r.UserID, &r.Username, &r.FirstName, &r.DeviceLimit, &r.TrafficGB, &r.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
