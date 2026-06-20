package service

import (
	"context"
	"errors"
	"fmt"
	"time"
	"vpnbottg/internal/client/xui"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

type Subscription struct {
	subs  repository.Subscriptions
	audit repository.Audit
	xui   *xui.Client
}

func NewSubscriptionService(subs repository.Subscriptions, audit repository.Audit, xui *xui.Client) *Subscription {
	return &Subscription{subs: subs, audit: audit, xui: xui}
}

func (s *Subscription) Create(ctx context.Context, userID int64, planDays, trafficGB int, inboundIDs []int) (*models.Subscription, error) {
	log := logger.L().With().Int64("user_id", userID).Logger()

	now := time.Now()
	expiresAt := now.AddDate(0, 0, planDays)
	email := fmt.Sprintf("u%d", userID)
	emailRelay := email + "r"

	if err := s.xui.AddClient(ctx, email, trafficGB, expiresAt, 0, inboundIDs); err != nil {
		log.Error().Err(err).Msg("createSubscription: xui addClient failed")
		return nil, fmt.Errorf("createSubscription: %w", err)
	}

	sub := &models.Subscription{
		UserID:         userID,
		XUIEmailDirect: email,
		XUIEmailRelay:  emailRelay,
		Bypass:         false,
		TrafficGB:      trafficGB,
		StartedAt:      now.Unix(),
		ExpiresAt:      expiresAt.Unix(),
	}
	id, err := s.subs.CreateSubscription(ctx, sub)
	if err != nil {
		log.Error().Err(err).Msg("createSubscription: db failed")
		return nil, fmt.Errorf("createSubscription: %w", err)
	}
	sub.ID = id

	_ = s.audit.Log(ctx, &userID, "subscription_created", fmt.Sprintf(`{"sub_id":%d,"days":%d,"traffic_gb":%d}`, id, planDays, trafficGB))

	log.Info().Int64("sub_id", id).Int("days", planDays).Msg("createSubscription: ok")
	return sub, nil
}

// Renew продлевает подписку — обновляет xui и expires_at в db.
func (s *Subscription) Renew(ctx context.Context, userID int64, addDays, trafficGB int) error {
	log := logger.L().With().Int64("user_id", userID).Logger()

	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.ErrNotFound
		}
		return fmt.Errorf("renew: %w", err)
	}

	newExpiry := time.Unix(sub.ExpiresAt, 0).AddDate(0, 0, addDays)

	if err := s.xui.UpdateClientByEmail(ctx, sub.XUIEmailDirect, trafficGB, newExpiry); err != nil {
		log.Error().Err(err).Msg("renew: xui update failed")
		return fmt.Errorf("renew: %w", err)
	}

	_ = s.audit.Log(ctx, &userID, "subscription_renewed", fmt.Sprintf(`{"sub_id":%d,"add_days":%d}`, sub.ID, addDays))

	log.Info().Int64("sub_id", sub.ID).Int("add_days", addDays).Msg("renew: ok")
	return nil
}

// GetActive возвращает активную подписку или ErrNotFound.
func (s *Subscription) GetActive(ctx context.Context, userID int64) (*models.Subscription, error) {
	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("getActive: %w", err)
	}
	return sub, nil
}
