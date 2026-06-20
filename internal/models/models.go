package models

type User struct {
	TgID       int64  `repository:"tg_id"`
	Username   string `repository:"username"`
	FirstName  string `repository:"first_name"`
	ReferredBy *int64 `repository:"referred_by"`
	CreatedAt  int64  `repository:"created_at"`
}

type Subscription struct {
	ID             int64  `repository:"id"`
	UserID         int64  `repository:"user_id"`
	XUIEmailDirect string `repository:"xui_email_direct"`
	XUIEmailRelay  string `repository:"xui_email_relay"`
	Bypass         bool   `repository:"bypass"`
	TrafficGB      int    `repository:"traffic_gb"`
	StartedAt      int64  `repository:"started_at"`
	ExpiresAt      int64  `repository:"expires_at"`
	CreatedAt      int64  `repository:"created_at"`
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

type AuditLog struct {
	ID        int64  `repository:"id"`
	UserID    *int64 `repository:"user_id"`
	Action    string `repository:"action"`
	Payload   string `repository:"payload"`
	CreatedAt int64  `repository:"created_at"`
}
