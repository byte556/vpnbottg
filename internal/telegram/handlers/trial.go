package handlers

import (
	"context"
	"errors"
	"strconv"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/repository"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// TrialStart — хендлер кнопки «Попробовать 7 дней».
// Если пользователь оплатил, но VPN не создался — ретраим провижнинг без новой оплаты.
// Если подписка есть — сообщаем что триал использован.
func TrialStart(paymentSvc *service.PaymentService, subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		ctx := context.Background()
		log := logger.L().With().Str("func", "TrialStart").Int64("user_id", c.Sender().ID).Logger()

		hasPaid, err := paymentSvc.HasEverPaid(ctx, c.Sender().ID)
		if err != nil {
			log.Error().Err(err).Msg("TrialStart: HasEverPaid failed")
			return editOrFresh(c, texts.T("create_payment.error"))
		}

		if hasPaid {
			_, subErr := subSvc.GetActive(ctx, c.Sender().ID)
			if subErr == nil {
				// Подписка есть — триал уже использован
				return editOrFresh(c, texts.T("trial.used"))
			}
			// Платёж прошёл, но подписки нет — ретраим провижнинг
			log.Info().Msg("TrialStart: orphaned payment — retrying provision")
			return retryOrphanedProvision(c, ctx, paymentSvc, subSvc)
		}

		log.Debug().Msg("TrialStart: showing info page")
		return editOrFresh(c, texts.T("trial.info"), keyboard.TrialInfo())
	}
}

// retryOrphanedProvision повторяет создание VPN по последнему успешному платежу.
func retryOrphanedProvision(c tele.Context, ctx context.Context, paymentSvc *service.PaymentService, subSvc *service.Subscription) error {
	log := logger.L().With().Str("func", "retryOrphanedProvision").Int64("user_id", c.Sender().ID).Logger()

	payment, err := paymentSvc.GetLastSucceededPayment(ctx, c.Sender().ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return editOrFresh(c, texts.T("trial.used"))
		}
		log.Error().Err(err).Msg("retryOrphanedProvision: GetLastSucceededPayment failed")
		return editOrFresh(c, texts.T("error.provision"))
	}

	ykPayment, err := paymentSvc.GetYkClient().FetchPayment(payment.ProviderPaymentID)
	if err != nil {
		log.Error().Err(err).Msg("retryOrphanedProvision: FetchPayment failed")
		return editOrFresh(c, texts.T("error.provision"))
	}

	if err := editOrFresh(c, texts.T("payment.processing")); err != nil {
		return err
	}

	meta := ykPayment.Metadata
	totalGB, _ := strconv.Atoi(meta["total_gb"])
	devices, _ := strconv.Atoi(meta["devices"])
	if totalGB == 0 {
		totalGB = 30
	}
	if devices == 0 {
		devices = 1
	}

	var subURL string
	if daysStr := meta["days"]; daysStr != "" {
		days, _ := strconv.Atoi(daysStr)
		if days <= 0 {
			days = 7
		}
		subURL, err = subSvc.ProvisionFromPaymentDays(ctx, c.Sender().ID, totalGB, devices, days)
	} else {
		months, _ := strconv.Atoi(meta["months"])
		if months <= 0 {
			months = 1
		}
		subURL, err = subSvc.ProvisionFromPayment(ctx, c.Sender().ID, totalGB, devices, months)
	}

	if err != nil {
		log.Error().Err(err).Msg("retryOrphanedProvision: provision failed")
		return editOrFresh(c, texts.T("error.provision"))
	}
	log.Info().Str("sub_url", subURL).Msg("retryOrphanedProvision: ok")
	return ProvisionSuccess(c, subURL, subSvc)
}
