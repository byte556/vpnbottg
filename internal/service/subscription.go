package service

import (
	"context"
	"errors"
	"fmt"
	"time"
	"vpnbottg/internal/client/remnawave"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

type Subscription struct {
	subs           repository.Subscriptions
	audit          repository.Audit
	panel          *remnawave.Client
	subURLTemplate string
}

func NewSubscriptionService(
	subs repository.Subscriptions,
	audit repository.Audit,
	panel *remnawave.Client,
	subURLTemplate string,
) *Subscription {
	return &Subscription{
		subs:           subs,
		audit:          audit,
		panel:          panel,
		subURLTemplate: subURLTemplate,
	}
}

func (s *Subscription) Create(ctx context.Context, userID int64, planDays, deviceLimit int) (*models.Subscription, error) {
	log := logger.L().With().Int64("user_id", userID).Logger()

	now := time.Now()
	expiresAt := now.AddDate(0, 0, planDays)
	email := fmt.Sprintf("u%d", userID)

	// Один пользователь в Remnawave, безлимит трафика (totalGB=0),
	// привязка к squad'ам из конфига. shortUuid панель генерирует сама.
	if err := s.panel.AddClient(ctx, email, 0, expiresAt, deviceLimit); err != nil {
		log.Error().Err(err).Msg("createSubscription: panel addClient failed")
		return nil, fmt.Errorf("createSubscription: %w", err)
	}

	// Получаем subId (shortUuid) клиента — он станет subId подписки.
	directClient, err := s.panel.GetClient(ctx, email)
	if err != nil {
		log.Error().Err(err).Msg("createSubscription: panel getClient failed")
		return nil, fmt.Errorf("createSubscription get client: %w", err)
	}

	// ВАЖНО: клиент мог остаться от прошлой (истёкшей) подписки — DISABLED
	// (reminder делает DisableClient) и со старым сроком: AddClient при уже занятом
	// username его не трогает. Тогда ссылка подписки отдаёт пустой конфиг.
	// Принудительно включаем и выставляем новые срок/лимит.
	if err := s.panel.ResetClient(ctx, email, 0, expiresAt, deviceLimit); err != nil {
		log.Error().Err(err).Msg("createSubscription: panel resetClient failed")
		return nil, fmt.Errorf("createSubscription reset client: %w", err)
	}

	sub := &models.Subscription{
		UserID:         userID,
		XUIEmailDirect: email, // безлимит
		XUIEmailRelay:  "",    // relay не используется — один клиент
		XUISubID:       directClient.SubID,
		Bypass:         false,
		TrafficGB:      0, // безлимит
		DeviceLimit:    deviceLimit,
		StartedAt:      now.Unix(),
		ExpiresAt:      expiresAt.Unix(),
	}
	id, err := s.subs.CreateSubscription(ctx, sub)
	if err != nil {
		log.Error().Err(err).Msg("createSubscription: db failed")
		return nil, fmt.Errorf("createSubscription: %w", err)
	}
	sub.ID = id

	_ = s.audit.Log(ctx, &userID, "subscription_created", fmt.Sprintf(`{"sub_id":%d,"xui_sub_id":%q,"days":%d}`, id, directClient.SubID, planDays))

	log.Info().Int64("sub_id", id).Int("days", planDays).Msg("createSubscription: ok")
	return sub, nil
}

// Renew продлевает подписку — обновляет xui и expires_at в db.
func (s *Subscription) Renew(ctx context.Context, userID int64, addDays int) error {
	log := logger.L().With().Int64("user_id", userID).Logger()

	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.ErrNotFound
		}
		return fmt.Errorf("renew: %w", err)
	}

	newExpiry := time.Unix(sub.ExpiresAt, 0).AddDate(0, 0, addDays)

	// Безлимит трафика
	if err := s.panel.UpdateClientByEmail(ctx, sub.XUIEmailDirect, 0, newExpiry); err != nil {
		log.Error().Err(err).Msg("renew: panel update failed")
		return fmt.Errorf("renew: %w", err)
	}

	if err := s.subs.UpdateSubscriptionExpiry(ctx, sub.ID, newExpiry.Unix()); err != nil {
		log.Error().Err(err).Msg("renew: db update failed")
		return fmt.Errorf("renew: %w", err)
	}

	_ = s.audit.Log(ctx, &userID, "subscription_renewed", fmt.Sprintf(`{"sub_id":%d,"add_days":%d}`, sub.ID, addDays))

	log.Info().Int64("sub_id", sub.ID).Int("add_days", addDays).Msg("renew: ok")
	return nil
}

// GetActive возвращает активную подписку или ErrNotFound.
func (s *Subscription) GetActive(ctx context.Context, userID int64) (*models.Subscription, error) {
	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("getActive: %w", err)
	}
	return sub, nil
}

