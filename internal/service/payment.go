// internal/service/payment.go
package service

import (
	"context"
	"fmt"
	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

type PaymentService struct {
	ykClient *yookassa.Client
	payments repository.Payments
	audit    repository.Audit
}

func NewPaymentService(payments repository.Payments, audit repository.Audit, ykClient *yookassa.Client) *PaymentService {
	return &PaymentService{payments: payments, audit: audit, ykClient: ykClient}
}

func (s *PaymentService) GetYkClient() *yookassa.Client {
	return s.ykClient
}

func (s *PaymentService) HasEverPaid(ctx context.Context, userID int64) (bool, error) {
	return s.payments.HasSucceededPayment(ctx, userID)
}

func (s *PaymentService) GetLastSucceededPayment(ctx context.Context, userID int64) (*models.Payment, error) {
	return s.payments.GetLastSucceededPayment(ctx, userID)
}
func (s *PaymentService) InitiatePayment(
	userID int64,
	req yookassa.CreatePaymentReq,
) (*yookassa.Payment, int64, error) {
	log := logger.L().With().
		Int64("user_id", userID).
		Int64("amount", int64(req.AmountRub)).
		Str("description", req.Description).
		Logger()

	log.Info().Msg("initiatePayment: creating in yookassa")

	ykPayment, err := s.ykClient.CreatePayment(req)
	if err != nil {
		log.Error().Err(err).Msg("initiatePayment: yookassa failed")
		return nil, 0, fmt.Errorf("yookassa: %w", err)
	}

	log.Info().
		Str("yk_payment_id", ykPayment.ID).
		Str("yk_status", ykPayment.Status).
		Msg("initiatePayment: yookassa ok, writing to db")

	p := &models.Payment{
		UserID:            userID,
		Amount:            int64(req.AmountRub),
		Provider:          "yookassa",
		ProviderPaymentID: ykPayment.ID,
		Status:            "pending",
	}
	dbID, err := s.payments.CreatePayment(p)
	if err != nil {
		log.Error().Err(err).
			Str("yk_payment_id", ykPayment.ID).
			Msg("initiatePayment: db failed after yookassa create — manual recovery needed")
		return nil, 0, fmt.Errorf("db after yk create (yk_id=%s): %w", ykPayment.ID, err)
	}

	log.Info().
		Int64("payment_id", dbID).
		Str("yk_payment_id", ykPayment.ID).
		Msg("initiatePayment: ok")

	return ykPayment, dbID, nil
}

// GetPendingPayments возвращает pending платежи за последние 2 часа.
func (s *PaymentService) GetPendingPayments(ctx context.Context) ([]*models.Payment, error) {
	return s.payments.GetPendingPayments(ctx)
}

// Cancel помечает платёж как canceled (идемпотентно).
func (s *PaymentService) Cancel(ctx context.Context, providerPaymentID string) error {
	_, err := s.payments.UpdatePaymentStatus(ctx, providerPaymentID, "canceled")
	return err
}

// Confirm обновляет статус платежа на succeeded.
// Возвращает (true, nil) если платеж был обновлён с pending→succeeded.
// Возвращает (false, nil) если платеж уже succeeded или статус не pending (идемпотентно).
// Возвращает (false, error) при ошибке БД.
func (s *PaymentService) Confirm(ctx context.Context, providerPaymentID string) (bool, error) {
	log := logger.L().With().Str("provider_payment_id", providerPaymentID).Logger()

	changed, err := s.payments.UpdatePaymentStatus(ctx, providerPaymentID, "succeeded")
	if err != nil {
		log.Error().Err(err).Msg("confirmPayment: failed")
		return false, fmt.Errorf("confirmPayment: %w", err)
	}

	if !changed {
		log.Info().Msg("confirmPayment: already confirmed")
		return false, nil
	}

	p, err := s.payments.GetPaymentByProviderID(ctx, providerPaymentID)
	if err != nil {
		return false, fmt.Errorf("confirmPayment: %w", err)
	}

	_ = s.audit.Log(ctx, &p.UserID, "payment_succeeded", fmt.Sprintf(`{"payment_id":%d,"amount":%d}`, p.ID, p.Amount))

	log.Info().Msg("confirmPayment: ok")
	return true, nil
}
