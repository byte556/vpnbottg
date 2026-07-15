package telegram

import (
	"context"
	"strconv"

	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/telegram/handlers"
	"vpnbottg/internal/telegram/texts"
)

// notify шлёт пользователю простое текстовое уведомление (ошибки провижининга).
// Экраны с карточками/клавиатурой отправляются через handlers.Push* — единый
// источник определений экранов.
func (h *WebhookHandler) notify(userID int64, text string) {
	if err := handlers.PushMessage(h.bot, userID, text); err != nil {
		logger.L().Error().Err(err).Int64("user_id", userID).Msg("webhook: notify failed")
	}
}

func (h *WebhookHandler) process(ctx context.Context, ykPayment *yookassa.Payment) {
	log := logger.L().With().Str("func", "process").Str("yk_id", ykPayment.ID).Logger()

	tgIDStr := ykPayment.Metadata["tg_id"]
	if tgIDStr == "" {
		log.Warn().Msg("process: no tg_id in metadata")
		return
	}
	tgID, err := strconv.ParseInt(tgIDStr, 10, 64)
	if err != nil {
		log.Error().Err(err).Str("tg_id_str", tgIDStr).Msg("process: invalid tg_id")
		return
	}
	log = log.With().Int64("tg_id", tgID).Logger()

	changed, err := h.payment.Confirm(ctx, ykPayment.ID)
	if err != nil {
		log.Error().Err(err).Msg("process: Confirm failed")
		h.notify(tgID, texts.T("error.payment_process"))
		return
	}
	if !changed {
		log.Info().Msg("process: already confirmed, skip")
		return
	}

	if err := h.user.EnsureUser(ctx, tgID, "", "", nil); err != nil {
		log.Error().Err(err).Msg("process: EnsureUser failed")
		h.notify(tgID, texts.T("error.user_create"))
		return
	}

	meta := ykPayment.Metadata
	if meta["addon_type"] == "device" {
		amount, _ := strconv.Atoi(meta["amount"])
		log.Info().Int("devices", amount).Msg("process: device addon")
		if err := h.sub.AddDevice(ctx, tgID, amount); err != nil {
			log.Error().Err(err).Msg("process: AddDevice failed")
			h.notify(tgID, texts.T("error.provision"))
			return
		}
		if balStr := meta["balance_used"]; balStr != "" {
			if bal, _ := strconv.ParseInt(balStr, 10, 64); bal > 0 {
				if deducted, err := h.user.DeductBalance(ctx, tgID, bal); err != nil {
					log.Error().Err(err).Int64("balance_used", bal).Msg("process: DeductBalance failed")
				} else {
					log.Info().Int64("deducted", deducted).Msg("process: balance deducted (addon)")
				}
			}
		}
		if err := handlers.PushDeviceAddonSuccess(h.bot, tgID, amount); err != nil {
			log.Error().Err(err).Msg("process: push device addon success failed")
		}
		if err := handlers.PushSubscriberMenu(h.bot, h.sub, tgID); err != nil {
			log.Warn().Err(err).Msg("process: push subscriber menu failed")
		}
		return
	}

	devicesStr := meta["devices"]
	monthsStr := meta["months"]
	daysStr := meta["days"]

	if devicesStr == "" || (monthsStr == "" && daysStr == "") {
		log.Warn().Msg("process: missing subscription metadata")
		h.notify(tgID, texts.T("error.incomplete_metadata"))
		return
	}
	devices, _ := strconv.Atoi(devicesStr)

	var subURL string
	if daysStr != "" {
		days, _ := strconv.Atoi(daysStr)
		log.Info().Int("devices", devices).Int("days", days).Msg("process: provisioning subscription (days)")
		subURL, err = h.sub.ProvisionFromPaymentDays(ctx, tgID, devices, days)
	} else {
		months, _ := strconv.Atoi(monthsStr)
		log.Info().Int("devices", devices).Int("months", months).Msg("process: provisioning subscription")
		subURL, err = h.sub.ProvisionFromPayment(ctx, tgID, devices, months)
	}
	if err != nil {
		log.Error().Err(err).Msg("process: ProvisionFromPayment failed")
		h.notify(tgID, texts.T("error.provision"))
		return
	}
	// Списываем промо-скидку, если она была применена к покупке.
	if h.promo != nil {
		if code := meta["promo_code"]; code != "" {
			if err := h.promo.ConsumeDiscount(ctx, tgID, code); err != nil {
				log.Error().Err(err).Str("code", code).Msg("process: ConsumeDiscount failed")
			}
		}
	}

	if err := handlers.PushProvisionSuccess(h.bot, tgID); err != nil {
		log.Error().Err(err).Msg("process: push provision success failed")
	}
	if err := handlers.PushSubscriberMenu(h.bot, h.sub, tgID); err != nil {
		log.Warn().Err(err).Msg("process: push subscriber menu failed")
	}
	log.Info().Str("sub_url", subURL).Msg("process: ok")

	// Списываем баланс (если покупатель использовал бонусы).
	if balStr := meta["balance_used"]; balStr != "" {
		if bal, _ := strconv.ParseInt(balStr, 10, 64); bal > 0 {
			if deducted, err := h.user.DeductBalance(ctx, tgID, bal); err != nil {
				log.Error().Err(err).Int64("balance_used", bal).Msg("process: DeductBalance failed")
			} else {
				log.Info().Int64("deducted", deducted).Msg("process: balance deducted")
			}
		}
	}

	// Реферальный кешбэк — только от реальной оплаты (не от баланса).
	dbPayment, err := h.payment.GetPaymentByProviderID(ctx, ykPayment.ID)
	if err != nil {
		log.Error().Err(err).Msg("process: GetPaymentByProviderID for referral failed")
	} else {
		handlers.RewardReferrer(ctx, h.bot, h.ref, h.user, tgID, dbPayment.Amount)
	}
}
