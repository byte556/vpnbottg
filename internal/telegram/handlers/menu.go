package handlers

import (
	"context"
	"time"
	"vpnbottg/internal/models"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// Menu checks the user's subscription status and routes to GuestMenu or SubscriberMenu.
func Menu(c tele.Context, subSvc *service.Subscription) error {
	sub, err := subSvc.GetActive(context.Background(), c.Sender().ID)
	if err == nil {
		return SubscriberMenu(c, sub)
	}
	return GuestMenu(c)
}

func GuestMenu(c tele.Context) error {
	return editOrFresh(c, texts.T("menu.guest.text"), keyboard.GuestMenu())
}

func SubscriberMenu(c tele.Context, sub *models.Subscription) error {
	daysLeft := max(0, int(time.Until(time.Unix(sub.ExpiresAt, 0)).Hours()/24))
	return editOrFresh(c,
		texts.T("menu.subscriber.text", map[string]any{
			"TrafficGB": sub.TrafficGB,
			"DaysLeft":  daysLeft,
		}),
		keyboard.SubscriberMenu(),
	)
}
