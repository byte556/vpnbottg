# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**VPN Subscription Bot** — A Telegram bot that automates VPN subscription management, pricing, payment processing, and 3X-UI panel integration. Users customize subscriptions (GB, devices, duration), pay via YooKassa, and receive VPN configurations.

## Tech Stack

- **Language**: Go 1.24
- **Telegram Bot**: [telebot.v3](https://gopkg.in/telebot.v3) (long polling)
- **Database**: SQLite3 with WAL mode
- **Payment Gateway**: YooKassa (Russian payment processor)
- **VPN Panel**: 3X-UI v2.6+ REST API
- **Logging**: zerolog (structured JSON logging)
- **Config**: YAML (gopkg.in/yaml.v3)
- **Database Driver**: github.com/jmoiron/sqlx

## Database Schema

All timestamps are Unix epochs. Foreign keys enabled (`_foreign_keys=on`), WAL mode for concurrent writes.

### users
```
tg_id (INT PRIMARY KEY) — Telegram user ID
username (TEXT) — Telegram username
first_name (TEXT) — User's first name
referred_by (INT FK) — Referrer's tg_id (nullable)
created_at (INT) — Registration time
```

### subscriptions
```
id (INT PK AUTOINCREMENT)
user_id (INT FK users.tg_id)
xui_email_direct (TEXT) — Email in direct 3X-UI inbound
xui_email_relay (TEXT) — Email in relay 3X-UI inbound
bypass (BOOL) — Whether subscription allows bypass mode
traffic_gb (INT) — Total traffic limit in GB
started_at (INT) — Subscription start time
expires_at (INT) — Subscription expiry time
created_at (INT)
```

### payments
```
id (INT PK AUTOINCREMENT)
user_id (INT FK)
amount (INT) — Amount in rubles (kopek precision as int)
provider (TEXT) — "yookassa" (extensible for other providers)
provider_payment_id (TEXT) — YooKassa payment UUID
status (TEXT) — pending|succeeded|canceled
created_at (INT)
```

### audit_log
```
id (INT PK AUTOINCREMENT)
user_id (INT FK, nullable) — NULL for system actions
action (TEXT) — Action name: payment_succeeded, etc.
payload (TEXT) — JSON string with additional context
created_at (INT)
```

### referrals (referenced in models but not yet created in migration)
```
id (INT PK AUTOINCREMENT)
referrer_id (INT FK users.tg_id)
referee_id (INT FK users.tg_id)
rewarded_at (INT, nullable) — When reward was paid
created_at (INT)
```

## FSM (Finite State Machine)

Telegram user session state is managed in-memory via `internal/telegram/fsm/fsm.go`:

### States
- **StateMenu** (`"menu"`) — Main menu, awaiting command selection
- **StateConstructor** (`"constructor"`) — VPN subscription builder (interactive)

### ConstructorState
User selects subscription parameters:
- **planGB** (int) — GB of traffic, default 30
- **devices** (int) — Concurrent connections allowed, default 1, min 1
- **months** (int) — Subscription duration, must be in config's month_discounts

### Pricing Calculation
```
price = (base_multiplier × planGB + fix_price) × devices × months × month_discount[months]
```
- `fix_price`: Fixed cost per subscription (config: 50 RUB)
- `base_multiplier`: Per-GB cost (config: 1.0)
- `month_discounts`: Map of months → discount multiplier (1→1.0, 3→0.9, 6→0.8, 12→0.54)
- `whitelist_multiplier`: Currently unused (1.2 in config)

Session storage is thread-safe (sync.Mutex) and in-memory only; survives bot restart only for active connections.

## Payment Flow (YooKassa Integration)

### Initiate Payment (`PaymentService.InitiatePayment`)
1. User enters StateConstructor, selects GB/devices/months, calculates price
2. Bot calls `PaymentService.InitiatePayment(userID, CreatePaymentReq{...})`
3. YooKassa client creates payment via `POST /v3/payments` with:
   - Amount in RUB (format: "50.00")
   - Metadata: `{"tg_id": "123456", ...}` (preserved in webhook)
   - Confirmation type: "redirect" (user visits YooKassa to enter card details)
   - Idempotence-Key: random UUID (prevents double-charge if network fails)
4. Payment record inserted into DB with status "pending" and YooKassa payment_id
5. Bot sends user confirmation URL (yk_payment.ConfirmationURL)

### Confirm Payment (Webhook)
1. YooKassa sends POST webhook to configured webhook_url with `notification`:
   ```json
   {
     "type": "notification",
     "event": "payment.succeeded",
     "object": {
       "id": "yk_payment_id",
       "status": "succeeded",
       "paid": true,
       "metadata": {"tg_id": "123456", ...}
     }
   }
   ```
2. `Webhook(bot)` handler extracts tg_id from metadata
3. `PaymentService.Confirm(ctx, yk_payment_id)` updates DB status to "succeeded" and logs audit entry
4. Bot sends user message "✅ Оплата прошла! Ваш конфиг VPN готов." (TODO: actually provision VPN)

### Saved Cards (Future)
YooKassa can return `PaymentMethodID` if user opts to save their card (`SaveMethod: true`). Subsequent payments can use `PaymentMethodID` for one-click checkout (no redirect).

## 3X-UI API Integration

`internal/client/xui/client.go` — Async HTTP client for 3X-UI v2.6+ panel.

### Configuration
```go
xui.NewClient(
  host,       // e.g., "https://panel.example.com:54321"
  xuiPath,    // e.g., "" or "xui" (appended to host if provided)
  token,      // Panel authentication token (Bearer token)
)
```

### Key Operations

**AddClient** — Create new VPN user in panel
- Email: generated identifier (e.g., "user_123456@vpn.local")
- InboundIDs: list of inbound configs to attach to (direct, relay, etc.)
- TotalGB: traffic quota in GB (0 = unlimited)
- ExpiryTime: subscription expiry Unix timestamp (zero = no expiry)
- LimitIP: concurrent connection limit (0 = unlimited)

**GetClient** — Fetch user and inbound assignments
- Returns UUID, SubID (subscription link), enable status, traffic, inbound IDs
- Error: `ErrClientNotFound` if email not in panel

**UpdateClientByEmail** — Extend traffic or expiry
- Fetches current client state first (panel does replace, not patch)
- Updates totalGB and expiryTime in one call
- Preserves UUID, flow, SubID, other fields

**GetClientTraffic** — Monitor usage
- Returns up/down/total traffic in bytes
- Used for alerting users near quota

### Inbound IDs
Inbound IDs are hardcoded or configured elsewhere (not yet in bot config). Current code assumes:
- Direct inbound: standard routing
- Relay inbound: alternative routing (potential bypass)

Subscriptions store `xui_email_direct` and `xui_email_relay` to track which inbound(s) each user is assigned to.

## Key Packages and Responsibilities

### cmd/main.go
Entry point. Initializes:
1. Texts (TOML localization)
2. Config (YAML)
3. SQLite DB with auto-migration
4. YooKassa client
5. Payment service
6. Telegram bot and handler registration
7. Blocks on `bot.Start()` (long polling)

### internal/config
- **config.go**: YAML struct and `Load()` function
- **Config structure**:
  ```yaml
  bot:
    token: "..."
    constructor:
      fix_price: 50
      base_multiplier: 1.0
      month_discounts: {1: 1.0, 3: 0.9, 6: 0.8, 12: 0.54}
  yookassa:
    shop_id: "..."
    secret_key: "..."
    webhook_url: "https://yourdomain.com/webhook/yookassa"
  ```

### internal/models
Struct definitions matching database schema (tagged with `repository:"col_name"`):
- **User**: TgID, Username, FirstName, ReferredBy, CreatedAt
- **Subscription**: ID, UserID, XUIEmails (direct/relay), Bypass, TrafficGB, Dates
- **Payment**: ID, UserID, Amount, Provider, ProviderPaymentID, Status, CreatedAt
- **Referral**: ID, ReferrerID, RefereeID, RewardedAt, CreatedAt
- **AuditLog**: ID, UserID, Action, Payload (JSON), CreatedAt

### internal/repository
Interface-based repository pattern. Two implementations:

**intrfaces.go**: Defines interfaces
- `Users.UpsertUser(ctx, user)`, `GetUser(ctx, tgID)`
- `Subscriptions.CreateSubscription(ctx, sub)`, `GetActiveSubscription(ctx, userID)`
- `Payments.CreatePayment(p)`, `UpdatePaymentStatus(ctx, id, status)`, `GetPaymentByProviderID(ctx, id)`
- `Referrals.CreateReferral(ctx, ref)`, `GetReferralCount(ctx, referrerID)`
- `Audit.Log(ctx, userID, action, payload)`

**sqlite/**: SQLite implementations
- **db.go**: DB struct, `New(path)` constructor, auto-migration via `migrate()`
- **user.go, subscriptions.go, payments.go, etc.**: Query methods

### internal/service
Business logic layer:
- **services.go**: Empty service container (TODO: expand)
- **payment.go**: `PaymentService` — wraps YooKassa client and payment/audit repos
  - `InitiatePayment(userID, req)`: Create YooKassa payment, store in DB
  - `Confirm(ctx, ykPaymentID)`: Mark payment succeeded, log audit
  - `GetYkClient()`: Access to raw YooKassa client
- **user.go, subscription.go**: (stubs, not yet implemented)

### internal/telegram
Bot framework and handlers:

**bot.go**: Initialization and startup
- `NewBot()`: Creates bot with long polling
- `Run(bot, paymentService)`: Registers handlers and starts polling

**fsm/fsm.go**: Session state machine
- `Store`: Thread-safe in-memory session map (tgID → Session)
- `Session`: Holds current state + constructor state (GB, devices, months)
- `ConstructorState.CalcPrice()`: Pricing logic
- `GetStore()`: Access global session store

**texts/texts.go & ru.toml**: Localization
- Embeds Russian TOML file with message templates
- `T(path, args)`: Render template, e.g., `T("start.text")` or `T("payment.amount", map[string]any{"rubles": 50})`
- Returns `[missing: key.path]` if key not found (visible in chat, doesn't crash)

**keyboard/**: Message reply buttons and inline keyboards
- `MainMenu()`, `Payment`, etc. — Telebot keyboard objects

**handlers/**: Telegram command/message handlers
- **register.go**: Wires handlers to bot commands
- **start.go**: `/start` command → show MainMenu
- **payment.go**: Payment-related handlers
- Constructor workflow (increments GB/devices/months, shows price, initiates payment)

**callbacks/**: Inline button callbacks (when user taps buttons)
- **register.go**: Wire callback handlers
- **constructor.go**: Handle ➕/➖ buttons for GB, devices, months
- Interacts with `fsm.GetStore()` to update session state

**middleware/state.go**: FSM middleware (runs before handler)
- Loads session from store
- Passes session to handler
- Saves session after handler

### internal/client
External API clients (pluggable, reusable):

**xui/client.go**: 3X-UI panel REST client (context-aware, structured logging)
- `NewClient(host, xuiPath, token)`
- `AddClient(ctx, email, totalGB, expiryTime, limitIP, inboundIDs)`
- `GetClient(ctx, email)`, `UpdateClientByEmail(ctx, email, gb, expiry)`
- `GetClientTraffic(ctx, email)`
- All methods use context for cancellation

**yookassa/client.go**: YooKassa payment API client
- `NewClient(shopID, secretKey, returnURL)`
- `CreatePayment(req)`: POST /v3/payments → Payment
- `FetchPayment(id)`: GET /v3/payments/{id} → Payment
- Handles Basic Auth, Idempotence-Key, JSON marshaling
- Returns `Payment` struct with Metadata preserved

**yookassa/webhook.go**: HTTP handler for YooKassa webhooks
- `Webhook(bot)`: Returns `http.HandlerFunc`
- Parses JSON notification, extracts tg_id from metadata
- Sends Telegram message to user on succeeded payment
- TODO: trigger VPN provisioning

### internal/infra
Infrastructure and utilities:

**logger/logger.go**: zerolog wrapper
- `Setup(format, level)`: Initialize global logger (format: "console" or "json", level: "debug"|"info"|"warn"|"error")
- `L()`: Get current logger instance
- Log with `.With().Int64("user_id", ...).Logger()` for structured context

**db/**: Database utilities (currently empty, DB logic in repository/sqlite/)

## Coding Conventions

### Telegram Architecture (enforce on every change)

**1. Callbacks never send messages — they call handlers.**
Callbacks (`internal/telegram/callbacks/`) are pure orchestrators: read session, call services, update session, then delegate all rendering to a handler. They must not call `c.Send`, `c.EditOrSend`, or `c.Edit` directly.

```go
// ✅ correct
func GbInc(c tele.Context) error {
    sess := fsm.GetStore().Get(c.Sender().ID)
    sess.Constructor.AddPlanGB(config.Cfg.Bot.Constructor.PlanGBStep)
    fsm.GetStore().Save(c.Sender().ID, sess)
    return handlers.Constructor(c)   // handler owns the send
}

// ❌ wrong
func GbInc(c tele.Context) error {
    ...
    return c.EditOrSend("some text", someKeyboard)
}
```

Exception: `c.Respond(&tele.CallbackResponse{...})` (inline popup, not a chat message) is allowed in callbacks, but the text must still come from `texts.T()`.

**2. All user-facing text comes from `ru.toml` via `texts.T()`.**
No string literals in `.go` files that a user will ever read. This includes button labels, error messages, success messages, and popup notifications.

```go
// ✅ correct
return c.Respond(&tele.CallbackResponse{Text: texts.T("check_payment.error_fetch")})

// ❌ wrong
return c.Respond(&tele.CallbackResponse{Text: "Ошибка проверки"})
```

The `internal/telegram/webhook.go` HTTP handler is exempt from rule 1 (it is not a Telegram callback) but must follow rule 2.

**3. After successful VPN provisioning, show a subscriber-specific main menu.**
This screen is not yet designed. When implementing provisioning success, use `handlers.ProvisionSuccess` and leave a `// TODO: show subscriber menu` comment. The menu itself will be added once designed.

### Naming & Style
- Package names: lowercase, no underscores (Go idiom)
- Functions: PascalCase for exported, camelCase for unexported
- Methods: Receiver as `c` for client, `s` for service/store, `d` for database, `r` for repository
- Error handling: explicit `if err != nil` (no panic in handlers)

### Error Messages
- Wrap external errors with context: `fmt.Errorf("action: %w", err)`
- Log entry point (handler) calls with structured fields: `log.Error().Err(err).Str("key", val).Msg("context")`
- Avoid error shadowing; use distinct variable names for wrapped errors

### Context Usage
- All database and API calls accept `context.Context` parameter
- Context cancellation signals graceful shutdown (20s HTTP client timeout)
- Webhook handler does not use context (net/http doesn't provide one until Go 1.21+)

### Logging
- Use zerolog structured logging throughout
- Entry/exit points in service methods:
  ```go
  log := logger.L().With().Int64("user_id", userID).Logger()
  log.Info().Msg("action: starting")
  // ... work ...
  log.Info().Msg("action: ok")
  ```
- Audit important events to database (`audit.Log(ctx, &userID, "action", payload)`)

### Database & Transactions
- Currently no transaction wrapping (single-row inserts ok for MVP)
- Future: wrap multi-step operations (create payment in YK, insert in DB) in transactions to avoid orphans
- Always close DB resources explicitly (defer close)

### Type Safety
- Use custom types for semantic clarity (e.g., `type State string` instead of raw string)
- Tag structs with `json:` for APIs and `repository:` for database column mapping

### Testing
- `internal/client/yookassa/client_test.go` exists (currently minimal)
- Test payment creation, webhook parsing, 3X-UI operations as priorities

## Environment Variables & Configuration

### Startup
1. **config.yaml** in current directory — loaded via `config.Load("./config.yaml")`
2. **bot.db** in current directory — SQLite database, auto-created on first run

### Runtime
- **YOOKASSA_WEBHOOK_URL**: Used in YooKassa client; set to public domain where bot receives webhooks
- **Telegram token**: Hardcoded in config.yaml (keep secure, don't commit real token)
- **YooKassa credentials**: shop_id + secret_key in config.yaml (test credentials in repo; rotate in production)

### Future Improvements
- Move secrets to environment variables or `.env` file (use `godotenv` package)
- Add health check endpoint for webhook delivery
- Implement transaction-based payment confirmation (idempotent webhook handling)
- Provision VPN in webhook callback (currently TODO)
- Referral reward logic
- User traffic analytics and alerts

## Development Commands

### Build
```bash
go build -o vpnbot ./cmd/main.go
```

### Run
```bash
go run ./cmd/main.go
```

### Test
```bash
go test ./... -v
go test -run TestPaymentCreation -v ./internal/client/yookassa
```

### Lint
```bash
go fmt ./...
go vet ./...
```

### Dependencies
```bash
go mod tidy
go mod download
```

## Important Notes

1. **Session State is Ephemeral** — FSM state in-memory only; survives only while bot is running. Long-term user state should be persisted to DB (future work).

2. **Payment Webhook Security** — Currently accepts all notifications. Should validate webhook signature via YooKassa secret key (TODO).

3. **VPN Provisioning Missing** — Webhook receives payment_succeeded but doesn't actually create VPN user or send subscription link to user (hardcoded TODO message).

4. **Inbound ID Configuration** — 3X-UI inbound IDs are hardcoded in callbacks or config (not clear yet). Should be externalized to config.yaml.

5. **Error Recovery** — If YooKassa payment succeeds but DB insert fails, orphaned payment in YK. Add transaction-like pattern to recover (manual review + audit log).

6. **Concurrent Writes** — SQLite WAL mode handles concurrent writers; suitable for bot scale. Upgrade to PostgreSQL if >100 concurrent users.

7. **Referral Logic** — Models and DB schema ready; business logic not wired (future feature).
