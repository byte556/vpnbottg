package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

var (
	ErrPromoNotFound     = errors.New("promo not found")
	ErrPromoInactive     = errors.New("promo inactive")
	ErrPromoExpired      = errors.New("promo expired")
	ErrPromoLimitReached = errors.New("promo limit reached")
	ErrPromoAlreadyUsed  = errors.New("promo already used")
	ErrPromoExists       = errors.New("promo code already exists")
	ErrPromoInvalid      = errors.New("promo params invalid")
)

type PromoService struct {
	promos repository.PromoCodes
	subs   *Subscription
	audit  repository.Audit
}

func NewPromoService(promos repository.PromoCodes, subs *Subscription, audit repository.Audit) *PromoService {
	return &PromoService{promos: promos, subs: subs, audit: audit}
}

// Типы результата активации (зеркалят models.PromoReward*).
const (
	RedeemTypeDays     = models.PromoRewardDays
	RedeemTypeDiscount = models.PromoRewardDiscount
)

// RedeemResult описывает результат активации промокода для рендера в боте.
// Для Type=days награда уже выдана (SubURL заполнен); для Type=discount скидку
// нужно положить в сессию и списать после оплаты через ConsumeDiscount.
type RedeemResult struct {
	Type        string
	Days        int
	GB          int
	Devices     int
	DiscountPct int
	Code        string
	SubURL      string
}

// Create валидирует и создаёт промокод.
func (s *PromoService) Create(ctx context.Context, p *models.PromoCode) error {
	if p.MaxUses < 1 {
		return ErrPromoInvalid
	}
	switch p.RewardType {
	case models.PromoRewardDays:
		if p.Days < 1 || p.GB < 1 || p.Devices < 1 {
			return ErrPromoInvalid
		}
	case models.PromoRewardDiscount:
		if p.DiscountPct < 1 || p.DiscountPct > 100 {
			return ErrPromoInvalid
		}
	default:
		return ErrPromoInvalid
	}

	id, err := s.promos.CreatePromoCode(ctx, p)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrPromoExists
		}
		return err
	}
	_ = s.audit.Log(ctx, nil, "promo_created",
		fmt.Sprintf(`{"id":%d,"code":%q,"type":%q,"max_uses":%d}`, id, p.Code, p.RewardType, p.MaxUses))
	return nil
}

func (s *PromoService) List(ctx context.Context) ([]*models.PromoCode, error) {
	return s.promos.ListPromoCodes(ctx)
}

func (s *PromoService) Deactivate(ctx context.Context, code string) error {
	err := s.promos.DeactivatePromoCode(ctx, code)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrPromoNotFound
	}
	if err == nil {
		_ = s.audit.Log(ctx, nil, "promo_deactivated", fmt.Sprintf(`{"code":%q}`, strings.ToUpper(code)))
	}
	return err
}

// validate загружает код и проверяет общие условия (активен, не истёк, есть слоты).
func (s *PromoService) validate(ctx context.Context, code string) (*models.PromoCode, error) {
	p, err := s.promos.GetPromoByCode(ctx, code)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrPromoNotFound
	}
	if err != nil {
		return nil, err
	}
	if !p.Active {
		return nil, ErrPromoInactive
	}
	if p.ExpiresAt != nil && *p.ExpiresAt > 0 && *p.ExpiresAt < time.Now().Unix() {
		return nil, ErrPromoExpired
	}
	if p.UsedCount >= p.MaxUses {
		return nil, ErrPromoLimitReached
	}
	return p, nil
}

// Redeem активирует промокод от имени пользователя.
func (s *PromoService) Redeem(ctx context.Context, userID int64, code string) (*RedeemResult, error) {
	log := logger.L().With().Str("func", "Redeem").Int64("user_id", userID).Str("code", strings.ToUpper(code)).Logger()

	p, err := s.validate(ctx, code)
	if err != nil {
		return nil, err
	}

	switch p.RewardType {
	case models.PromoRewardDiscount:
		// Скидка не бронируется здесь — она списывается после оплаты (ConsumeDiscount).
		// Но одну активацию на пользователя проверяем сразу.
		used, err := s.promos.HasRedeemed(ctx, p.ID, userID)
		if err != nil {
			return nil, err
		}
		if used {
			return nil, ErrPromoAlreadyUsed
		}
		log.Info().Int("pct", p.DiscountPct).Msg("Redeem: discount applied to session")
		return &RedeemResult{Type: models.PromoRewardDiscount, DiscountPct: p.DiscountPct, Code: p.Code}, nil

	case models.PromoRewardDays:
		// Бронируем активацию атомарно, затем выдаём подписку. При сбое выдачи — откат.
		if err := s.promos.RedeemPromo(ctx, p.ID, userID); err != nil {
			if errors.Is(err, repository.ErrPromoAlreadyUsed) {
				return nil, ErrPromoAlreadyUsed
			}
			if errors.Is(err, repository.ErrPromoLimitReached) {
				return nil, ErrPromoLimitReached
			}
			return nil, err
		}
		subURL, err := s.subs.ProvisionFromPaymentDays(ctx, userID, p.GB, p.Devices, p.Days)
		if err != nil {
			if relErr := s.promos.ReleasePromo(ctx, p.ID, userID); relErr != nil {
				log.Error().Err(relErr).Msg("Redeem: ReleasePromo failed after provision error")
			}
			return nil, fmt.Errorf("redeem provision: %w", err)
		}
		_ = s.audit.Log(ctx, &userID, "promo_redeemed",
			fmt.Sprintf(`{"code":%q,"days":%d,"gb":%d,"devices":%d}`, p.Code, p.Days, p.GB, p.Devices))
		log.Info().Int("days", p.Days).Msg("Redeem: days granted")
		return &RedeemResult{Type: models.PromoRewardDays, Days: p.Days, GB: p.GB, Devices: p.Devices, Code: p.Code, SubURL: subURL}, nil
	}
	return nil, ErrPromoNotFound
}

// ConsumeDiscount списывает активацию discount-кода после подтверждённой оплаты.
// Best-effort: если код уже списан этим юзером или исчерпан — не ошибка.
func (s *PromoService) ConsumeDiscount(ctx context.Context, userID int64, code string) error {
	p, err := s.promos.GetPromoByCode(ctx, code)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.promos.RedeemPromo(ctx, p.ID, userID); err != nil {
		if errors.Is(err, repository.ErrPromoAlreadyUsed) || errors.Is(err, repository.ErrPromoLimitReached) {
			return nil
		}
		return err
	}
	_ = s.audit.Log(ctx, &userID, "promo_discount_consumed",
		fmt.Sprintf(`{"code":%q,"pct":%d}`, p.Code, p.DiscountPct))
	return nil
}
