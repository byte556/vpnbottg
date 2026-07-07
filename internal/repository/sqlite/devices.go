package sqlite

import (
	"context"
	"fmt"
	"vpnbottg/internal/models"
)

// AuthorizeDeviceConnection — проверка лимита + учёт устройства по HWID (x-hwid).
// Регистрируем/обновляем и возвращаем allowed=true, если устройство уже зарегано
// (тогда обновляем last_seen/platform/user_agent) ИЛИ ещё не достигнут device_limit.
// Новое устройство сверх лимита не записывается и получает allowed=false — хэндлер
// в этом случае не отдаёт конфиг. Слот освобождается отвязкой устройства в ТГ.
// device_limit <= 0 трактуется как безлимит. Один атомарный запрос — без гонок:
// RowsAffected > 0 значит запись прошла (вставка нового или обновление известного).
func (d *DB) AuthorizeDeviceConnection(ctx context.Context, subID, deviceID, userAgent, platform string) (allowed, isNew bool, err error) {
	var exists int
	_ = d.sql.QueryRowContext(ctx,
		`SELECT 1 FROM device_connections WHERE sub_id = ? AND device_id = ?`,
		subID, deviceID,
	).Scan(&exists)
	known := exists == 1

	res, err := d.sql.ExecContext(ctx, `
		INSERT INTO device_connections (sub_id, device_id, platform, user_agent, first_seen, last_seen)
		SELECT ?, ?, ?, ?, unixepoch(), unixepoch()
		WHERE EXISTS (
			SELECT 1 FROM device_connections WHERE sub_id = ? AND device_id = ?
		)
		OR EXISTS (
			SELECT 1 FROM (
				SELECT device_limit FROM subscriptions
				WHERE xui_sub_id = ? AND expires_at > unixepoch()
				ORDER BY expires_at DESC LIMIT 1
			) AS active_sub
			WHERE active_sub.device_limit <= 0
				OR (SELECT COUNT(*) FROM device_connections WHERE sub_id = ?) < active_sub.device_limit
		)
		ON CONFLICT(sub_id, device_id) DO UPDATE SET
			last_seen  = unixepoch(),
			platform   = excluded.platform,
			user_agent = excluded.user_agent
	`, subID, deviceID, platform, userAgent, subID, deviceID, subID, subID)
	if err != nil {
		return false, false, fmt.Errorf("authorizeDeviceConnection: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, false, fmt.Errorf("authorizeDeviceConnection rows: %w", err)
	}
	return n > 0, n > 0 && !known, nil
}

func (d *DB) ListDeviceConnections(ctx context.Context, subID string) ([]*models.DeviceConnection, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, sub_id, device_id, platform, user_agent, first_seen, last_seen
		FROM device_connections
		WHERE sub_id = ?
		ORDER BY last_seen DESC, id DESC
	`, subID)
	if err != nil {
		return nil, fmt.Errorf("listDeviceConnections: %w", err)
	}
	defer rows.Close()

	var result []*models.DeviceConnection
	for rows.Next() {
		var dc models.DeviceConnection
		if err := rows.Scan(&dc.ID, &dc.SubID, &dc.DeviceID, &dc.Platform, &dc.UserAgent, &dc.FirstSeen, &dc.LastSeen); err != nil {
			return nil, fmt.Errorf("listDeviceConnections scan: %w", err)
		}
		result = append(result, &dc)
	}
	return result, rows.Err()
}

// DeleteDeviceConnectionsBySubID удаляет все устройства подписки —
// используется при выдаче новой подписки с тем же subId (свежие слоты).
func (d *DB) DeleteDeviceConnectionsBySubID(ctx context.Context, subID string) error {
	_, err := d.sql.ExecContext(ctx, `
		DELETE FROM device_connections
		WHERE sub_id = ?
	`, subID)
	if err != nil {
		return fmt.Errorf("deleteDeviceConnectionsBySubID: %w", err)
	}
	return nil
}

func (d *DB) DeleteDeviceConnectionByID(ctx context.Context, subID string, deviceConnID int64) error {
	_, err := d.sql.ExecContext(ctx, `
		DELETE FROM device_connections
		WHERE sub_id = ? AND id = ?
	`, subID, deviceConnID)
	if err != nil {
		return fmt.Errorf("deleteDeviceConnectionByID: %w", err)
	}
	return nil
}

// CountDeviceConnections возвращает число зарегистрированных устройств подписки.
func (d *DB) CountDeviceConnections(ctx context.Context, subID string) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM device_connections WHERE sub_id = ?`, subID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("countDeviceConnections: %w", err)
	}
	return n, nil
}

// DeleteNewestDeviceConnections удаляет n самых новых устройств подписки
// (по last_seen, затем id). Используется при уменьшении лимита устройств —
// освобождаем слоты сверх нового лимита, начиная с недавно подключённых.
func (d *DB) DeleteNewestDeviceConnections(ctx context.Context, subID string, n int) error {
	if n <= 0 {
		return nil
	}
	_, err := d.sql.ExecContext(ctx, `
		DELETE FROM device_connections
		WHERE id IN (
			SELECT id FROM device_connections
			WHERE sub_id = ?
			ORDER BY last_seen DESC, id DESC
			LIMIT ?
		)
	`, subID, n)
	if err != nil {
		return fmt.Errorf("deleteNewestDeviceConnections: %w", err)
	}
	return nil
}
