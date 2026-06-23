package service

import (
	"context"
	"errors"
	"fmt"
	"vpnbottg/internal/config"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

type ReferralService struct {
	refs  repository.Referrals
	subs  *Subscription
	audit repository.Audit
}

func NewReferralService(refs repository.Referrals, subs *Subscription, audit repository.Audit) *ReferralService {
	return &ReferralService{refs: refs, subs: subs, audit: audit}
}

// Record записывает реферала при первом /start с ref-ссылкой.
// Идемпотентно: повторный вызов для того же referee игнорируется (UNIQUE в БД).
func (s *ReferralService) Record(ctx context.Context, refereeID, referrerID int64) error {
	if refereeID == referrerID {
		return nil
	}
	err := s.refs.CreateReferral(ctx, &models.Referral{
		ReferrerID: referrerID,
		RefereeID:  refereeID,
	})
	if err != nil {
		return fmt.Errorf("referral record: %w", err)
	}
	logger.L().Info().Int64("referee", refereeID).Int64("referrer", referrerID).Msg("referral: recorded")
	return nil
}

// GetCount возвращает число рефералов пользователя (пришедших по его ссылке).
func (s *ReferralService) GetCount(ctx context.Context, userID int64) int {
	count, _ := s.refs.GetReferralCount(ctx, userID)
	return count
}

// Reward проверяет наличие невознаграждённого реферала для refereeID,
// продлевает подписку реферрера и помечает реферал как вознаграждённый.
// Возвращает (referrerID, nil) при успехе, (0, nil) если нечего вознаграждать,
// (0, err) при ошибке.
func (s *ReferralService) Reward(ctx context.Context, refereeID int64) (int64, error) {
	log := logger.L().With().Int64("referee_id", refereeID).Logger()

	ref, err := s.refs.GetUnrewardedByReferee(ctx, refereeID)
	if errors.Is(err, repository.ErrNotFound) {
		return 0, nil // нет реферала — не ошибка
	}
	if err != nil {
		return 0, fmt.Errorf("referral reward: %w", err)
	}

	days := config.Cfg.Bot.ReferralRewardDays
	if days <= 0 {
		days = 7
	}

	if err := s.subs.ExtendExpiry(ctx, ref.ReferrerID, days); err != nil {
		log.Error().Err(err).Int64("referrer_id", ref.ReferrerID).Msg("referral reward: ExtendExpiry failed")
		return 0, fmt.Errorf("referral reward extend: %w", err)
	}

	if err := s.refs.MarkReferralRewarded(ctx, ref.ID); err != nil {
		log.Error().Err(err).Msg("referral reward: MarkRewarded failed")
		return 0, fmt.Errorf("referral reward mark: %w", err)
	}

	_ = s.audit.Log(ctx, &ref.ReferrerID, "referral_rewarded",
		fmt.Sprintf(`{"referee_id":%d,"days":%d}`, refereeID, days))

	log.Info().Int64("referrer_id", ref.ReferrerID).Int("days", days).Msg("referral: rewarded")
	return ref.ReferrerID, nil
}
