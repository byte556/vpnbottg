# VPN Subscription Bot

Telegram-бот для продажи и управления VPN-подписками. Принимает оплату через
YooKassa, выдаёт доступ через панель [Remnawave](https://docs.rw), ведёт
рефералов, промокоды и баланс — весь цикл от `/start` до автопродления.

**Стек:** Go 1.25 · [telebot.v3](https://gopkg.in/telebot.v3) · SQLite (WAL) ·
YooKassa · Remnawave.

---

## Возможности

- **Конструктор подписки** — выбор числа устройств и срока (мес.), цена
  считается динамически со скидками за длительность.
- **Оплата** — YooKassa (карты, СБП). Вебхук + резервный поллер, так что выдача
  не теряется даже при недоставке уведомления.
- **Выдача доступа** — пользователь в Remnawave (безлимит трафика, лимит
  устройств через нативный HWID), ссылка-подписка + QR-код.
- **Баланс и рефералы** — кешбэк процентом от оплаты реферала зачисляется
  реферреру на баланс; баланс тратится на покупки.
- **Промокоды** — скидка/дни/триал.
- **Напоминания** — уведомление об истечении, автоматическое отключение доступа.
- **Админ-панель** — статистика, поиск и удаление пользователей, рассылка,
  возвраты, управление промокодами.

---

## Архитектура

Слоистая: доступ к БД спрятан за интерфейсами репозитория, бизнес-логика — в
сервисах, Telegram-слой рисует экраны и обрабатывает ввод.

```
Telegram (long polling)  ──▶  handlers / callbacks / commands
                                      │
YooKassa webhook (HTTP)  ──▶  service (бизнес-логика)
                                      │
                              repository (интерфейсы)
                                      │
                              SQLite (WAL, встроенные миграции)
```

### Каталоги

| Путь | Назначение |
|---|---|
| `cmd/main.go` | Точка входа: конфиг, БД, сервисы, запуск бота + вебхук-сервера + фоновых горутин |
| `internal/config` | Загрузка YAML-конфига |
| `internal/models` | Доменные модели (User, Subscription, Payment, …) |
| `internal/repository` | Интерфейсы доступа к данным |
| `internal/repository/sqlite` | Реализация на SQLite |
| `internal/infra/db` | Подключение к SQLite + встроенные (`embed`) миграции |
| `internal/infra/logger` | Структурированный лог (zerolog) |
| `internal/client/yookassa` | REST-клиент YooKassa + разбор вебхука |
| `internal/client/remnawave` | REST-клиент панели: `client.go` (ядро HTTP), `users.go`, `devices.go` |
| `internal/service` | Бизнес-логика: payment, subscription, user, referral, promo, admin, reminder |
| `internal/telegram` | Вебхук-хендлер (`webhook.go`), поллер (`poller.go`), выдача (`provision.go`) |
| `internal/telegram/handlers` | **Единый реестр экранов** (`screens.go`) + интерактивные экраны |
| `internal/telegram/callbacks` | Обработчики inline-кнопок |
| `internal/telegram/commands` | Обработчики команд (`/start`, `/help`, админские) |
| `internal/telegram/keyboard` | Определения клавиатур |
| `internal/telegram/session` | In-memory состояние конструктора (thread-safe) |
| `internal/telegram/assets` | Карточки-обложки экранов |
| `internal/telegram/texts` | Тексты (`ru.toml`) |

### Экраны — один источник

Все экраны с карточкой/клавиатурой определяются один раз в
`handlers/screens.go` (тип `Screen`) и рендерятся двумя способами:

- `Render(c)` — интерактивно, редактированием сообщения (юзер нажал кнопку);
- `Push(bot, userID)` — свежим сообщением для асинхронных событий (вебхук,
  напоминания), где `tele.Context` недоступен.

Вебхук и фоновые горутины **не рисуют экраны сами** — они вызывают `handlers.Push*`.

---

## Поток оплаты

1. Пользователь собирает подписку в конструкторе → бот создаёт платёж в YooKassa
   (метаданные: `tg_id`, `devices`, `months`/`days`, промокод, баланс) и отдаёт
   ссылку на оплату.
2. YooKassa шлёт `payment.succeeded` на `POST /webhook/yookassa`. Хендлер
   проверяет IP-источник и **перепроверяет статус через API** (не доверяет телу).
3. `process` идемпотентно подтверждает платёж, создаёт/реактивирует клиента в
   Remnawave, списывает баланс/промо, начисляет реферальный кешбэк и пушит экран
   успеха.
4. **Резервный поллер** каждые 2 минуты проверяет «зависшие» платежи — выдача не
   теряется, даже если вебхук не дошёл.

---

## Запуск

### Требования
- Go 1.25+
- Панель Remnawave с API-токеном
- Магазин YooKassa (shop_id + secret_key)
- Публичный HTTPS-эндпоинт для вебхука (обычно за reverse proxy)

### Локально
```bash
cp config.example.yaml config.yaml   # заполнить токены
go build -o vpnbot ./cmd/main.go
./vpnbot
```

БД (`bot.db`) создаётся автоматически, миграции применяются на старте (встроены
через `embed`).

### Конфигурация

Все настройки — в `config.yaml` (см. `config.example.yaml`):

- `bot` — токен, admin_id, поддержка, `referral_reward_pct`, параметры
  конструктора (базовая цена, цена за устройство, скидки за срок).
- `yookassa` — shop_id, secret_key, URL и порт вебхука.
- `remnawave` — URL панели, API-токен, `squad_uuids` (internal squad'ы для новых
  юзеров), `sub_url_template` (шаблон ссылки-подписки).

---

## Разработка

```bash
go build ./...
go vet ./...
go test ./...
golangci-lint run          # линтеры
golangci-lint fmt          # форматирование (gofumpt + goimports)
```

Форматирование и линт настроены в `.golangci.yml`.

> Примечание: `golangci-lint run` может выдавать нестабильные ложные
> gofumpt-предупреждения (разные файлы от запуска к запуску). Авторитетная
> проверка форматирования — `golangci-lint fmt --diff`.

---

## Деплой

Автоматический через GitHub Actions (`.github/workflows/deploy.yml`): пуш в
`master` собирает бинарь под Linux, копирует на VPS и перезапускает
systemd-сервис. Секреты прод-окружения — в systemd `EnvironmentFile`, не в репозитории.
