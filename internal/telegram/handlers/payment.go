package handlers

import (
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func Payment(c tele.Context) error {
	return c.EditOrSend(texts.T("create_payment.error"), keyboard.Payment)
}

func PaymentError(c tele.Context) error {
	return c.Send(texts.T("create_payment.error"))
}
