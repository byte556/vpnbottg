package commands

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/handlers"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/middleware"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func Register(bot *tele.Bot, subSvc *service.Subscription, refSvc *service.ReferralService, payment *service.PaymentService, user *service.User, adminSvc *service.AdminService, promoSvc *service.PromoService) {
	menuHandler := func(c tele.Context) error {
		return handlers.Menu(c, subSvc)
	}
	inviteHandler := handlers.Invite(bot.Me.Username, refSvc, user)

	bot.Handle("/start", func(c tele.Context) error {
		ctx := context.Background()
		s := c.Sender()

		if err := user.EnsureUser(ctx, s.ID, s.Username, s.FirstName, nil); err != nil {
			logger.L().Error().Err(err).Int64("tg_id", s.ID).Msg("start: EnsureUser failed")
		}

		if refSvc != nil {
			if payload := c.Message().Payload; strings.HasPrefix(payload, "ref_") {
				if refID, err := strconv.ParseInt(strings.TrimPrefix(payload, "ref_"), 10, 64); err == nil {
					if err := refSvc.Record(ctx, s.ID, refID); err != nil {
						logger.L().Warn().Err(err).Msg("start: record referral failed")
					}
				}
			}
		}
		// Ставим постоянную нижнюю кнопку «☰ Меню» (держится для всех следующих
		// сообщений). Заодно вытесняет старую полную reply-клаву у прежних юзеров.
		if err := c.Send(texts.T("start.text"), keyboard.MenuBar()); err != nil {
			return err
		}
		return menuHandler(c)
	})

	// Кнопка «☰ Меню» (постоянная нижняя панель) — быстрый возврат в меню.
	bot.Handle(texts.T("buttons.menu"), menuHandler)

	// /promo <код> — быстрая активация промокода (альтернатива кнопке «🎟 Промокод»).
	bot.Handle("/promo", func(c tele.Context) error {
		code := strings.TrimSpace(c.Message().Payload)
		if code == "" {
			sess := session.GetStore().Get(c.Sender().ID)
			sess.AwaitPromo = true
			session.GetStore().Save(c.Sender().ID, sess)
			return handlers.PromoPrompt(c)
		}
		return handleRedeemPromo(c, context.Background(), promoSvc, subSvc, code)
	})

	// Меню теперь inline (см. callbacks.Register / keyboard.GuestMenu+SubscriberMenu).
	// Эти text-обработчики оставлены как fallback для пользователей, у которых ещё
	// висит старая reply-клавиатура от прошлой версии — нажатие маршрутизируется
	// в тот же хендлер, дублирования с inline-кнопками нет (разные namespace).
	bot.Handle(texts.T("start.buttons.buy"), handlers.Constructor)
	bot.Handle(texts.T("start.buttons.trial"), handlers.TrialStart(payment, subSvc))
	bot.Handle(texts.T("start.buttons.help"), handlers.Help)
	bot.Handle(texts.T("start.buttons.invite"), inviteHandler)

	bot.Handle(texts.T("menu.subscriber.buttons.my_config"), handlers.MyConfig(subSvc))
	bot.Handle(texts.T("menu.subscriber.buttons.devices"), handlers.Devices(subSvc))
	bot.Handle(texts.T("menu.subscriber.buttons.settings"), handlers.Settings(subSvc))
	bot.Handle(texts.T("menu.subscriber.buttons.help"), handlers.Help)
	bot.Handle(texts.T("menu.subscriber.buttons.invite"), inviteHandler)

	// Admin slash commands (только /admin остаётся как точка входа)
	bot.Handle("/admin", middleware.AdminOnly(func(c tele.Context) error {
		// Clear any pending AdminAction so /admin always resets the admin FSM.
		sess := session.GetStore().Get(c.Sender().ID)
		sess.AdminAction = ""
		session.GetStore().Save(c.Sender().ID, sess)
		return handlers.AdminMenu(c)
	}))

	// Перехват текстового ввода администратора после нажатия action-кнопки.
	// Специфичные текстовые обработчики (меню-кнопки) зарегистрированы выше
	// и имеют приоритет над tele.OnText.
	bot.Handle(tele.OnText, func(c tele.Context) error {
		sess := session.GetStore().Get(c.Sender().ID)

		// Ввод промокода пользователем (после кнопки «🎟 Промокод» или /promo).
		// Проверяется до админ-ветки: промокоды доступны всем.
		if sess.AwaitPromo {
			sess.AwaitPromo = false
			session.GetStore().Save(c.Sender().ID, sess)
			return handleRedeemPromo(c, context.Background(), promoSvc, subSvc, strings.TrimSpace(c.Text()))
		}

		if !middleware.IsAdmin(c) {
			return nil
		}
		if sess.AdminAction == "" {
			return nil
		}

		action := sess.AdminAction
		sess.AdminAction = ""
		session.GetStore().Save(c.Sender().ID, sess)

		input := strings.TrimSpace(c.Text())
		ctx := context.Background()

		switch action {
		case "find_user":
			return handleFindUser(c, ctx, adminSvc, subSvc, input)
		case "broadcast":
			return handleBroadcast(c, ctx, bot, adminSvc, input)
		case "refund":
			return handleRefund(c, ctx, adminSvc, input)
		case "delete_user":
			return handleDeleteUser(c, ctx, adminSvc, input)
		case "promo":
			return handleCreatePromo(c, ctx, promoSvc, input)
		}
		return nil
	})
}

