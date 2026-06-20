package yookassa

import (
	"encoding/json"
	"testing"
)

// Проверяем, что ответ Webhook (включая metadata, токен карты, last4 и
// confirmation_url) корректно раскладывается в Payment. metadata критично:
// именно из него webhook узнаёт preset/period для активации.
func TestRawPaymentToPayment_WithMetadata(t *testing.T) {
	raw := []byte(`{
		"id": "2d8f...abc",
		"status": "succeeded",
		"paid": true,
		"confirmation": { "confirmation_url": "https://yoomoney.ru/checkout/123" },
		"payment_method": {
			"id": "pm_777",
			"saved": true,
			"card": { "last4": "4242" }
		},
		"metadata": { "kind": "renew", "preset": "pro", "period": "m3" }
	}`)

	var rp rawPayment
	if err := json.Unmarshal(raw, &rp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p := rp.toPayment()

	if p.ID != "2d8f...abc" {
		t.Errorf("ID = %q", p.ID)
	}
	if p.Status != "succeeded" || !p.Paid {
		t.Errorf("status/paid = %q/%v", p.Status, p.Paid)
	}
	if p.ConfirmationURL != "https://yoomoney.ru/checkout/123" {
		t.Errorf("confirmation url = %q", p.ConfirmationURL)
	}
	if p.PaymentMethodID != "pm_777" {
		t.Errorf("payment method id = %q", p.PaymentMethodID)
	}
	if p.Last4 != "4242" {
		t.Errorf("last4 = %q", p.Last4)
	}
	if p.Metadata["preset"] != "pro" || p.Metadata["period"] != "m3" || p.Metadata["kind"] != "renew" {
		t.Errorf("metadata not carried through: %#v", p.Metadata)
	}
}

// Платёж без metadata не должен ломать парсинг.
func TestRawPaymentToPayment_NoMetadata(t *testing.T) {
	var rp rawPayment
	if err := json.Unmarshal([]byte(`{"id":"x","status":"pending","paid":false}`), &rp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p := rp.toPayment()
	if p.ID != "x" || p.Status != "pending" {
		t.Errorf("got %#v", p)
	}
	if len(p.Metadata) != 0 {
		t.Errorf("expected empty metadata, got %#v", p.Metadata)
	}
}
