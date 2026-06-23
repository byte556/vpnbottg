package handlers

import (
	"context"
	"fmt"
	"vpnbottg/internal/config"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func Help(c tele.Context) error {
	return c.Send(texts.T("help.text", map[string]any{
		"Support": config.Cfg.Bot.Support,
	}), &tele.SendOptions{ParseMode: tele.ModeHTML})
}

func Invite(botUsername string, refSvc *service.ReferralService) tele.HandlerFunc {
	return func(c tele.Context) error {
		ctx := context.Background()
		link := fmt.Sprintf("https://t.me/%s?start=ref_%d", botUsername, c.Sender().ID)
		count := 0
		if refSvc != nil {
			count = refSvc.GetCount(ctx, c.Sender().ID)
		}
		rewardDays := config.Cfg.Bot.ReferralRewardDays
		if rewardDays <= 0 {
			rewardDays = 7
		}
		return c.Send(texts.T("invite.text", map[string]any{
			"Link":       link,
			"Count":      count,
			"RewardDays": rewardDays,
		}), &tele.SendOptions{ParseMode: tele.ModeHTML})
	}
}
