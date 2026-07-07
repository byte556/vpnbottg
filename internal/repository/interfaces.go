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
	SearchUserByUsername(ctx context.Context, username string) (*models.User, error)
	GetAllUserIDs(ctx context.Context) ([]int64, error)
}

type Subscriptions interface {
	CreateSubscription(ctx context.Context, s *models.Subscription) (int64, error)
	GetActiveSubscription(ctx context.Context, userID int64) (*models.Subscription, error)
	UpdateSubscriptionExpiry(ctx context.Context, subID int64, expiresAt int64) error
	UpdateSubscriptionDevices(ctx context.Context, subID int64, deviceLimit, trafficGB int) error
	UpdateSubscriptionXUISubID(ctx context.Context, subID int64, xuiSubID string) error
	GetExpiringUnreminded(ctx context.Context, from, to int64) ([]*models.Subscription, error)
	GetRecentlyExpiredUnnotified(ctx context.Context, since int64) ([]*models.Subscription, error)
	MarkReminded(ctx context.Context, subID int64) error
	MarkExpiredNotified(ctx context.Context, subID int64) error
	// GetUserIDBySubID возвращает tg_id владельца активной подписки по xui_sub_id.
	GetUserIDBySubID(ctx context.Context, xuiSubID string) (int64, error)
}

type Payments interface {
	CreatePayment(p *models.Payment) (int64, error)
	UpdatePaymentStatus(ctx context.Context, providerPaymentID, status string) (bool, error)
	GetPaymentByProviderID(ctx context.Context, providerPaymentID string) (*models.Payment, error)
	// GetPendingPayments возвращает платежи со статусом pending, созданные за последние 2 часа.
	GetPendingPayments(ctx context.Context) ([]*models.Payment, error)
	// HasSucceededPayment возвращает true если у пользователя есть хотя бы один успешный платёж.
	HasSucceededPayment(ctx context.Context, userID int64) (bool, error)
	// GetLastSucceededPayment возвращает последний успешный платёж пользователя или ErrNotFound.
	GetLastSucceededPayment(ctx context.Context, userID int64) (*models.Payment, error)
	// MarkRefunded помечает платёж как возвращённый.
	MarkRefunded(ctx context.Context, providerPaymentID string) error
}

type Referrals interface {
	// CreateReferral записывает реферала — INSERT OR IGNORE (идемпотентно).
	CreateReferral(ctx context.Context, r *models.Referral) error
	// GetUnrewardedByReferee возвращает невознаграждённую запись для данного реферала.
	GetUnrewardedByReferee(ctx context.Context, refereeID int64) (*models.Referral, error)
	// MarkReferralRewarded проставляет rewarded_at = now().
	MarkReferralRewarded(ctx context.Context, referralID int64) error
	GetReferralCount(ctx context.Context, referrerID int64) (int, error)
}

type Audit interface {
	Log(ctx context.Context, userID *int64, action, payload string) error
}

// DeviceConnections — учёт и лимитирование устройств подписки.
// Идентификация по HWID (x-hwid от Happ); device_id хранит HWID.
type DeviceConnections interface {
	// AuthorizeDeviceConnection — проверка device_limit + upsert по (sub_id, device_id=HWID).
	// allowed=true, если устройство уже зарегано или лимит ещё не достигнут;
	// новое устройство сверх лимита не пишется и получает allowed=false.
	// isNew=true, если устройство было зарегистрировано впервые (не обновление existing).
	AuthorizeDeviceConnection(ctx context.Context, subID, deviceID, userAgent, platform string) (allowed, isNew bool, err error)
	ListDeviceConnections(ctx context.Context, subID string) ([]*models.DeviceConnection, error)
	DeleteDeviceConnectionByID(ctx context.Context, subID string, deviceConnID int64) error
	// DeleteDeviceConnectionsBySubID удаляет все устройства подписки
	// (свежие слоты при выдаче новой подписки с тем же subId).
	DeleteDeviceConnectionsBySubID(ctx context.Context, subID string) error
}

// ErrPromoLimitReached — у промокода исчерпан лимит активаций (used_count >= max_uses) или он неактивен.
// ErrPromoAlreadyUsed — пользователь уже активировал этот промокод.
var (
	ErrPromoLimitReached = errors.New("promo limit reached")
	ErrPromoAlreadyUsed  = errors.New("promo already used by user")
)

// PromoCodes — промокоды для розыгрышей (см. models.PromoCode).
type PromoCodes interface {
	CreatePromoCode(ctx context.Context, p *models.PromoCode) (int64, error)
	GetPromoByCode(ctx context.Context, code string) (*models.PromoCode, error)
	ListPromoCodes(ctx context.Context) ([]*models.PromoCode, error)
	DeactivatePromoCode(ctx context.Context, code string) error
	// RedeemPromo атомарно бронирует одну активацию: вставляет redemption (UNIQUE →
	// ErrPromoAlreadyUsed) и инкрементит used_count при used_count<max_uses AND active
	// (иначе ErrPromoLimitReached). Обе ошибки — типизированные.
	RedeemPromo(ctx context.Context, promoID, userID int64) error
	// ReleasePromo откатывает бронь (удаляет redemption + декрементит used_count),
	// если выдача награды после RedeemPromo не удалась.
	ReleasePromo(ctx context.Context, promoID, userID int64) error
	// HasRedeemed — активировал ли пользователь этот промокод ранее.
	HasRedeemed(ctx context.Context, promoID, userID int64) (bool, error)
}

// BotStats — агрегированная статистика для админ панели.
type BotStats struct {
	TotalUsers       int
	ActiveSubs       int
	TotalRevenue     int64 // рублей
	TodayRevenue     int64 // рублей
	OrphanedPayments int   // оплачено, но VPN не выдан
}

// RecentPayment — строка из последних платежей.
type RecentPayment struct {
	Amount    int64
	Username  string
	FirstName string
	CreatedAt int64
}

// ActiveSubInfo — строка из списка активных подписок.
type ActiveSubInfo struct {
	UserID      int64
	Username    string
	FirstName   string
	DeviceLimit int
	TrafficGB   int
	ExpiresAt   int64
}

// OrphanedPaymentInfo — платёж прошёл, но VPN не выдан.
type OrphanedPaymentInfo struct {
	UserID            int64
	Username          string
	FirstName         string
	Amount            int64
	ProviderPaymentID string
	CreatedAt         int64
}

// UserPurge — атомарное удаление всех данных пользователя из БД.
// Возвращает XUI emails (direct, relay) для последующего удаления из панели.
type UserPurge interface {
	PurgeUserData(ctx context.Context, tgID int64) (xuiEmailDirect, xuiEmailRelay string, err error)
}

// DailyRevenue — выручка за один день (для графика).
type DailyRevenue struct {
	Day     string // "MM-DD"
	Revenue int64  // рублей
}

type Stats interface {
	GetBotStats(ctx context.Context) (*BotStats, error)
	GetRecentPayments(ctx context.Context, limit int) ([]*RecentPayment, error)
	GetActiveSubs(ctx context.Context, limit int) ([]*ActiveSubInfo, error)
	GetOrphanedPayments(ctx context.Context, limit int) ([]*OrphanedPaymentInfo, error)
	GetDailyRevenue(ctx context.Context, days int) ([]*DailyRevenue, error)
}