// GetConfigURL возвращает URL конфига для активной подписки пользователя.
func (s *Subscription) GetConfigURL(ctx context.Context, userID int64) (string, error) {
	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("getConfigURL: %w", err)
	}
	cl, err := s.panel.GetClient(ctx, sub.XUIEmailDirect)
	if err != nil {
		return "", fmt.Errorf("getConfigURL: %w", err)
	}
	if sub.XUISubID == "" && cl.SubID != "" {
		if err := s.subs.UpdateSubscriptionXUISubID(ctx, sub.ID, cl.SubID); err != nil {
			return "", fmt.Errorf("getConfigURL sync sub id: %w", err)
		}
		sub.XUISubID = cl.SubID
	}
	return fmt.Sprintf(s.subURLTemplate, cl.SubID), nil
}

// AddDevice добавляет устройства к текущей активной подписке.
// Трафик (GB) при этом не меняется — устройства и трафик независимы.
func (s *Subscription) AddDevice(ctx context.Context, userID int64, addDevices int) error {
	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		return fmt.Errorf("addDevice: %w", err)
	}
	newDevices := sub.DeviceLimit + addDevices

	// Меняем только лимит подключений (addGB = 0), трафик остаётся прежним.
	if err := s.panel.AddClientCapacity(ctx, sub.XUIEmailDirect, 0, addDevices); err != nil {
		return fmt.Errorf("addDevice: panel: %w", err)
	}
	if err := s.subs.UpdateSubscriptionDevices(ctx, sub.ID, newDevices, sub.TrafficGB); err != nil {
		return fmt.Errorf("addDevice: db: %w", err)
	}
	_ = s.audit.Log(ctx, &userID, "addon_device",
		fmt.Sprintf(`{"sub_id":%d,"add_devices":%d,"new_limit":%d}`, sub.ID, addDevices, newDevices))
	return nil
}

// setDeviceLimit приводит лимит устройств активной подписки к newLimit (не ниже 1).
// Обновляет панель XUI и БД. При уменьшении освобождает лишние слоты — удаляет
// самые новые зарегистрированные устройства из БД. Деньги не возвращаются.
func (s *Subscription) setDeviceLimit(ctx context.Context, userID int64, sub *models.Subscription, newLimit int) error {
	if newLimit < 1 {
		newLimit = 1
	}
	delta := newLimit - sub.DeviceLimit
	if delta == 0 {
		return nil
	}

	// Меняем только лимит подключений (addGB = 0); delta может быть отрицательным.
	if err := s.panel.AddClientCapacity(ctx, sub.XUIEmailDirect, 0, delta); err != nil {
		return fmt.Errorf("setDeviceLimit: panel: %w", err)
	}
	if err := s.subs.UpdateSubscriptionDevices(ctx, sub.ID, newLimit, sub.TrafficGB); err != nil {
		return fmt.Errorf("setDeviceLimit: db: %w", err)
	}
	// Освобождение лишних слотов при уменьшении лимита теперь не нужно: панель
	// Remnawave сама применяет новый hwidDeviceLimit и режет лишние устройства.
	return nil
}

// ListDevices возвращает активную подписку и её HWID-устройства из панели Remnawave.
func (s *Subscription) ListDevices(ctx context.Context, userID int64) (*models.Subscription, []*remnawave.Device, error) {
	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("listDevices: %w", err)
	}
	devices, err := s.panel.ListDevices(ctx, sub.XUIEmailDirect)
	if err != nil {
		return nil, nil, fmt.Errorf("listDevices: %w", err)
	}
	return sub, devices, nil
}

// DeleteDevice отвязывает устройство активной подписки по его индексу в списке
// (список берётся из панели; индекс приходит из callback'а кнопки).
func (s *Subscription) DeleteDevice(ctx context.Context, userID int64, index int) error {
	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		return fmt.Errorf("deleteDevice: %w", err)
	}
	devices, err := s.panel.ListDevices(ctx, sub.XUIEmailDirect)
	if err != nil {
		return fmt.Errorf("deleteDevice list: %w", err)
	}
	if index < 0 || index >= len(devices) {
		return fmt.Errorf("deleteDevice: index %d out of range (%d devices)", index, len(devices))
	}
	hwid := devices[index].HWID
	if err := s.panel.DeleteDevice(ctx, sub.XUIEmailDirect, hwid); err != nil {
		return fmt.Errorf("deleteDevice: %w", err)
	}
	_ = s.audit.Log(ctx, &userID, "device_deleted", fmt.Sprintf(`{"sub_id":%d,"hwid":%q}`, sub.ID, hwid))
	return nil
}

// ExtendExpiry добавляет дни к активной подписке без изменения трафика.
// Используется для реферального вознаграждения.
func (s *Subscription) ExtendExpiry(ctx context.Context, userID int64, addDays int) error {
	log := logger.L().With().Int64("user_id", userID).Int("add_days", addDays).Logger()

	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil // нет активной подписки — пропускаем, не ошибка
		}
		return fmt.Errorf("extendExpiry: %w", err)
	}

	newExpiry := time.Unix(sub.ExpiresAt, 0).AddDate(0, 0, addDays)

	// Безлимит трафика
	if err := s.panel.UpdateClientByEmail(ctx, sub.XUIEmailDirect, 0, newExpiry); err != nil {
		log.Error().Err(err).Msg("extendExpiry: panel failed")
		return fmt.Errorf("extendExpiry: %w", err)
	}
	if err := s.subs.UpdateSubscriptionExpiry(ctx, sub.ID, newExpiry.Unix()); err != nil {
		return fmt.Errorf("extendExpiry: db: %w", err)
	}

	_ = s.audit.Log(ctx, &userID, "subscription_extended", fmt.Sprintf(`{"sub_id":%d,"add_days":%d}`, sub.ID, addDays))
	log.Info().Int64("sub_id", sub.ID).Msg("extendExpiry: ok")
	return nil
}

