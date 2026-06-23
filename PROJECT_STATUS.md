# VPN Subscription Bot — Статус проекта

**Дата анализа:** 2026-06-22  
**Версия:** 08cef62 (last commit)

---

## 📋 Обзор проекта

**VPN Subscription Bot** — Telegram-бот для автоматизации управления VPN-подписками с интеграцией платежей и панели управления 3X-UI.

### Основные возможности

✅ **Реализовано:**
- Управление пользователями (регистрация, профиль)
- Конструктор подписок (выбор устройств и месяцев; трафик безлимитный)
- Система расчета цены (по устройствам, с учетом скидок за месяцы)
- Интеграция с YooKassa (инициирование платежей, вебхуки)
- Интеграция с 3X-UI для provisioning VPN
- Система подписок (создание, продление; трафик безлимитный на всех inbound'ах)
- Реферальная система (отслеживание рефереров)
- Логирование всех действий в audit_log
- Напоминания об истечении подписки
- Admin-панель для управления (запись о пользователях, статистика)
- Device tracking (отслеживание устройств по HWID)
- Sub-сервер для выдачи конфигов клиентам

🚧 **В разработке / TODO:**
- Trial-подписки (основа есть, нужна логика)
- Расширенная статистика и аналитика
- Оптимизация производительности для масштабирования

---

## 🏗️ Архитектура

### Слои приложения

```
Telegram Bot (telebot.v3 long polling)
    ↓
Handlers + Callbacks (обработчики команд и кнопок)
    ↓
Services (бизнес-логика)
    ↓
Repository (SQLite, интерфейсы)
    ↓
Database (SQLite3 WAL mode)
```

### Основные компоненты

#### 1. **cmd/main.go** — Точка входа
- Инициализация логирования (zerolog)
- Загрузка конфига (YAML)
- Создание подключений к БД, YooKassa, 3X-UI
- Запуск всех сервисов (reminder, webhook poller, bot)
- Graceful shutdown

#### 2. **internal/telegram/bot.go** — Telegram-бот
- Long polling через telebot.v3
- Регистрация handlers, callbacks, commands
- Middleware для FSM-состояния и администратора

#### 3. **internal/service/** — Бизнес-логика
- `payment.go` — Инициирование платежей, подтверждение
- `subscription.go` — Создание, продление подписок (трафик безлимитный)
- `user.go` — Управление пользователями
- `referral.go` — Реферальная система
- `admin.go` — Admin-команды
- `reminder.go` — Напоминания (отдельная горутина)

#### 4. **internal/repository/sqlite/** — Доступ к БД
- `user.go`, `subscriptions.go`, `payments.go`, `devices.go`, `referrals.go`, `stats.go`
- Interface-based pattern для тестируемости
- Использует `sqlx` для удобства

#### 5. **internal/telegram/handlers/** — Обработчики
- `menu.go` — Главное меню
- `payment.go` — Показ статуса платежа
- `constructor.go` — Конструктор подписки
- `subscription.go` — Показ активных подписок
- `admin.go` — Admin-команды
- `addon.go`, `trial.go`, `help.go` — Дополнительные экраны

#### 6. **internal/telegram/callbacks/** — Кнопки (inline)
- `constructor.go` — Кнопки ➕/➖ для устройств и выбор месяцев
- `devices.go` — Управление устройствами
- `trial.go` — Trial-подписка
- `admin.go` — Admin-кнопки
- `addon.go` — Дополнительные услуги

#### 7. **internal/client/**
- `yookassa/client.go` — REST-клиент YooKassa API
- `xui/client.go` — REST-клиент 3X-UI панели
- Асинхронные, с поддержкой контекста

#### 8. **internal/telegram/session/** — Состояние сессии
- In-memory хранилище (для одной сессии)
- Thread-safe (sync.Mutex)
- Хранит: текущее состояние, параметры конструктора, URL платежа

---

## 📊 Схема БД

### Таблицы

#### `users`
```sql
tg_id (INT PRIMARY KEY) — ID пользователя в Telegram
username (TEXT) — Никнейм
first_name (TEXT) — Имя
referred_by (INT FK) — ID реферрера (nullable)
created_at (INT) — Время регистрации (Unix epoch)
```

#### `subscriptions`
```sql
id (INT PK AUTOINCREMENT)
user_id (INT FK) — Владелец подписки
xui_email_direct (TEXT) — Email в 3X-UI (direct inbound)
xui_email_relay (TEXT) — Email в 3X-UI (relay inbound)
xui_sub_id (TEXT) — ID подписки в 3X-UI
bypass (BOOL) — Поддерживает ли bypass-режим
traffic_gb (INT) — Устарело: всегда 0 (трафик безлимитный); колонка оставлена для совместимости
device_limit (INT) — Макс. одновременных устройств
started_at (INT) — Начало подписки
expires_at (INT) — Истечение подписки
reminded_at (INT, nullable) — Когда было последнее напоминание
expired_notified (BOOL) — Уведомлен ли об истечении
created_at (INT)
```

#### `payments`
```sql
id (INT PK AUTOINCREMENT)
user_id (INT FK)
amount (INT) — Сумма в копейках (целое число)
provider (TEXT) — "yookassa" (расширяемо)
provider_payment_id (TEXT) — UUID платежа в YK
status (TEXT) — pending|succeeded|canceled
created_at (INT)
```

#### `referrals`
```sql
id (INT PK AUTOINCREMENT)
referrer_id (INT FK) — Тот, кто пригласил
referee_id (INT FK) — Новый пользователь
rewarded_at (INT, nullable) — Когда выплачена награда
created_at (INT)
```

#### `device_connections`
```sql
id (INT PK AUTOINCREMENT)
sub_id (TEXT) — ID подписки в 3X-UI
device_id (TEXT) — HWID устройства (из x-hwid заголовка)
platform (TEXT) — ОС устройства
user_agent (TEXT) — User-Agent клиента
first_seen (INT) — Первое подключение
last_seen (INT) — Последнее подключение
```

#### `audit_log`
```sql
id (INT PK AUTOINCREMENT)
user_id (INT FK, nullable) — Кто совершил (NULL для системных)
action (TEXT) — payment_succeeded, subscription_created, etc.
payload (TEXT) — JSON с деталями
created_at (INT)
```

#### `schema_migrations`
```sql
version (INT PK)
name (TEXT)
applied_at (INT)
```

---

## 💳 Поток платежа

### 1. Инициирование платежа
1. Пользователь вводит `/start` → видит главное меню
2. Нажимает "🚀 Купить VPN"
3. Переходит в конструктор (выбирает устройства и месяцы; трафик безлимитный)
4. Видит цену: `devices × price_per_device × months × discount[months]`
5. Нажимает "Оплатить"
6. `PaymentService.InitiatePayment()` → создает платеж в YooKassa
7. БД получает запись с `status: pending`
8. Бот отправляет URL на оплату

### 2. Подтверждение (Webhook)
1. Пользователь оплачивает на сайте YooKassa
2. YooKassa отправляет `POST /webhook/yookassa` с `notification`:
```json
{
  "type": "notification",
  "event": "payment.succeeded",
  "object": {
    "id": "yk_payment_id",
    "status": "succeeded",
    "metadata": { "tg_id": "123456", ... }
  }
}
```
3. Webhook-обработчик (`telegram.NewWebhookHandler`) парсит уведомление
4. `PaymentService.Confirm()` → обновляет статус на `succeeded` в БД
5. `SubscriptionService.Create()` → создает VPN-пользователя в 3X-UI
6. Бот отправляет пользователю конфиг или ссылку на скачивание

### Расчет цены
```go
price = devices × config.PricePerDevice × months × monthDiscount[months]
```
Трафик безлимитный, в цене не участвует (см. `session.ConstructorState.CalcPrice`).

**Параметры из config.yaml:**
```yaml
bot:
  constructor:
    price_per_device: 30  # RUB за устройство в месяц
    devices_step: 1       # Шаг изменения числа устройств
    default_devices: 1    # Стартовое число устройств в конструкторе
    month_discounts:
      1: 1.00             # Скидка за 1 месяц = нет
      3: 0.85             # Скидка за 3 месяца = -15%
      6: 0.75             # Скидка за 6 месяцев = -25%
      12: 0.55            # Скидка за 12 месяцев = -45%
```

---

## 🔌 Интеграции

### YooKassa API
- **Создание платежа:** `POST /v3/payments`
- **Получение платежа:** `GET /v3/payments/{id}`
- **Вебхук:** получение `payment.succeeded` уведомлений
- **Auth:** Basic Auth (shopID:secretKey)
- **Идемпотентность:** Idempotence-Key UUID

### 3X-UI API
- **AddClient** — создать нового пользователя VPN
- **GetClient** — получить данные пользователя (трафик, ID)
- **UpdateClientByEmail** — продлить трафик/срок
- **GetClientTraffic** — мониторить использование
- **Emails:** `u{tgID}` (direct), `u{tgID}r` (relay)

---

## 🔄 Недавние изменения (08cef62)

### Упрощение архитектуры (commit 08cef62)
Была упрощена архитектура путем изменений в:

1. **cmd/main.go**
   - Было: инициализация DB, создание PaymentService
   - Теперь: приняты сервисы как параметр в Run()

2. **internal/service/payment.go**
   - Удален метод `GetYkClient()` (не нужен если передавать YK напрямую)
   - Конструктор упрощен

3. **internal/telegram/bot.go**
   - Было: `Run(bot, paymentService)`
   - Теперь: `Run(bot, ykClient)` (проще для callbacks)

4. **internal/telegram/callbacks/constructor.go**
   - Удалены вызовы через `paymentServ.GetYkClient()`
   - Прямое использование `ykClient.FetchPayment()`

### ❓ Вопрос
**Была ли эта упрощение полной?**  
В текущем main.go (исходящем из git) видно, что:
- DB инициализируется
- Создаются ВСЕ сервисы (user, payment, subscription, referral, admin)
- Передаются в `telegram.Run()`

Значит, git показывает более свежее состояние, чем последний коммит.

---

## 🎯 Текущий статус функций

### ✅ Полностью реализовано
- Регистрация пользователей
- Конструктор подписок (выбор параметров)
- Расчет цены с учетом скидок
- Платежи через YooKassa
- Вебхуки YooKassa
- Provisioning в 3X-UI
- Управление подписками (просмотр, продление)
- Напоминания об истечении
- Реферальная система (основа)
- Логирование всех действий
- Device tracking
- Sub-сервер для конфигов

### 🚧 Частично реализовано
- Trial-подписки (модель есть, логика нужна)
- Admin-панель (основные команды есть)
- Статистика (база есть, выводы нужны)

### ❌ Не реализовано
- Расширенная аналитика
- Автоматическое отключение при превышении трафика
- Система поддержки (tickets)
- Мобильное приложение

---

## 🛠️ Технический стек

| Компонент | Технология |
|-----------|-----------|
| Язык | Go 1.24 |
| Бот | telebot.v3 (long polling) |
| БД | SQLite3 (WAL mode) |
| Платежи | YooKassa REST API |
| VPN-панель | 3X-UI v2.6+ REST API |
| Логирование | zerolog (JSON) |
| Конфиг | YAML (gopkg.in/yaml.v3) |
| HTTP-клиент | net/http (async, context-aware) |
| Тестирование | Go standard testing + custom |

---

## 📁 Структура проекта

```
vpnbottg/
├── cmd/
│   └── main.go                          # Точка входа, инициализация
├── internal/
│   ├── client/
│   │   ├── xui/
│   │   │   └── client.go               # 3X-UI API client
│   │   └── yookassa/
│   │       ├── client.go               # YooKassa API client
│   │       └── notification.go         # Вебхук notification models
│   ├── config/
│   │   └── config.go                   # YAML config loading
│   ├── infra/
│   │   ├── db/
│   │   │   ├── 00X_*.up.sql           # Migrations
│   │   │   └── db.go                   # DB utilities
│   │   └── logger/
│   │       └── logger.go               # zerolog wrapper
│   ├── models/
│   │   └── models.go                   # Data structs
│   ├── repository/
│   │   ├── interfaces.go               # Repository interfaces
│   │   └── sqlite/
│   │       ├── db.go                   # DB connection + migration
│   │       ├── user.go                 # User CRUD
│   │       ├── subscriptions.go        # Subscription CRUD
│   │       ├── payments.go             # Payment CRUD
│   │       ├── devices.go              # Device tracking
│   │       ├── referrals.go            # Referral CRUD
│   │       ├── stats.go                # Statistics queries
│   │       └── audit.go                # Audit log
│   ├── service/
│   │   ├── payment.go                  # Payment business logic
│   │   ├── subscription.go             # Subscription business logic
│   │   ├── user.go                     # User management
│   │   ├── referral.go                 # Referral rewards
│   │   ├── admin.go                    # Admin operations
│   │   └── reminder.go                 # Expiry reminders (goroutine)
│   ├── subserver/
│   │   └── ...                         # Sub-конфиг выдачи (happ client)
│   └── telegram/
│       ├── bot.go                      # Bot initialization
│       ├── webhook.go                  # YooKassa webhook handler
│       ├── session/
│       │   └── session.go              # In-memory session storage
│       ├── handlers/
│       │   ├── menu.go                 # Main menu
│       │   ├── payment.go              # Payment screens
│       │   ├── constructor.go          # Subscription builder
│       │   ├── subscription.go         # Active subscriptions
│       │   ├── admin.go                # Admin screens
│       │   ├── trial.go                # Trial subscription
│       │   ├── addon.go                # Add-ons
│       │   ├── help.go                 # Help text
│       │   └── register.go             # Handler registration
│       ├── callbacks/
│       │   ├── constructor.go          # GB/device/month buttons
│       │   ├── devices.go              # Device management
│       │   ├── trial.go                # Trial buttons
│       │   ├── admin.go                # Admin buttons
│       │   ├── addon.go                # Add-on buttons
│       │   └── register.go             # Callback registration
│       ├── commands/
│       │   └── register.go             # /command registration
│       ├── keyboard/
│       │   └── keyboard.go             # Keyboard layouts
│       ├── middleware/
│       │   ├── state.go                # Session FSM middleware
│       │   ├── admin.go                # Admin check
│       │   └── dedup.go                # Deduplication
│       ├── texts/
│       │   ├── texts.go                # Text loading + T() function
│       │   └── ru.toml                 # Russian localization
│       └── commands/
│           └── register.go             # /command registration
├── config.yaml                          # Config file
├── config.example.yaml                  # Example config
├── bot.db                              # SQLite database (auto-created)
├── go.mod, go.sum                      # Dependencies
└── CLAUDE.md                           # Project documentation
```

---

## 🚀 Развертывание

### Требования
- Go 1.24+
- SQLite3
- Интернет для YooKassa и 3X-UI APIs

### Запуск локально
```bash
# Установка зависимостей
go mod download

# Копирование конфига
cp config.example.yaml config.yaml
# Отредактировать config.yaml с реальными токенами

# Запуск
go run ./cmd/main.go
```

### Сборка
```bash
go build -o vpnbot ./cmd/main.go
./vpnbot
```

### Конфигурация (config.yaml)
```yaml
bot:
  token: "YOUR_TELEGRAM_BOT_TOKEN"
  admin_id: 123456789
  constructor:
    price_per_device: 30
    devices_step: 1
    default_devices: 1
    month_discounts:
      1: 1.00
      3: 0.85
      6: 0.75
      12: 0.55

yookassa:
  shop_id: "YOUR_SHOP_ID"
  secret_key: "YOUR_SECRET_KEY"
  webhook_url: "https://yourdomain.com/webhook/yookassa"
  webhook_port: 8080

xui:
  host: "https://panel.example.com:54321"
  path: "xui"
  token: "YOUR_XUI_TOKEN"
  inbounds_direct: [1, 2]      # Direct inbound IDs
  inbounds_relay: [3, 4]       # Relay inbound IDs
  sub_url_template: "https://yourdomain.com/sub/{subId}"

subserver:
  port: 8081
  upstream_template: "https://your-upstream-panel/api/{path}"
  public_base_url: "https://yourdomain.com"

logging:
  format: "json"  # "json" or "console"
  level: "info"   # "debug", "info", "warn", "error"
```

---

## 📈 Масштабирование

### Текущие ограничения
- **SQLite** подходит для <100 активных пользователей одновременно
- **In-memory sessions** теряются при рестарте бота
- **Long polling** — сцентрировано на одном боте (не масштабируется горизонтально)

### Рекомендации для роста
1. **PostgreSQL вместо SQLite** при >1000 пользователей
2. **Redis для сессий** вместо in-memory
3. **Webhook вместо long polling** для масштабируемости
4. **Rate limiting** для API endpoints
5. **Load balancing** для webhook-сервера

---

## 🔐 Безопасность

### ✅ Реализовано
- Basic Auth для 3X-UI
- Bearer Token для YooKassa
- Хеширование паролей (если будут)
- Аудит-логирование всех действий
- Context-based cancellation for cleanup

### 🚧 TODO
- Валидация вебхука YooKassa (signature verification)
- Rate limiting для платежей
- Transaction-based payment confirmation (atomicity)
- Защита от CSRF для webhook

---

## 📝 Примеры использования бота

### Пользовательский сценарий
```
/start
→ Главное меню
  - 💳 Купить подписку
  - 📊 Мои подписки
  - 🔗 Реферальная ссылка
  - ❓ Помощь

🚀 Купить VPN
→ Конструктор (трафик безлимитный)
  Devices: [⬅️ 1 ➡️]
  Months: [⬅️ 1 ➡️]
  Цена: 30 RUB
  
[Оплатить]
→ Отправка URL на оплату YooKassa

Пользователь оплачивает
→ Вебхук подтверждает платеж
→ Создание VPN в 3X-UI
→ Отправка конфига пользователю
```

### Admin-сценарий
```
/admin
→ Admin-панель
  - 👥 Пользователи
  - 💰 Платежи
  - 📊 Статистика

👥 Пользователи
→ Показ списка с фильтрацией
  - По ID
  - По статусу платежа
  - По активности
```

---

## 🐛 Известные проблемы и TODO

1. **Session persistence** — сессии теряются при рестарте
2. **Trial logic incomplete** — модель есть, реализация нужна
3. **Error recovery** — orphaned payments в YK если БД упадет
4. **Webhook validation** — нет проверки подписи от YK
5. **Device blocking** — нет автоматического отключения при лимите трафика

---

## 📊 Статистика кода

| Метрика | Значение |
|---------|----------|
| Строк Go кода | ~5000+ |
| Пакетов | 12+ |
| Обработчиков | 15+ |
| Миграций БД | 10 |
| Моделей | 6 |
| Интеграций | 2 (YooKassa, 3X-UI) |

---

## 🎓 Lessons Learned

1. **Repository pattern** — interface-based access to DB works well for testing
2. **Service layer** — separates business logic from handlers cleanly
3. **Middleware** — FSM and admin checks reduce boilerplate in handlers
4. **Async clients** — context-aware HTTP clients are essential
5. **Audit logging** — critical for debugging payment flows

---

## 🔮 Возможные улучшения (Priority)

### High
- [ ] Автоматическое отключение при лимите трафика
- [ ] Статистика по пользователям и платежам
- [ ] Расширенная help/FAQ

### Medium
- [ ] Trial-подписки (полная реализация)
- [ ] Прямой импорт конфигов из 3X-UI
- [ ] Шаблоны сообщений в конфиге
- [ ] Multi-язычность (пока только RU)

### Low
- [ ] Webhook вместо long polling
- [ ] Кэширование конфигов
- [ ] Миграция на PostgreSQL
- [ ] Mobile app

---

**Проект активно развивается.** Готов к production-использованию с базовым функционалом платежей и управления подписками.
