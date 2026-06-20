package keyboard

import (
	"vpnbottg/internal/telegram/fsm"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

var (
	GbDec = "gb_dec"
	GbInc = "gb_inc"

	DevicesDec = "devices_dec"
	DevicesInc = "devices_inc"

	WhiteListToggle = "wl_toggle"

	MonthsDec    = "months_dec"
	MonthsInc    = "months_inc"
	Buy          = "buy"
	CheckPayment = "check_payment"
)

func MainMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{
		ResizeKeyboard: true,
	}

	btnBuy := menu.Text(texts.T("start.buttons.buy"))
	btnHelp := menu.Text(texts.T("start.buttons.help"))
	btnInvite := menu.Text(texts.T("start.buttons.invite"))

	menu.Reply(
		menu.Row(btnBuy),
		menu.Row(btnHelp, btnInvite),
	)

	return menu
}

func Constructor(s *fsm.ConstructorState) *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}

	m.Inline(
		m.Row(
			m.Data(texts.T("constructor.buttons.gb_dec"), GbDec),
			m.Data(texts.T("constructor.buttons.gb_inc"), GbInc),
		),
		m.Row(
			m.Data(texts.T("constructor.buttons.dev_dec"), DevicesDec),
			m.Data(texts.T("constructor.buttons.dev_inc"), DevicesInc),
		),
		m.Row(
			m.Data(texts.T("constructor.buttons.months_dec"), MonthsDec),
			m.Data(texts.T("constructor.buttons.months_inc"), MonthsInc),
		),
		m.Row(
			m.Data(texts.T("constructor.buttons.buy"), Buy),
		),
	)

	return m
}
func Payment() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}

	m.Inline(
		m.Row(
			m.Data(texts.T("constructor.buttons.gb_dec"), GbDec),
			m.Data(texts.T("constructor.buttons.gb_inc"), GbInc),
		),
		m.Row(
			m.Data(texts.T("constructor.buttons.dev_dec"), DevicesDec),
			m.Data(texts.T("constructor.buttons.dev_inc"), DevicesInc),
		),
		m.Row(
			m.Data(texts.T("constructor.buttons.months_dec"), MonthsDec),
			m.Data(texts.T("constructor.buttons.months_inc"), MonthsInc),
		),
		m.Row(
			m.Data(texts.T("constructor.buttons.buy"), Buy),
		),
	)

	return m
}
