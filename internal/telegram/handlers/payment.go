package handlers

import (
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// PaymentPending renders the payment confirmation screen using the URL stored
// in the user's session after a successful InitiatePayment call.
func PaymentPending(c tele.Context) error {
	sess := session.GetStore().Get(c.Sender().ID)
	return c.EditOrSend(
		texts.T("payment.pending", map[string]any{"URL": sess.PaymentURL}),
		keyboard.PaymentPending(sess.PaymentID),
	)
}

func PaymentError(c tele.Context) error {
	return c.Send(texts.T("create_payment.error"))
}
