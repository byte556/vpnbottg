package callbacks

import (
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/keyboard"

	tele "gopkg.in/telebot.v3"
)

func Register(bot *tele.Bot, payment *service.PaymentService) {
	bot.Handle(&tele.Btn{Unique: keyboard.DevicesDec}, DevicesDec)
	bot.Handle(&tele.Btn{Unique: keyboard.DevicesInc}, DevicesInc)

	bot.Handle(&tele.Btn{Unique: keyboard.GbDec}, GbDec)
	bot.Handle(&tele.Btn{Unique: keyboard.GbInc}, GbInc)

	bot.Handle(&tele.Btn{Unique: keyboard.MonthsDec}, MonthsDec)
	bot.Handle(&tele.Btn{Unique: keyboard.MonthsInc}, MonthsInc)

	bot.Handle(&tele.Btn{Unique: keyboard.Buy}, Buy(payment))
	bot.Handle(&tele.Btn{Unique: keyboard.CheckPayment}, CheckPayment(payment))
}
