package telegram

import (
	"time"
	"vpnbottg/internal/config"
	"vpnbottg/internal/service"
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

func Run(bot *tele.Bot, payment *service.PaymentService) {
	callbacks.Register(bot, payment)
	handlers.Register(bot)
	commands.Register(bot)
	bot.Start()
}
