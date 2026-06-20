package handlers

import (
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func Register(bot *tele.Bot) {
	bot.Handle(texts.T("start.buttons.buy"), Constructor)

}
