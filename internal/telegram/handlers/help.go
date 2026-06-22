package handlers

import (
	"fmt"
	"vpnbottg/internal/config"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func Help(c tele.Context) error {
	return c.Send(texts.T("help.text", map[string]any{
		"Support": config.Cfg.Bot.Support,
	}))
}

func Invite(botUsername string) tele.HandlerFunc {
	return func(c tele.Context) error {
		link := fmt.Sprintf("https://t.me/%s?start=ref_%d", botUsername, c.Sender().ID)
		return c.Send(texts.T("invite.text", map[string]any{
			"Link": link,
		}))
	}
}