func handleFindUser(c tele.Context, ctx context.Context, adminSvc *service.AdminService, subSvc *service.Subscription, input string) error {
	u, err := adminSvc.SearchUser(ctx, input)
	if err != nil {
		return c.Send(texts.T("admin.search.not_found"))
	}

	subInfo := texts.T("admin.search.no_sub")
	if sub, err := subSvc.GetActive(ctx, u.TgID); err == nil {
		daysLeft := max(0, int(time.Until(time.Unix(sub.ExpiresAt, 0)).Hours()/24))
		subInfo = texts.T("admin.search.sub_active", map[string]any{
			"DaysLeft":    daysLeft,
			"DeviceLimit": sub.DeviceLimit,
		})
	}

	username := "—"
	if u.Username != "" {
		username = "@" + u.Username
	}
	return c.Send(texts.T("admin.search.result", map[string]any{
		"TgID":      u.TgID,
		"Username":  username,
		"FirstName": u.FirstName,
		"CreatedAt": time.Unix(u.CreatedAt, 0).Format("02.01.2006"),
		"SubInfo":   subInfo,
	}))
}

func handleBroadcast(c tele.Context, ctx context.Context, bot *tele.Bot, adminSvc *service.AdminService, text string) error {
	if err := c.Send(texts.T("admin.broadcast.started")); err != nil {
		return err
	}
	adminID := c.Sender().ID
	go func() {
		sent, errs := adminSvc.BroadcastAll(ctx, text, func(id int64, msg string) error {
			_, err := bot.Send(&tele.User{ID: id}, msg)
			return err
		})
		bot.Send(&tele.User{ID: adminID}, texts.T("admin.broadcast.done", map[string]any{
			"Sent":   sent,
			"Errors": errs,
		}))
	}()
	return nil
}

func handleRefund(c tele.Context, ctx context.Context, adminSvc *service.AdminService, ykID string) error {
	amount, err := adminSvc.RefundPayment(ctx, ykID)
	if err != nil {
		log := logger.L().With().Str("yk_id", ykID).Logger()
		if errors.Is(err, repository.ErrNotFound) {
			return c.Send(texts.T("admin.refund.not_found"))
		}
		if errors.Is(err, service.ErrPaymentNotSucceeded) {
			return c.Send(texts.T("admin.refund.not_succeeded"))
		}
		log.Error().Err(err).Msg("refund failed")
		return c.Send(texts.T("admin.refund.err"))
	}
	return c.Send(texts.T("admin.refund.ok", map[string]any{"Amount": amount}))
}

