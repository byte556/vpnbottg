package service

import (
	"context"
	"time"
	"vpnbottg/internal/client/xui"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/repository"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// Notify отправляет пользователю экран напоминания: карточку-обложку card
// (пустая строка — без карточки) с подписью text и inline-клавиатурой markup.
type Notify func(userID int64, card, text string, markup *tele.ReplyMarkup)

type Reminder struct {
	subs   repository.Subscriptions
	xui    *xui.Client
	notify Notify
}

func NewReminderService(
	subs repository.Subscriptions,
	xui *xui.Client,
	notify Notify,
) *Reminder {
	return &Reminder{
		subs:   subs,
		xui:    xui,
		notify: notify,
	}
}

func (r *Reminder) Run(ctx context.Context) {
	log := logger.L().With().Str("service", "reminder").Logger()
	log.Info().Msg("reminder service started")

	r.check(ctx)

	ticker := time.NewTicker(4 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("reminder service stopped")
			return
		case <-ticker.C:
			r.check(ctx)
		}
	}
}

func (r *Reminder) check(ctx context.Context) {
	log := logger.L().With().Str("service", "reminder").Logger()
	now := time.Now()

	// Подписки, истекающие через 1–3 дня, которым ещё не отправляли напоминание
	from := now.Add(24 * time.Hour).Unix()
	to := now.Add(3 * 24 * time.Hour).Unix()
	expiring, err := r.subs.GetExpiringUnreminded(ctx, from, to)
	if err != nil {
		log.Error().Err(err).Msg("check: GetExpiringUnreminded failed")
	}
	for _, sub := range expiring {
		daysLeft := max(1, int(time.Until(time.Unix(sub.ExpiresAt, 0)).Hours()/24))
		r.notify(sub.UserID, "", texts.T("reminder.expiry", map[string]any{"Days": daysLeft}), keyboard.RenewKeyboard())
		if err := r.subs.MarkReminded(ctx, sub.ID); err != nil {
			log.Error().Err(err).Int64("sub_id", sub.ID).Msg("check: MarkReminded failed")
		}
		log.Info().Int64("user_id", sub.UserID).Int("days_left", daysLeft).Msg("reminder sent")
	}

	// Подписки, истёкшие за последние 48 часов — уведомление + деактивация в XUI
	since := now.Add(-48 * time.Hour).Unix()
	expired, err := r.subs.GetRecentlyExpiredUnnotified(ctx, since)
	if err != nil {
		log.Error().Err(err).Msg("check: GetRecentlyExpiredUnnotified failed")
	}
	for _, sub := range expired {
		r.notify(sub.UserID, "expired", texts.T("reminder.expired"), keyboard.RenewKeyboard())

		if err := r.xui.DisableClient(ctx, sub.XUIEmailDirect); err != nil {
			log.Warn().Err(err).Str("email", sub.XUIEmailDirect).Msg("check: disable failed")
		}

		if err := r.subs.MarkExpiredNotified(ctx, sub.ID); err != nil {
			log.Error().Err(err).Int64("sub_id", sub.ID).Msg("check: MarkExpiredNotified failed")
		}
		log.Info().Int64("user_id", sub.UserID).Msg("expiry notification + disable sent")
	}
}
