package keyboard

import (
	"fmt"
	"vpnbottg/internal/models"
	"vpnbottg/internal/telegram/session"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

var (
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

	Back       = "back_menu"
	ShowConfig = "show_config"

	// Кнопка-подпись в ряду ➕/➖ (значение по центру). Ничего не делает,
	// но должна быть зарегистрирована, иначе Telegram крутит вечный спиннер.
	NoopDev = "noop_dev"

	// Главное меню (inline-навигация).
	NavBuy      = "nav_buy"
	NavTrial    = "nav_trial"
	NavHelp     = "nav_help"
	NavInvite   = "nav_invite"
	NavConfig   = "nav_config"
	NavDevices  = "nav_devices"
	NavSettings = "nav_settings"

	PromoRedeem = "promo_redeem"

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
	AdminPromo       = "admin_promo"
)

// MenuBar — постоянная нижняя кнопка быстрого возврата в меню. Inline-навигация
// остаётся на самих сообщениях; эта кнопка просто всегда под рукой, чтобы вернуться
// в меню, даже если экран с «◀️ Меню» уехал вверх по чату.
func MenuBar() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{ResizeKeyboard: true}
	m.Reply(m.Row(m.Text(texts.T("buttons.menu"))))
	return m
}

func GuestMenu() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(m.Data(texts.T("start.buttons.buy"), NavBuy)),
		m.Row(m.Data(texts.T("start.buttons.trial"), NavTrial)),
		m.Row(m.Data(texts.T("buttons.promo"), PromoRedeem)),
		m.Row(
			m.Data(texts.T("start.buttons.help"), NavHelp),
			m.Data(texts.T("start.buttons.invite"), NavInvite),
		),
	)
	return m
}

func SubscriberMenu() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(m.Data(texts.T("menu.subscriber.buttons.my_config"), NavConfig)),
		m.Row(
			m.Data(texts.T("menu.subscriber.buttons.devices"), NavDevices),
			m.Data(texts.T("menu.subscriber.buttons.settings"), NavSettings),
		),
		m.Row(m.Data(texts.T("buttons.promo"), PromoRedeem)),
		m.Row(
			m.Data(texts.T("menu.subscriber.buttons.help"), NavHelp),
			m.Data(texts.T("menu.subscriber.buttons.invite"), NavInvite),
		),
	)
	return m
}

func TrialInfo() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(m.Data(texts.T("trial.buttons.confirm"), TrialBuy)),
		m.Row(m.Data(texts.T("buttons.back"), Back)),
	)
	return m
}

// HelpKeyboard и InviteKeyboard — одна кнопка «Назад» для информационных экранов.
func HelpKeyboard() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(m.Row(m.Data(texts.T("buttons.back"), Back)))
	return m
}

func InviteKeyboard() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(m.Row(m.Data(texts.T("buttons.back"), Back)))
	return m
}

// MyConfigKeyboard — кнопка «Назад» на экране с QR-кодом (фото).
func MyConfigKeyboard() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(m.Row(m.Data(texts.T("buttons.back"), Back)))
	return m
}

// PromoPromptKeyboard — экран ввода промокода: одна кнопка «Назад».
func PromoPromptKeyboard() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(m.Row(m.Data(texts.T("buttons.back"), Back)))
	return m
}

func Constructor(s *session.ConstructorState, finalPrice int) *tele.ReplyMarkup {
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

	devLabel := fmt.Sprintf("%d устр.", s.GetDevices())
	buyLabel := texts.T("constructor.buttons.buy", map[string]any{"Price": finalPrice})

	m.Inline(
		m.Row(
			m.Data("➖", DevicesDec),
			m.Data(devLabel, NoopDev),
			m.Data("➕", DevicesInc),
		),
		m.Row(monthBtn(1), monthBtn(3), monthBtn(6), monthBtn(12)),
		m.Row(m.Data(buyLabel, Buy)),
		m.Row(m.Data(texts.T("buttons.back"), Back)),
	)

	return m
}

func SettingsKeyboard(addDev, devPrice int) *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(
			m.Data("➖", AddonDevDec),
			m.Data(fmt.Sprintf("+%d устр.", addDev), NoopDev),
			m.Data("➕", AddonDevInc),
		),
		m.Row(m.Data(fmt.Sprintf("💳 %d ₽ за устройство", devPrice), BuyAddonDev)),
		m.Row(m.Data(texts.T("buttons.back"), Back)),
	)
	return m
}

func DevicesKeyboard(devices []*models.DeviceConnection) *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(devices)+1)
	for i, device := range devices {
		label := fmt.Sprintf("🗑 Удалить #%d", i+1)
		rows = append(rows, m.Row(m.Data(label, DeleteDevice, fmt.Sprintf("%d", device.ID))))
	}
	rows = append(rows, m.Row(m.Data(texts.T("buttons.back"), Back)))
	m.Inline(rows...)
	return m
}

func ProvisionSuccessKeyboard() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(m.Data("📡 Получить конфиг →", ShowConfig)),
	)
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
		m.Row(m.Data(texts.T("admin.buttons.promo"), AdminPromo)),
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
		m.Row(m.Data(texts.T("buttons.back"), Back)),
	)
	return m
}
