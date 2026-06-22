package keyboard

import (
	"fmt"
	"vpnbottg/internal/models"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

var (
	GbDec = "gb_dec"
	GbInc = "gb_inc"

	DevicesDec = "devices_dec"
	DevicesInc = "devices_inc"

	Month1  = "month_1"
	Month3  = "month_3"
	Month6  = "month_6"
	Month12 = "month_12"

	Buy          = "buy"
	CheckPayment = "check_payment"

	AddonDevDec = "addon_dev_dec"
	AddonDevInc = "addon_dev_inc"
	BuyAddonDev = "buy_addon_dev"

	DeleteDevice = "delete_device"

	TrialBuy = "trial_buy"

	AdminStats       = "admin_stats"
	AdminSubs        = "admin_subs"
	AdminPay         = "admin_pay"
	AdminOrphaned    = "admin_orphaned"
	AdminCharts      = "admin_charts"
	AdminReissue     = "admin_reissue"
	AdminFindUser    = "admin_find_user"
	AdminBroadcast   = "admin_broadcast"
	AdminRefund      = "admin_refund"
	AdminDeleteUser  = "admin_delete_user"
	AdminCancelInput = "admin_cancel_input"
	AdminBack        = "admin_back"
)

func GuestMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text(texts.T("start.buttons.trial"))),
		menu.Row(menu.Text(texts.T("start.buttons.buy"))),
		menu.Row(menu.Text(texts.T("start.buttons.help")), menu.Text(texts.T("start.buttons.invite"))),
	)
	return menu
}

func SubscriberMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	menu.Reply(
		menu.Row(menu.Text(texts.T("menu.subscriber.buttons.my_config"))),
		menu.Row(menu.Text(texts.T("menu.subscriber.buttons.devices")), menu.Text(texts.T("menu.subscriber.buttons.add"))),
		menu.Row(menu.Text(texts.T("menu.subscriber.buttons.status"))),
		menu.Row(menu.Text(texts.T("menu.subscriber.buttons.help")), menu.Text(texts.T("menu.subscriber.buttons.invite"))),
	)
	return menu
}

func TrialInfo() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(m.Row(m.Data(texts.T("trial.buttons.confirm"), TrialBuy)))
	return m
}

func Constructor(s *session.ConstructorState) *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}

	monthBtn := func(n int) tele.Btn {
		var label string
		switch n {
		case 12:
			label = "🔥 12 мес"
		default:
			label = fmt.Sprintf("%d мес", n)
		}
		if s.GetMonths() == n {
			label = "✅ " + label
		}
		return m.Data(label, fmt.Sprintf("month_%d", n))
	}

	gbLabel := fmt.Sprintf("%d ГБ", s.GetGB())
	devLabel := fmt.Sprintf("%d устр.", s.GetDevices())
	buyLabel := texts.T("constructor.buttons.buy", map[string]any{"Price": s.CalcPrice()})

	m.Inline(
		m.Row(
			m.Data("➖", GbDec),
			m.Data(gbLabel, "noop_gb"),
			m.Data("➕", GbInc),
		),
		m.Row(
			m.Data("➖", DevicesDec),
			m.Data(devLabel, "noop_dev"),
			m.Data("➕", DevicesInc),
		),
		m.Row(monthBtn(1), monthBtn(3), monthBtn(6), monthBtn(12)),
		m.Row(m.Data(buyLabel, Buy)),
	)

	return m
}

func AddDeviceKeyboard(amount, price int) *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(
			m.Data("➖", AddonDevDec),
			m.Data(fmt.Sprintf("+%d устр.", amount), "noop_dev"),
			m.Data("➕", AddonDevInc),
		),
		m.Row(m.Data(fmt.Sprintf("💳 Оплатить %d ₽", price), BuyAddonDev)),
	)
	return m
}

func DevicesKeyboard(devices []*models.DeviceConnection) *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	if len(devices) == 0 {
		return m
	}

	rows := make([]tele.Row, 0, len(devices))
	for i, device := range devices {
		label := fmt.Sprintf("Удалить #%d", i+1)
		rows = append(rows, m.Row(m.Data(label, DeleteDevice, fmt.Sprintf("%d", device.ID))))
	}
	m.Inline(rows...)
	return m
}

func AdminMenu() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(
			m.Data(texts.T("admin.buttons.stats"), AdminStats),
			m.Data(texts.T("admin.buttons.subs"), AdminSubs),
		),
		m.Row(
			m.Data(texts.T("admin.buttons.payments"), AdminPay),
			m.Data(texts.T("admin.buttons.orphaned"), AdminOrphaned),
		),
		m.Row(
			m.Data(texts.T("admin.buttons.charts"), AdminCharts),
		),
		m.Row(
			m.Data(texts.T("admin.buttons.find_user"), AdminFindUser),
			m.Data(texts.T("admin.buttons.broadcast"), AdminBroadcast),
		),
		m.Row(
			m.Data(texts.T("admin.buttons.refund"), AdminRefund),
			m.Data(texts.T("admin.buttons.delete_user"), AdminDeleteUser),
		),
	)
	return m
}

func AdminBackKeyboard() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(m.Row(m.Data(texts.T("admin.buttons.back"), AdminBack)))
	return m
}

func AdminCancelKeyboard() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(m.Row(m.Data(texts.T("admin.buttons.cancel"), AdminCancelInput)))
	return m
}

func PaymentPending(paymentID string) *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(m.Data(texts.T("check_payment.buttons.verify"), CheckPayment, paymentID)),
	)
	return m
}
