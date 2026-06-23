package models

type User struct {
	TgID       int64  `repository:"tg_id"`
	Username   string `repository:"username"`
	FirstName  string `repository:"first_name"`
	ReferredBy *int64 `repository:"referred_by"`
	CreatedAt  int64  `repository:"created_at"`
}

type Subscription struct {
	ID              int64  `repository:"id"`
	UserID          int64  `repository:"user_id"`
	XUIEmailDirect  string `repository:"xui_email_direct"`
	XUIEmailRelay   string `repository:"xui_email_relay"`
	XUISubID        string `repository:"xui_sub_id"`
	Bypass          bool   `repository:"bypass"`
	TrafficGB       int    `repository:"traffic_gb"`
	DeviceLimit     int    `repository:"device_limit"`
	StartedAt       int64  `repository:"started_at"`
	ExpiresAt       int64  `repository:"expires_at"`
	CreatedAt       int64  `repository:"created_at"`
	RemindedAt      *int64 `repository:"reminded_at"`
	ExpiredNotified bool   `repository:"expired_notified"`
}

type Payment struct {
	ID                int64  `repository:"id"`
	UserID            int64  `repository:"user_id"`
	Amount            int64  `repository:"amount"`
	Provider          string `repository:"provider"`
	ProviderPaymentID string `repository:"provider_payment_id"`
	Status            string `repository:"status"`
	CreatedAt         int64  `repository:"created_at"`
}

type Referral struct {
	ID         int64  `repository:"id"`
	ReferrerID int64  `repository:"referrer_id"`
	RefereeID  int64  `repository:"referee_id"`
	RewardedAt *int64 `repository:"rewarded_at"`
	CreatedAt  int64  `repository:"created_at"`
}

// DeviceConnection — факт обращения устройства к подписке через sub-сервер.
// DeviceID — это HWID из заголовка x-hwid клиента Happ (по нему идёт трекинг).
// В будущем — для отображения в меню и отвязки устройств, пер-платформенной тарификации.
type DeviceConnection struct {
	ID        int64  `repository:"id"`
	SubID     string `repository:"sub_id"`
	DeviceID  string `repository:"device_id"` // HWID (x-hwid)
	Platform  string `repository:"platform"`
	UserAgent string `repository:"user_agent"`
	FirstSeen int64  `repository:"first_seen"`
	LastSeen  int64  `repository:"last_seen"`
}

// PromoCode — промокод для розыгрышей. RewardType определяет, что даёт активация:
// "days" — бесплатные дни подписки (Days/GB/Devices), "discount" — % скидки (DiscountPct).
// Один код, много активаций до MaxUses; одна активация на пользователя (promo_redemptions).
type PromoCode struct {
	ID          int64  `repository:"id"`
	Code        string `repository:"code"`
	RewardType  string `repository:"reward_type"`
	Days        int    `repository:"days"`
	GB          int    `repository:"gb"`
	Devices     int    `repository:"devices"`
	DiscountPct int    `repository:"discount_pct"`
	MaxUses     int    `repository:"max_uses"`
	UsedCount   int    `repository:"used_count"`
	Active      bool   `repository:"active"`
	ExpiresAt   *int64 `repository:"expires_at"`
	CreatedAt   int64  `repository:"created_at"`
}

const (
	PromoRewardDays     = "days"
	PromoRewardDiscount = "discount"
)

type AuditLog struct {
	ID        int64  `repository:"id"`
	UserID    *int64 `repository:"user_id"`
	Action    string `repository:"action"`
	Payload   string `repository:"payload"`
	CreatedAt int64  `repository:"created_at"`
}
