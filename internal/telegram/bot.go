package telegram

import (
	"time"
	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/config"
	"vpnbottg/internal/telegram/callbacks"
	"vpnbottg/internal/telegram/commands"
	"vpnbottg/internal/telegram/handlers"

	tele "gopkg.in/telebot.v3"
)

func NewBot() (*tele.Bot, error) {
	return tele.NewBot(tele.Settings{
		Token:  config.Cfg.Bot.Token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
}

func Run(bot *tele.Bot, yk *yookassa.Client) {
	callbacks.Register(bot, yk)
	handlers.Register(bot)
	commands.Register(bot)
	bot.Start()
}
