package sqlite

import (
	"context"
	"testing"
	"time"
	"vpnbottg/internal/models"
)

func TestAuthorizeDeviceConnection(t *testing.T) {
	ctx := context.Background()
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	if err := db.UpsertUser(ctx, &models.User{TgID: 1001, Username: "u", FirstName: "User"}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if _, err = db.CreateSubscription(ctx, &models.Subscription{
		UserID:         1001,
		XUIEmailDirect: "u1001",
		XUIEmailRelay:  "u1001r",
		XUISubID:       "sub-1",
		TrafficGB:      10,
		DeviceLimit:    2,
		StartedAt:      time.Now().Unix(),
		ExpiresAt:      time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}

	count := func() int {
		devices, err := db.ListDeviceConnections(ctx, "sub-1")
		if err != nil {
			t.Fatalf("ListDeviceConnections: %v", err)
		}
		return len(devices)
	}
	authorize := func(hwid, ua, platform string) bool {
		allowed, err := db.AuthorizeDeviceConnection(ctx, "sub-1", hwid, ua, platform)
		if err != nil {
			t.Fatalf("AuthorizeDeviceConnection(%s): %v", hwid, err)
		}
		return allowed
	}

	// Новые устройства разрешаются, пока не достигнут device_limit (=2).
	if !authorize("hwid-a", "Happ/1.0 iPhone", "iOS") {
		t.Fatal("hwid-a should be allowed (under limit)")
	}
	if !authorize("hwid-b", "Happ/1.0 Pixel", "Android") {
		t.Fatal("hwid-b should be allowed (under limit)")
	}
	if got := count(); got != 2 {
		t.Fatalf("after 2 devices got %d, want 2", got)
	}

	// Третье НОВОЕ устройство сверх лимита — отказ, не записывается.
	if authorize("hwid-c", "Happ/1.0 Mac", "macOS") {
		t.Fatal("hwid-c should be denied (over limit)")
	}
	if got := count(); got != 2 {
		t.Fatalf("new device over limit got %d devices, want 2", got)
	}

	// Уже зареганное устройство всегда разрешено и обновляется даже на полном лимите.
	if !authorize("hwid-a", "Happ/2.0 iPhone", "iOS") {
		t.Fatal("known hwid-a should stay allowed at full limit")
	}
	devices, _ := db.ListDeviceConnections(ctx, "sub-1")
	if len(devices) != 2 {
		t.Fatalf("after re-auth got %d devices, want 2 (upsert)", len(devices))
	}
	var aID int64
	for _, dc := range devices {
		if dc.DeviceID == "hwid-a" {
			aID = dc.ID
			if dc.UserAgent != "Happ/2.0 iPhone" {
				t.Errorf("user_agent not updated on upsert: %q", dc.UserAgent)
			}
		}
	}

	// Отвязали устройство в ТГ → освободился слот → новое устройство снова проходит.
	if err := db.DeleteDeviceConnectionByID(ctx, "sub-1", aID); err != nil {
		t.Fatalf("DeleteDeviceConnectionByID: %v", err)
	}
	if got := count(); got != 1 {
		t.Fatalf("after unlink got %d devices, want 1", got)
	}
	if !authorize("hwid-c", "Happ/1.0 Mac", "macOS") {
		t.Fatal("hwid-c should be allowed after freeing a slot")
	}
	if got := count(); got != 2 {
		t.Fatalf("after freeing slot got %d devices, want 2", got)
	}
}
