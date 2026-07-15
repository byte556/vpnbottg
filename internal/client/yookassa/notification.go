package yookassa

import (
	"encoding/json"
	"fmt"
)

type Notification struct {
	Type   string  `json:"type"`
	Event  string  `json:"event"`
	Object Payment `json:"object"`
}

// ParseNotification парсит тело webhook запроса от YooKassa.
func ParseNotification(body []byte) (*Notification, error) {
	var n Notification
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, fmt.Errorf("parse notification: %w", err)
	}
	return &n, nil
}
