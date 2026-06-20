// internal/repository/audit.go
package sqlite

import (
	"context"
	"fmt"
)

func (d *DB) Log(ctx context.Context, userID *int64, action, payload string) error {
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO audit_log (user_id, action, payload) VALUES (?, ?, ?)
	`, userID, action, payload)
	if err != nil {
		return fmt.Errorf("auditLog: %w", err)
	}
	return nil
}
