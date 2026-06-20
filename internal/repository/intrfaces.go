package repository

import (
	"context"
	"errors"
	"vpnbottg/internal/models"
)

var ErrNotFound = errors.New("not found")

type Users interface {
	UpsertUser(ctx context.Context, u *models.User) error
	GetUser(ctx context.Context, tgID int64) (*models.User, error)
}

type Subscriptions interface {
	CreateSubscription(ctx context.Context, s *models.Subscription) (int64, error)
	GetActiveSubscription(ctx context.Context, userID int64) (*models.Subscription, error)
}

type Payments interface {
	CreatePayment(p *models.Payment) (int64, error)
	UpdatePaymentStatus(ctx context.Context, providerPaymentID, status string) error
	GetPaymentByProviderID(ctx context.Context, providerPaymentID string) (*models.Payment, error)
}

type Referrals interface {
	CreateReferral(ctx context.Context, r *models.Referral) error
	GetReferralCount(ctx context.Context, referrerID int64) (int, error)
}

type Audit interface {
	Log(ctx context.Context, userID *int64, action, payload string) error
}
