package handlers

import (
	"context"
	"fmt"

	"vpnbottg/internal/config"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func Help(c tele.Context) error {
	return screen(c, "help", texts.T("help.text", map[string]any{
		"Support": config.Cfg.Bot.Support,
	}), keyboard.HelpKeyboard(), &tele.SendOptions{ParseMode: tele.ModeHTML})
}

func Invite(botUsername string, refSvc *service.ReferralService, userSvc *service.User) tele.HandlerFunc {
	return func(c tele.Context) error {
		ctx := context.Background()
		link := fmt.Sprintf("https://t.me/%s?start=ref_%d", botUsername, c.Sender().ID)
		count := 0
		if refSvc != nil {
			count = refSvc.GetCount(ctx, c.Sender().ID)
		}
		var balance int64
		if userSvc != nil {
			balance, _ = userSvc.GetBalance(ctx, c.Sender().ID)
		}
		rewardPct := config.Cfg.Bot.ReferralRewardPct
		if rewardPct <= 0 {
			rewardPct = 50
		}
		return screen(c, "invite", texts.T("invite.text", map[string]any{
			"Link":      link,
			"Count":     count,
			"RewardPct": rewardPct,
			"Balance":   balance,
			"Support":   config.Cfg.Bot.Support,
		}), keyboard.InviteKeyboard(), &tele.SendOptions{ParseMode: tele.ModeHTML})
	}
}
