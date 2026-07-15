package callbacks

import (
	"context"
	"strconv"

	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/repository"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/handlers"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func AdminStatsCallback(stats repository.Stats) tele.HandlerFunc {
	return handlers.AdminStats(stats)
}

func AdminSubsCallback(stats repository.Stats) tele.HandlerFunc {
	return handlers.AdminSubs(stats)
}

func AdminPaymentsCallback(stats repository.Stats) tele.HandlerFunc {
	return handlers.AdminPayments(stats)
}

func AdminOrphanedCallback(stats repository.Stats) tele.HandlerFunc {
	return handlers.AdminOrphaned(stats)
}

func AdminChartsCallback(stats repository.Stats) tele.HandlerFunc {
	return handlers.AdminCharts(stats)
}

// adminPrompt устанавливает AdminAction в сессии и делегирует рендер хендлеру.
func adminPrompt(action, promptKey string) tele.HandlerFunc {
	return func(c tele.Context) error {
		sess := session.GetStore().Get(c.Sender().ID)
		sess.AdminAction = action
		session.GetStore().Save(c.Sender().ID, sess)
		return handlers.AdminPromptInput(promptKey)(c)
	}
}

func AdminFindUserCallback() tele.HandlerFunc {
	return adminPrompt("find_user", "admin.prompt.find_user")
}

func AdminBroadcastCallback() tele.HandlerFunc {
	return adminPrompt("broadcast", "admin.prompt.broadcast")
}

func AdminRefundCallback() tele.HandlerFunc {
	return adminPrompt("refund", "admin.prompt.refund")
}

func AdminDeleteUserCallback() tele.HandlerFunc {
	return adminPrompt("delete_user", "admin.prompt.delete_user")
}

func AdminCancelInputCallback(c tele.Context) error {
	sess := session.GetStore().Get(c.Sender().ID)
	sess.AdminAction = ""
	session.GetStore().Save(c.Sender().ID, sess)
	if err := c.Respond(&tele.CallbackResponse{Text: texts.T("admin.prompt.cancelled")}); err != nil {
		_ = err
	}
	return handlers.AdminMenu(c)
}

func AdminReissueCallback(paym *service.PaymentService, sub *service.Subscription, user *service.User) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "AdminReissue").Str("yk_id", c.Data()).Logger()

		ctx := context.Background()
		ykPayment, err := paym.GetYkClient().FetchPayment(c.Data())
		if err != nil {
			log.Error().Err(err).Msg("AdminReissue: FetchPayment failed")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("admin.reissue_err")})
		}
		if ykPayment.Status != "succeeded" {
			log.Warn().Str("status", ykPayment.Status).Msg("AdminReissue: payment not succeeded")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("admin.refund.not_succeeded")})
		}

		meta := ykPayment.Metadata
		tgIDStr := meta["tg_id"]
		tgID, err := strconv.ParseInt(tgIDStr, 10, 64)
		if err != nil {
			log.Error().Err(err).Str("tg_id_str", tgIDStr).Msg("AdminReissue: bad tg_id")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("admin.reissue_err")})
		}

		if err := user.EnsureUser(ctx, tgID, "", "", nil); err != nil {
			log.Error().Err(err).Msg("AdminReissue: EnsureUser failed")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("admin.reissue_err")})
		}

		var provErr error
		if meta["addon_type"] == "device" {
			amount, _ := strconv.Atoi(meta["amount"])
			provErr = sub.AddDevice(ctx, tgID, amount)
		} else {
			devices, _ := strconv.Atoi(meta["devices"])
			months, _ := strconv.Atoi(meta["months"])
			if devices == 0 {
				devices = 1
			}
			if months == 0 {
				months = 1
			}
			_, provErr = sub.ProvisionFromPayment(ctx, tgID, devices, months)
		}
		if provErr != nil {
			log.Error().Err(provErr).Int64("target_user", tgID).Msg("AdminReissue: provision failed")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("admin.reissue_err")})
		}

		log.Info().Int64("target_user", tgID).Msg("AdminReissue: ok")
		name := tgIDStr
		if u, err := user.GetUser(ctx, tgID); err == nil {
			if u.Username != "" {
				name = "@" + u.Username
			} else {
				name = u.FirstName
			}
		}
		return c.Respond(&tele.CallbackResponse{
			Text: texts.T("admin.reissue_ok", map[string]any{"Name": name}),
		})
	}
}

func AdminBackCallback(c tele.Context) error {
	return handlers.AdminMenu(c)
}
