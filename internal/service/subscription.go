package service

import (
	"context"
	"errors"
	"fmt"
	"time"
	"vpnbottg/internal/client/xui"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

type Subscription struct {
	subs           repository.Subscriptions
	devices        repository.DeviceConnections
	audit          repository.Audit
	xui            *xui.Client
	inboundsDirect []int
	subURLTemplate string
}

func NewSubscriptionService(
	subs repository.Subscriptions,
	devices repository.DeviceConnections,
	audit repository.Audit,
	xui *xui.Client,
	inboundsDirect []int,
	subURLTemplate string,
) *Subscription {
	return &Subscription{
		subs:           subs,
		devices:        devices,
		audit:          audit,
		xui:            xui,
		inboundsDirect: inboundsDirect,
		subURLTemplate: subURLTemplate,
	}
}

func (s *Subscription) Create(ctx context.Context, userID int64, planDays, deviceLimit int) (*models.Subscription, error) {
	log := logger.L().With().Int64("user_id", userID).Logger()

	now := time.Now()
	expiresAt := now.AddDate(0, 0, planDays)
	email := fmt.Sprintf("u%d", userID)

	// Один клиент на direct inbound'ах, безлимит трафика (totalGB=0).
	// subID не передаём — панель сгенерирует свой.
	if err := s.xui.AddClient(ctx, email, 0, expiresAt, deviceLimit, s.inboundsDirect, ""); err != nil {
		log.Error().Err(err).Msg("createSubscription: xui addClient failed")
		return nil, fmt.Errorf("createSubscription: %w", err)
	}

	// Получаем subId клиента — он станет subId подписки.
	directClient, err := s.xui.GetClient(ctx, email)
	if err != nil {
		log.Error().Err(err).Msg("createSubscription: xui getClient failed")
		return nil, fmt.Errorf("createSubscription get client: %w", err)
	}

	// ВАЖНО: клиент мог остаться от прошлой (истёкшей) подписки — выключенным
	// (reminder делает DisableClient) и со старым сроком: AddClient при
	// "email already in use" его не трогает. Тогда ссылка подписки отдаёт
	// 404/пустой конфиг. Принудительно включаем и выставляем новые срок/лимит.
	if err := s.xui.ResetClient(ctx, email, 0, expiresAt, deviceLimit); err != nil {
		log.Error().Err(err).Msg("createSubscription: xui resetClient failed")
		return nil, fmt.Errorf("createSubscription reset client: %w", err)
	}

	// Чистим учёт устройств прошлой подписки с тем же subId — новая покупка
	// получает свежие слоты, устройства перерегистрируются при первом запросе.
	if directClient.SubID != "" {
		if err := s.devices.DeleteDeviceConnectionsBySubID(ctx, directClient.SubID); err != nil {
			log.Warn().Err(err).Str("xui_sub_id", directClient.SubID).Msg("createSubscription: clear old devices failed")
		}
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
	if err := s.xui.UpdateClientByEmail(ctx, sub.XUIEmailDirect, 0, newExpiry); err != nil {
		log.Error().Err(err).Msg("renew: xui update failed")
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
	cl, err := s.xui.GetClient(ctx, sub.XUIEmailDirect)
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
	if err := s.xui.AddClientCapacity(ctx, sub.XUIEmailDirect, 0, addDevices); err != nil {
		return fmt.Errorf("addDevice: xui: %w", err)
	}
	if err := s.subs.UpdateSubscriptionDevices(ctx, sub.ID, newDevices, sub.TrafficGB); err != nil {
		return fmt.Errorf("addDevice: db: %w", err)
	}
	_ = s.audit.Log(ctx, &userID, "addon_device",
		fmt.Sprintf(`{"sub_id":%d,"add_devices":%d,"new_limit":%d}`, sub.ID, addDevices, newDevices))
	return nil
}

// RemoveDevice уменьшает лимит устройств активной подписки на removeDevices
// (но не ниже 1). Деньги не возвращаются. Если зарегистрированных устройств
// стало больше нового лимита — удаляем самые новые записи из БД, освобождая слоты.
// Возвращает новый лимит устройств.
func (s *Subscription) RemoveDevice(ctx context.Context, userID int64, removeDevices int) (int, error) {
	log := logger.L().With().Int64("user_id", userID).Int("remove", removeDevices).Logger()

	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("removeDevice: %w", err)
	}

	newLimit := sub.DeviceLimit - removeDevices
	if newLimit < 1 {
		newLimit = 1
	}
	actualRemoved := sub.DeviceLimit - newLimit
	if actualRemoved <= 0 {
		return sub.DeviceLimit, nil // уже на минимуме — ничего не делаем
	}

	// Уменьшаем лимит подключений в панели (addGB = 0, отрицательный delta устройств).
	if err := s.xui.AddClientCapacity(ctx, sub.XUIEmailDirect, 0, -actualRemoved); err != nil {
		return 0, fmt.Errorf("removeDevice: xui: %w", err)
	}
	if err := s.subs.UpdateSubscriptionDevices(ctx, sub.ID, newLimit, sub.TrafficGB); err != nil {
		return 0, fmt.Errorf("removeDevice: db: %w", err)
	}

	// Освобождаем слоты: если зарегистрированных устройств больше нового лимита —
	// удаляем самые новые записи из БД (последние подключившиеся).
	if sub.XUISubID == "" {
		if _, err := s.GetConfigURL(ctx, userID); err == nil {
			if reloaded, rErr := s.subs.GetActiveSubscription(ctx, userID); rErr == nil {
				sub = reloaded
			}
		}
	}
	if sub.XUISubID != "" {
		count, err := s.devices.CountDeviceConnections(ctx, sub.XUISubID)
		if err != nil {
			log.Warn().Err(err).Msg("removeDevice: count connections failed")
		} else if excess := count - newLimit; excess > 0 {
			if err := s.devices.DeleteNewestDeviceConnections(ctx, sub.XUISubID, excess); err != nil {
				log.Warn().Err(err).Int("excess", excess).Msg("removeDevice: delete excess failed")
			} else {
				log.Info().Int("excess", excess).Msg("removeDevice: freed device slots")
			}
		}
	}

	_ = s.audit.Log(ctx, &userID, "remove_device",
		fmt.Sprintf(`{"sub_id":%d,"remove_devices":%d,"new_limit":%d}`, sub.ID, actualRemoved, newLimit))
	log.Info().Int64("sub_id", sub.ID).Int("new_limit", newLimit).Msg("removeDevice: ok")
	return newLimit, nil
}

func (s *Subscription) ListDevices(ctx context.Context, userID int64) (*models.Subscription, []*models.DeviceConnection, error) {
	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("listDevices: %w", err)
	}
	if sub.XUISubID == "" {
		if _, err := s.GetConfigURL(ctx, userID); err != nil {
			return nil, nil, fmt.Errorf("listDevices sync sub id: %w", err)
		}
		sub, err = s.subs.GetActiveSubscription(ctx, userID)
		if err != nil {
			return nil, nil, fmt.Errorf("listDevices reload: %w", err)
		}
	}
	devices, err := s.devices.ListDeviceConnections(ctx, sub.XUISubID)
	if err != nil {
		return nil, nil, fmt.Errorf("listDevices: %w", err)
	}
	return sub, devices, nil
}

func (s *Subscription) DeleteDevice(ctx context.Context, userID int64, deviceConnID int64) error {
	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		return fmt.Errorf("deleteDevice: %w", err)
	}
	if sub.XUISubID == "" {
		if _, err := s.GetConfigURL(ctx, userID); err != nil {
			return fmt.Errorf("deleteDevice sync sub id: %w", err)
		}
		sub, err = s.subs.GetActiveSubscription(ctx, userID)
		if err != nil {
			return fmt.Errorf("deleteDevice reload: %w", err)
		}
	}
	if err := s.devices.DeleteDeviceConnectionByID(ctx, sub.XUISubID, deviceConnID); err != nil {
		return fmt.Errorf("deleteDevice: %w", err)
	}
	_ = s.audit.Log(ctx, &userID, "device_deleted", fmt.Sprintf(`{"sub_id":%d,"xui_sub_id":%q,"device_conn_id":%d}`, sub.ID, sub.XUISubID, deviceConnID))
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
	if err := s.xui.UpdateClientByEmail(ctx, sub.XUIEmailDirect, 0, newExpiry); err != nil {
		log.Error().Err(err).Msg("extendExpiry: xui failed")
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
	t, err := s.xui.GetClientTraffic(ctx, sub.XUIEmailDirect)
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

	xuiClient, err := s.xui.GetClient(ctx, sub.XUIEmailDirect)
	if err != nil {
		log.Error().Err(err).Msg("provisionFromPaymentDays: getClient failed")
		return "", fmt.Errorf("provisionFromPaymentDays: getClient failed: %w", err)
	}

	subURL := fmt.Sprintf(s.subURLTemplate, xuiClient.SubID)
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

	planDays := months * 30
	_, err := s.GetActive(ctx, userID)

	if err == nil {
		if err := s.Renew(ctx, userID, planDays); err != nil {
			return "", fmt.Errorf("provisionFromPayment: renew failed: %w", err)
		}
	} else if errors.Is(err, repository.ErrNotFound) {
		_, err := s.Create(ctx, userID, planDays, devices)
		if err != nil {
			return "", fmt.Errorf("provisionFromPayment: create failed: %w", err)
		}
	} else {
		return "", fmt.Errorf("provisionFromPayment: getActive failed: %w", err)
	}

	sub, err := s.GetActive(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("provisionFromPayment: getActive after provision failed: %w", err)
	}

	email := sub.XUIEmailDirect
	xuiClient, err := s.xui.GetClient(ctx, email)
	if err != nil {
		log.Error().Err(err).Msg("provisionFromPayment: getClient failed")
		return "", fmt.Errorf("provisionFromPayment: getClient failed: %w", err)
	}

	subURL := fmt.Sprintf(s.subURLTemplate, xuiClient.SubID)
	log.Info().Str("sub_url", subURL).Msg("provisionFromPayment: ok")
	return subURL, nil
}
