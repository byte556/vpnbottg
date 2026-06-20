package commands

import (
	"vpnbottg/internal/telegram/handlers"

	tele "gopkg.in/telebot.v3"
)

func Register(bot *tele.Bot) {
	bot.Handle("/start", handlers.Start)
}
