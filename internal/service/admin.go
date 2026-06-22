package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"vpnbottg/internal/client/xui"
	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

// ErrPaymentNotSucceeded — возврат невозможен: платёж не в статусе succeeded.
var ErrPaymentNotSucceeded = errors.New("payment not succeeded")

type AdminService struct {
	purge    repository.UserPurge
	users    repository.Users
	payments repository.Payments
	stats    repository.Stats
	xui      *xui.Client
	yk       *yookassa.Client
	audit    repository.Audit
}

func NewAdminService(
	purge repository.UserPurge,
	users repository.Users,
	payments repository.Payments,
	stats repository.Stats,
	xuiClient *xui.Client,
	ykClient *yookassa.Client,
	audit repository.Audit,
) *AdminService {
	return &AdminService{
		purge: purge, users: users,
		payments: payments, stats: stats,
		xui: xuiClient, yk: ykClient, audit: audit,
	}
}

// DeleteUser удаляет пользователя: сначала чистит БД (транзакция),
// затем удаляет клиентов из 3X-UI (best-effort — ошибки логируются, не прерывают удаление).
// Возвращает ErrNotFound если пользователь не найден.
func (s *AdminService) DeleteUser(ctx context.Context, tgID int64) error {
	log := logger.L().With().Int64("target_user", tgID).Logger()

	if _, err := s.users.GetUser(ctx, tgID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.ErrNotFound
		}
		return fmt.Errorf("deleteUser: getUser: %w", err)
	}

	log.Info().Msg("deleteUser: purging DB")
	direct, relay, err := s.purge.PurgeUserData(ctx, tgID)
	if err != nil {
		log.Error().Err(err).Msg("deleteUser: PurgeUserData failed")
		return fmt.Errorf("deleteUser: db purge: %w", err)
	}
	log.Info().Str("direct", direct).Str("relay", relay).Msg("deleteUser: DB purged, deleting from XUI")

	if direct != "" {
		if err := s.xui.DeleteClient(ctx, direct); err != nil {
			log.Warn().Err(err).Str("email", direct).Msg("deleteUser: xui DeleteClient direct failed (ignored)")
		}
	}
	if relay != "" && relay != direct {
		if err := s.xui.DeleteClient(ctx, relay); err != nil {
			log.Warn().Err(err).Str("email", relay).Msg("deleteUser: xui DeleteClient relay failed (ignored)")
		}
	}

	_ = s.audit.Log(ctx, nil, "admin_user_deleted", fmt.Sprintf(`{"tg_id":%d}`, tgID))
	log.Info().Msg("deleteUser: ok")
	return nil
}

// RefundPayment создаёт возврат по YooKassa payment ID.
// Возвращает сумму возврата в рублях.
// Ошибки: ErrNotFound (платёж не в БД), ErrPaymentNotSucceeded, или API ошибку.
func (s *AdminService) RefundPayment(ctx context.Context, providerPaymentID string) (int64, error) {
	log := logger.L().With().Str("yk_id", providerPaymentID).Logger()

	p, err := s.payments.GetPaymentByProviderID(ctx, providerPaymentID)
	if err != nil {
		return 0, err // repository.ErrNotFound пробрасывается как есть
	}
	if p.Status != "succeeded" {
		return 0, fmt.Errorf("%w: current status=%s", ErrPaymentNotSucceeded, p.Status)
	}

	log.Info().Int64("amount", p.Amount).Msg("refund: calling YK API")
	refund, err := s.yk.CreateRefund(providerPaymentID, p.Amount)
	if err != nil {
		log.Error().Err(err).Msg("refund: YK API failed")
		return 0, fmt.Errorf("refund: %w", err)
	}

	_ = s.audit.Log(ctx, nil, "admin_refund",
		fmt.Sprintf(`{"yk_id":%q,"refund_id":%q,"amount":%d,"status":%q}`,
			providerPaymentID, refund.ID, p.Amount, refund.Status))

	switch refund.Status {
	case "succeeded", "pending":
		if err := s.payments.MarkRefunded(ctx, providerPaymentID); err != nil {
			log.Error().Err(err).Msg("refund: MarkRefunded failed")
		}
		log.Info().Str("refund_id", refund.ID).Msg("refund: ok")
		return p.Amount, nil
	default:
		log.Warn().Str("refund_status", refund.Status).Msg("refund: provider rejected")
		return 0, fmt.Errorf("refund rejected by provider: status=%s", refund.Status)
	}
}

// SearchUser ищет пользователя по @username или tg_id (число).
func (s *AdminService) SearchUser(ctx context.Context, query string) (*models.User, error) {
	if id, err := strconv.ParseInt(query, 10, 64); err == nil {
		return s.users.GetUser(ctx, id)
	}
	username := strings.TrimPrefix(query, "@")
	return s.users.SearchUserByUsername(ctx, username)
}

// GetAllUserIDs возвращает все tg_id из таблицы users.
func (s *AdminService) GetAllUserIDs(ctx context.Context) ([]int64, error) {
	return s.users.GetAllUserIDs(ctx)
}

// GetStats возвращает агрегированную статистику.
func (s *AdminService) GetStats() repository.Stats {
	return s.stats
}

// BroadcastAll рассылает text всем пользователям через send.
// Возвращает (sent, errors). Встроенный rate limit: пауза 1 сек каждые 25 сообщений.
func (s *AdminService) BroadcastAll(ctx context.Context, text string, send func(int64, string) error) (sent, errs int) {
	ids, err := s.users.GetAllUserIDs(ctx)
	if err != nil {
		logger.L().Error().Err(err).Msg("broadcast: GetAllUserIDs failed")
		return 0, 1
	}
	for i, id := range ids {
		if ctx.Err() != nil {
			break
		}
		if err := send(id, text); err != nil {
			errs++
		} else {
			sent++
		}
		if (i+1)%25 == 0 {
			time.Sleep(time.Second)
		}
	}
	return sent, errs
}
