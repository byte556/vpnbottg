package handlers

import (
	"strings"
	"vpnbottg/internal/telegram/fsm"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func Constructor(c tele.Context) error {
	session := fsm.GetStore().Get(c.Sender().ID)

	text := texts.T("constructor.text", map[string]any{
		"PlanGB":  session.Constructor.GetPlanGB(),
		"Devices": session.Constructor.GetDevices(),
		"Months":  session.Constructor.GetMonths(),
		"Price":   session.Constructor.CalcPrice(),
	})
	err := c.EditOrSend(text, keyboard.Constructor(&session.Constructor))
	if err != nil && strings.Contains(err.Error(), "specified new message content and reply") {
		return nil
	}
	return err
}
