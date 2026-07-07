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
	users repository.Users
	audit repository.Audit
}

func NewReferralService(refs repository.Referrals, users repository.Users, audit repository.Audit) *ReferralService {
	return &ReferralService{refs: refs, users: users, audit: audit}
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

// RewardBalance начисляет referrer-у процент от оплаты реферала на баланс.
// Возвращает (referrerID, rewardRub, nil) при успехе, (0, 0, nil) если нет реферала.
func (s *ReferralService) RewardBalance(ctx context.Context, refereeID, paymentRub int64) (int64, int64, error) {
	log := logger.L().With().Int64("referee_id", refereeID).Int64("payment_rub", paymentRub).Logger()

	referrerID, err := s.refs.GetReferrerByReferee(ctx, refereeID)
	if errors.Is(err, repository.ErrNotFound) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("referral reward: %w", err)
	}

	pct := config.Cfg.Bot.ReferralRewardPct
	if pct <= 0 {
		pct = 50
	}
	rewardRub := paymentRub * int64(pct) / 100
	if rewardRub <= 0 {
		return 0, 0, nil
	}

	if err := s.users.AddBalance(ctx, referrerID, rewardRub); err != nil {
		log.Error().Err(err).Int64("referrer_id", referrerID).Msg("referral reward: AddBalance failed")
		return 0, 0, fmt.Errorf("referral reward balance: %w", err)
	}

	_ = s.audit.Log(ctx, &referrerID, "referral_balance",
		fmt.Sprintf(`{"referee_id":%d,"payment_rub":%d,"reward_rub":%d}`, refereeID, paymentRub, rewardRub))

	log.Info().Int64("referrer_id", referrerID).Int64("reward_rub", rewardRub).Msg("referral: balance rewarded")
	return referrerID, rewardRub, nil
}
