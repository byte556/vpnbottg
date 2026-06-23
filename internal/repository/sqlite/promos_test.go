package sqlite

import (
	"context"
	"errors"
	"testing"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

func TestPromoRedeemFlow(t *testing.T) {
	ctx := context.Background()
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	id, err := db.CreatePromoCode(ctx, &models.PromoCode{
		Code:       "gift",
		RewardType: models.PromoRewardDays,
		Days:       30,
		GB:         50,
		Devices:    2,
		MaxUses:    2,
		Active:     true,
	})
	if err != nil {
		t.Fatalf("CreatePromoCode: %v", err)
	}

	// Код хранится в UPPER и читается без учёта регистра.
	p, err := db.GetPromoByCode(ctx, "GiFt")
	if err != nil {
		t.Fatalf("GetPromoByCode: %v", err)
	}
	if p.Code != "GIFT" || p.ID != id || !p.Active {
		t.Fatalf("unexpected promo: %+v", p)
	}

	// Первая активация юзером 1 — успех.
	if err := db.RedeemPromo(ctx, id, 1); err != nil {
		t.Fatalf("RedeemPromo(user 1): %v", err)
	}
	// Повторная активация тем же юзером — already used.
	if err := db.RedeemPromo(ctx, id, 1); !errors.Is(err, repository.ErrPromoAlreadyUsed) {
		t.Fatalf("RedeemPromo(user 1 again): want ErrPromoAlreadyUsed, got %v", err)
	}
	// Второй юзер исчерпывает лимит (max_uses=2).
	if err := db.RedeemPromo(ctx, id, 2); err != nil {
		t.Fatalf("RedeemPromo(user 2): %v", err)
	}
	// Третий юзер — лимит исчерпан.
	if err := db.RedeemPromo(ctx, id, 3); !errors.Is(err, repository.ErrPromoLimitReached) {
		t.Fatalf("RedeemPromo(user 3): want ErrPromoLimitReached, got %v", err)
	}

	p, _ = db.GetPromoByCode(ctx, "GIFT")
	if p.UsedCount != 2 {
		t.Fatalf("UsedCount = %d, want 2", p.UsedCount)
	}

	// HasRedeemed отражает состояние.
	if ok, _ := db.HasRedeemed(ctx, id, 1); !ok {
		t.Fatal("HasRedeemed(user 1) = false, want true")
	}
	if ok, _ := db.HasRedeemed(ctx, id, 99); ok {
		t.Fatal("HasRedeemed(user 99) = true, want false")
	}

	// Release откатывает бронь user 2: счётчик уменьшается, повторный redeem возможен.
	if err := db.ReleasePromo(ctx, id, 2); err != nil {
		t.Fatalf("ReleasePromo(user 2): %v", err)
	}
	p, _ = db.GetPromoByCode(ctx, "GIFT")
	if p.UsedCount != 1 {
		t.Fatalf("after release UsedCount = %d, want 1", p.UsedCount)
	}
	if err := db.RedeemPromo(ctx, id, 2); err != nil {
		t.Fatalf("RedeemPromo(user 2 after release): %v", err)
	}

	// Деактивация блокирует новые активации (через used_count<max уже исчерпан,
	// поэтому используем свежий код).
	if err := db.DeactivatePromoCode(ctx, "GIFT"); err != nil {
		t.Fatalf("DeactivatePromoCode: %v", err)
	}
	p, _ = db.GetPromoByCode(ctx, "GIFT")
	if p.Active {
		t.Fatal("promo still active after deactivate")
	}
	if err := db.DeactivatePromoCode(ctx, "NOPE"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("DeactivatePromoCode(NOPE): want ErrNotFound, got %v", err)
	}
}

func TestPromoUniqueCode(t *testing.T) {
	ctx := context.Background()
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	if _, err := db.CreatePromoCode(ctx, &models.PromoCode{
		Code: "DUP", RewardType: models.PromoRewardDiscount, DiscountPct: 50, MaxUses: 1, Active: true,
	}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err = db.CreatePromoCode(ctx, &models.PromoCode{
		Code: "dup", RewardType: models.PromoRewardDiscount, DiscountPct: 10, MaxUses: 1, Active: true,
	})
	if err == nil {
		t.Fatal("duplicate code should fail")
	}
}
