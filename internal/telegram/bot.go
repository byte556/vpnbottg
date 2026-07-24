package telegram

import (
	"time"

	"vpnbottg/internal/config"
	"vpnbottg/internal/repository"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/callbacks"
	"vpnbottg/internal/telegram/commands"
	"vpnbottg/internal/telegram/middleware"

	tele "gopkg.in/telebot.v3"
)

func NewBot() (*tele.Bot, error) {
	return tele.NewBot(tele.Settings{
		Token:  config.Cfg.Bot.Token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
}

func Run(bot *tele.Bot, payment *service.PaymentService, sub *service.Subscription, user *service.User, ref *service.ReferralService, stats repository.Stats, adminSvc *service.AdminService, promo *service.PromoService, nav repository.Nav) {
	bot.Use(middleware.Dedup)
	bot.Use(middleware.TrackNav(nav))
	callbacks.Register(bot, payment, sub, user, ref, stats, promo)
	commands.Register(bot, sub, ref, payment, user, adminSvc, promo)
	bot.Start()
}
