package handlers

import (
	"context"

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
	return screen(c, "menu_main", texts.T("menu.guest.text"), keyboard.GuestMenu())
}

func SubscriberMenu(c tele.Context, sub *models.Subscription) error {
	return subscriberMenuScreen(sub).Render(c)
}