// GetTrafficUsedGB возвращает использованный трафик в ГБ.
// Возвращает 0 без ошибки если подписки нет или XUI недоступен.
func (s *Subscription) GetTrafficUsedGB(ctx context.Context, userID int64) float64 {
	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		return 0
	}
	t, err := s.panel.GetClientTraffic(ctx, sub.XUIEmailDirect)
	if err != nil {
		return 0
	}
	const giB = 1024 * 1024 * 1024
	return float64(t.Down+t.Up) / giB
}

// ProvisionFromPaymentDays — то же что ProvisionFromPayment, но принимает явное количество дней
// (используется для пробного периода и других случаев когда срок не кратен месяцам).
func (s *Subscription) ProvisionFromPaymentDays(
	ctx context.Context,
	userID int64,
	devices, days int,
) (string, error) {
	log := logger.L().With().Int64("user_id", userID).Logger()

	_, err := s.GetActive(ctx, userID)
	if err == nil {
		if err := s.Renew(ctx, userID, days); err != nil {
			return "", fmt.Errorf("provisionFromPaymentDays: renew failed: %w", err)
		}
	} else if errors.Is(err, repository.ErrNotFound) {
		if _, err := s.Create(ctx, userID, days, devices); err != nil {
			return "", fmt.Errorf("provisionFromPaymentDays: create failed: %w", err)
		}
	} else {
		return "", fmt.Errorf("provisionFromPaymentDays: getActive failed: %w", err)
	}

	sub, err := s.GetActive(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("provisionFromPaymentDays: getActive after provision failed: %w", err)
	}

	panelClient, err := s.panel.GetClient(ctx, sub.XUIEmailDirect)
	if err != nil {
		log.Error().Err(err).Msg("provisionFromPaymentDays: getClient failed")
		return "", fmt.Errorf("provisionFromPaymentDays: getClient failed: %w", err)
	}

	subURL := fmt.Sprintf(s.subURLTemplate, panelClient.SubID)
	log.Info().Str("sub_url", subURL).Msg("provisionFromPaymentDays: ok")
	return subURL, nil
}

// ProvisionFromPayment создаёт или продлевает подписку в зависимости от текущего состояния,
// затем возвращает URL для подачи конфига пользователю.
func (s *Subscription) ProvisionFromPayment(
	ctx context.Context,
	userID int64,
	devices, months int,
) (string, error) {
	log := logger.L().With().Int64("user_id", userID).Logger()

	_, err := s.GetActive(ctx, userID)

	if err == nil {
		// Активная подписка: months>0 — продлеваем срок; months<=0 — только
		// меняем устройства (управление тарифом без продления), срок не трогаем.
		if months > 0 {
			if err := s.Renew(ctx, userID, months*30); err != nil {
				return "", fmt.Errorf("provisionFromPayment: renew failed: %w", err)
			}
		}
	} else if errors.Is(err, repository.ErrNotFound) {
		planDays := months * 30
		if planDays <= 0 {
			planDays = 30 // нет активной подписки — months=0 не имеет смысла, берём месяц
		}
		if _, err := s.Create(ctx, userID, planDays, devices); err != nil {
			return "", fmt.Errorf("provisionFromPayment: create failed: %w", err)
		}
	} else {
		return "", fmt.Errorf("provisionFromPayment: getActive failed: %w", err)
	}

	sub, err := s.GetActive(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("provisionFromPayment: getActive after provision failed: %w", err)
	}

	// Приводим лимит устройств к выбранному в конструкторе значению. При создании
	// подписки delta = 0 (Create уже выставил devices) — это no-op. При продлении
	// активной подписки Renew срок продлил, но устройства не трогал — доводим здесь
	// (увеличение добавляет слоты, уменьшение освобождает лишние, без возврата денег).
	if devices > 0 && devices != sub.DeviceLimit {
		if err := s.setDeviceLimit(ctx, userID, sub, devices); err != nil {
			log.Error().Err(err).Int("target_devices", devices).Msg("provisionFromPayment: setDeviceLimit failed")
		} else {
			sub.DeviceLimit = devices
		}
	}

	email := sub.XUIEmailDirect
	panelClient, err := s.panel.GetClient(ctx, email)
	if err != nil {
		log.Error().Err(err).Msg("provisionFromPayment: getClient failed")
		return "", fmt.Errorf("provisionFromPayment: getClient failed: %w", err)
	}

	subURL := fmt.Sprintf(s.subURLTemplate, panelClient.SubID)
	log.Info().Str("sub_url", subURL).Msg("provisionFromPayment: ok")
	return subURL, nil
}
