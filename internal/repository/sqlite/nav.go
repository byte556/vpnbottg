// internal/repository/sqlite/nav.go
package sqlite

import (
	"context"
	"fmt"
)

// TrackNav записывает переход пользователя на экран (воронка навигации).
func (d *DB) TrackNav(ctx context.Context, userID int64, screen string) error {
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO nav_events (user_id, screen) VALUES (?, ?)
	`, userID, screen)
	if err != nil {
		return fmt.Errorf("trackNav: %w", err)
	}
	return nil
}
