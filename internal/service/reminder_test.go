package service

import (
	"strings"
	"testing"

	"vpnbottg/internal/telegram/texts"
)

func TestExpiryTextKey(t *testing.T) {
	cases := map[int]string{
		1:  "reminder.expiry_1",
		2:  "reminder.expiry_2",
		3:  "reminder.expiry_3",
		5:  "reminder.expiry",
		10: "reminder.expiry",
	}
	for days, want := range cases {
		if got := expiryTextKey(days); got != want {
			t.Errorf("expiryTextKey(%d) = %q, want %q", days, got, want)
		}
	}
}

// Гарантируем, что все ключи напоминаний существуют в ru.toml и рендерятся
// без [missing: ...] / [template ...] — иначе юзер получит мусор вместо текста.
func TestReminderTextsResolve(t *testing.T) {
	if err := texts.Load(); err != nil {
		t.Fatalf("texts.Load: %v", err)
	}
	keys := []string{
		"reminder.expiry_1", "reminder.expiry_2", "reminder.expiry_3",
		"reminder.expiry", "reminder.expired",
	}
	for _, k := range keys {
		out := texts.T(k, map[string]any{"Days": 3})
		if strings.HasPrefix(out, "[") {
			t.Errorf("text %q did not resolve: %q", k, out)
		}
	}
}
