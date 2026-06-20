package main

import (
	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/config"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/telegram"
	"vpnbottg/internal/telegram/texts"
)

func main() {
	texts.Load()
	config.Load("./config.yaml")

	ykClient := yookassa.NewClient(
		config.Cfg.YooKassa.ShopID,
		config.Cfg.YooKassa.SecretKey,
		config.Cfg.YooKassa.WebhookURL,
	)

	bot, err := telegram.NewBot()
	if err != nil {
		logger.L().Err(err)
		return
	}

	telegram.Run(bot, ykClient)
}
