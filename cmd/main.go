package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"vpnbottg/internal/client/xui"
	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/config"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/repository/sqlite"
	"vpnbottg/internal/service"
	"vpnbottg/internal/subserver"
	"vpnbottg/internal/telegram"
	"vpnbottg/internal/telegram/assets"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func main() {
	if err := texts.Load(); err != nil {
		panic("texts.Load: " + err.Error())
	}
	if err := config.Load("./config.yaml"); err != nil {
		panic("config.Load: " + err.Error())
	}

	log := logger.L()
	log.Info().Int64("admin_id", config.Cfg.Bot.AdminID).Msg("startup: config loaded")

	db, err := sqlite.New("./bot.db")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open database")
	}
	defer db.Close()

	ykClient := yookassa.NewClient(
		config.Cfg.YooKassa.ShopID,
		config.Cfg.YooKassa.SecretKey,
		config.Cfg.YooKassa.WebhookURL,
	)

	xuiClient := xui.NewClient(
		config.Cfg.XUI.Host,
		config.Cfg.XUI.Path,
		config.Cfg.XUI.Token,
	)

	userSvc := service.NewUserService(db, db)
	paym := service.NewPaymentService(db, db, ykClient)
	subSvc := service.NewSubscriptionService(
		db, db, db, xuiClient,
		config.Cfg.XUI.InboundsDirect,
		config.Cfg.XUI.SubURLTemplate,
	)
	refSvc := service.NewReferralService(db, subSvc, db)
	adminSvc := service.NewAdminService(db, db, db, db, xuiClient, ykClient, db)
	promoSvc := service.NewPromoService(db, subSvc, db)

	bot, err := telegram.NewBot()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create bot")
	}

	// Корневой контекст — отменяется при получении сигнала завершения.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reminderSvc := service.NewReminderService(
		db, xuiClient,
		func(userID int64, card, text string, markup *tele.ReplyMarkup) {
			to := &tele.User{ID: userID}
			// SendOptions первым — иначе telebot затирает ReplyMarkup (см. handlers.sendOptsFirst).
			opts := &tele.SendOptions{ParseMode: tele.ModeHTML}
			if photo := assets.Photo(card, text); photo != nil {
				bot.Send(to, photo, opts, markup)
				return
			}
			bot.Send(to, text, opts, markup)
		},
	)
	go reminderSvc.Run(ctx)

	webhookHandler := telegram.NewWebhookHandler(bot, paym, subSvc, userSvc, refSvc, promoSvc)
	go webhookHandler.StartPoller(ctx)

	port := config.Cfg.YooKassa.WebhookPort
	if port == 0 {
		port = 8080
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/yookassa", webhookHandler.Handle)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("webhook server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("webhook server error")
		}
	}()

	// Sub-сервер выдачи конфигов happ-клиенту — отдельный порт, опционален.
	var subSrv *http.Server
	if config.Cfg.SubServer.Port != 0 && config.Cfg.SubServer.UpstreamTemplate != "" {
		ss := subserver.New(
			config.Cfg.SubServer.UpstreamTemplate,
			config.Cfg.SubServer.PublicBaseURL,
			db,
		)
		subSrv = &http.Server{
			Addr:    fmt.Sprintf(":%d", config.Cfg.SubServer.Port),
			Handler: ss.Handler(),
		}
		go func() {
			log.Info().Str("addr", subSrv.Addr).Msg("sub server listening")
			if err := subSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error().Err(err).Msg("sub server error")
			}
		}()
	}

	// Бот в горутине — останавливается через bot.Stop().
	go telegram.Run(bot, paym, subSvc, userSvc, refSvc, db, adminSvc, promoSvc)

	// Ждём SIGTERM или SIGINT.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	sig := <-quit
	log.Info().Str("signal", sig.String()).Msg("shutdown: signal received")

	cancel() // останавливаем reminder + поллер

	bot.Stop() // останавливаем long polling

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("shutdown: http server drain failed")
	}
	if subSrv != nil {
		if err := subSrv.Shutdown(shutCtx); err != nil {
			log.Error().Err(err).Msg("shutdown: sub server drain failed")
		}
	}

	log.Info().Msg("shutdown: ok")
	os.Exit(0)
}
