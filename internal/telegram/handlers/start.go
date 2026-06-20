package handlers

import (
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func Start(c tele.Context) error {
	return c.EditOrSend(texts.T("start.text"), keyboard.MainMenu())
}