// handleRedeemPromo активирует промокод от имени пользователя и делегирует рендер хендлеру.
func handleRedeemPromo(c tele.Context, ctx context.Context, promoSvc *service.PromoService, subSvc *service.Subscription, code string) error {
	if code == "" {
		return c.Send(texts.T("promo.empty"))
	}
	res, err := promoSvc.Redeem(ctx, c.Sender().ID, code)
	if err != nil {
		var key string
		switch {
		case errors.Is(err, service.ErrPromoNotFound):
			key = "promo.err_not_found"
		case errors.Is(err, service.ErrPromoAlreadyUsed):
			key = "promo.err_already_used"
		case errors.Is(err, service.ErrPromoLimitReached):
			key = "promo.err_limit"
		case errors.Is(err, service.ErrPromoInactive):
			key = "promo.err_inactive"
		case errors.Is(err, service.ErrPromoExpired):
			key = "promo.err_expired"
		default:
			logger.L().Error().Err(err).Str("code", code).Int64("user_id", c.Sender().ID).Msg("redeem promo failed")
			key = "promo.err_generic"
		}
		return c.Send(texts.T(key))
	}

	if res.Type == service.RedeemTypeDiscount {
		sess := session.GetStore().Get(c.Sender().ID)
		sess.PromoCode = res.Code
		sess.PromoDiscountPct = res.DiscountPct
		session.GetStore().Save(c.Sender().ID, sess)
		return handlers.PromoDiscountApplied(c, res)
	}
	return handlers.PromoDaysSuccess(c, res, subSvc)
}

// handleCreatePromo парсит админский ввод и создаёт/деактивирует промокод.
//
//	del КОД                         — деактивировать
//	КОД days <дней> <устр> <лимит>  — код на бесплатные дни
//	КОД discount <%> <лимит>        — код на скидку
func handleCreatePromo(c tele.Context, ctx context.Context, promoSvc *service.PromoService, input string) error {
	html := func(key string, args ...map[string]any) error {
		return c.Send(texts.T(key, args...), &tele.SendOptions{ParseMode: tele.ModeHTML})
	}

	fields := strings.Fields(input)
	if len(fields) < 2 {
		return html("admin.promo.usage")
	}

	// del КОД — деактивация.
	if strings.EqualFold(fields[0], "del") {
		if err := promoSvc.Deactivate(ctx, fields[1]); err != nil {
			if errors.Is(err, service.ErrPromoNotFound) {
				return html("admin.promo.not_found")
			}
			logger.L().Error().Err(err).Msg("deactivate promo failed")
			return html("admin.promo.err")
		}
		return html("admin.promo.deleted", map[string]any{"Code": strings.ToUpper(fields[1])})
	}

	code := strings.ToUpper(fields[0])
	kind := strings.ToLower(fields[1])
	p := &models.PromoCode{Code: code, RewardType: kind, Active: true}

	switch kind {
	case models.PromoRewardDays:
		if len(fields) != 5 {
			return html("admin.promo.usage")
		}
		days, e1 := strconv.Atoi(fields[2])
		dev, e2 := strconv.Atoi(fields[3])
		lim, e3 := strconv.Atoi(fields[4])
		if e1 != nil || e2 != nil || e3 != nil {
			return html("admin.promo.usage")
		}
		p.Days, p.Devices, p.MaxUses = days, dev, lim
	case models.PromoRewardDiscount:
		if len(fields) != 4 {
			return html("admin.promo.usage")
		}
		pct, e1 := strconv.Atoi(fields[2])
		lim, e2 := strconv.Atoi(fields[3])
		if e1 != nil || e2 != nil {
			return html("admin.promo.usage")
		}
		p.DiscountPct, p.MaxUses = pct, lim
	default:
		return html("admin.promo.usage")
	}

	if err := promoSvc.Create(ctx, p); err != nil {
		if errors.Is(err, service.ErrPromoExists) {
			return html("admin.promo.exists")
		}
		if errors.Is(err, service.ErrPromoInvalid) {
			return html("admin.promo.usage")
		}
		logger.L().Error().Err(err).Msg("create promo failed")
		return html("admin.promo.err")
	}
	return html("admin.promo.created", map[string]any{"Code": code})
}

func handleDeleteUser(c tele.Context, ctx context.Context, adminSvc *service.AdminService, input string) error {
	tgID, err := strconv.ParseInt(input, 10, 64)
	if err != nil || tgID == 0 {
		return c.Send(texts.T("admin.deluser_usage"))
	}
	if err := adminSvc.DeleteUser(ctx, tgID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return c.Send(texts.T("admin.delete_not_found"))
		}
		logger.L().Error().Err(err).Int64("target", tgID).Msg("deleteUser failed")
		return c.Send(texts.T("admin.delete_err"))
	}
	return c.Send(texts.T("admin.delete_ok", map[string]any{"ID": tgID}))
}
