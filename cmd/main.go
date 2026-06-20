package main

import (
	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/config"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/repository/sqlite"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram"
	"vpnbottg/internal/telegram/texts"
)

func main() {
	texts.Load()
	config.Load("./config.yaml")

	// создаём подключение к бд
	db, err := sqlite.New("./bot.db")

	ykClient := yookassa.NewClient(
		config.Cfg.YooKassa.ShopID,
		config.Cfg.YooKassa.SecretKey,
		config.Cfg.YooKassa.WebhookURL,
	)
	// создаём сервисы
	paym := service.NewPaymentService(db, db, ykClient)
	bot, err := telegram.NewBot()
	if err != nil {
		logger.L().Err(err)
		return
	}

	telegram.Run(bot, paym)
}
