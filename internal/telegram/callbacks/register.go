package callbacks

import (
	"vpnbottg/internal/repository"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/handlers"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/middleware"

	tele "gopkg.in/telebot.v3"
)

func Register(bot *tele.Bot, payment *service.PaymentService, sub *service.Subscription, user *service.User, ref *service.ReferralService, stats repository.Stats) {
	// Главное меню — inline-навигация. Каждая кнопка делегирует рендеринг хендлеру.
	bot.Handle(&tele.Btn{Unique: keyboard.NavBuy}, handlers.Constructor)
	bot.Handle(&tele.Btn{Unique: keyboard.NavTrial}, handlers.TrialStart(payment, sub))
	bot.Handle(&tele.Btn{Unique: keyboard.NavHelp}, handlers.Help)
	bot.Handle(&tele.Btn{Unique: keyboard.NavInvite}, handlers.Invite(bot.Me.Username, ref))
	bot.Handle(&tele.Btn{Unique: keyboard.NavConfig}, handlers.MyConfig(sub))
	bot.Handle(&tele.Btn{Unique: keyboard.NavDevices}, handlers.Devices(sub))
	bot.Handle(&tele.Btn{Unique: keyboard.NavSettings}, handlers.Settings(sub))

	// Кнопки-подписи (значение по центру ряда ➕/➖) — гасим спиннер, ничего не меняем.
	noop := func(c tele.Context) error { return c.Respond() }
	bot.Handle(&tele.Btn{Unique: keyboard.NoopGB}, noop)
	bot.Handle(&tele.Btn{Unique: keyboard.NoopDev}, noop)

	bot.Handle(&tele.Btn{Unique: keyboard.GbDec}, GbDec)
	bot.Handle(&tele.Btn{Unique: keyboard.GbInc}, GbInc)

	bot.Handle(&tele.Btn{Unique: keyboard.DevicesDec}, DevicesDec)
	bot.Handle(&tele.Btn{Unique: keyboard.DevicesInc}, DevicesInc)

	bot.Handle(&tele.Btn{Unique: keyboard.Month1}, Month(1))
	bot.Handle(&tele.Btn{Unique: keyboard.Month3}, Month(3))
	bot.Handle(&tele.Btn{Unique: keyboard.Month6}, Month(6))
	bot.Handle(&tele.Btn{Unique: keyboard.Month12}, Month(12))

	bot.Handle(&tele.Btn{Unique: keyboard.TrialBuy}, TrialBuy(payment, user))

	bot.Handle(&tele.Btn{Unique: keyboard.Buy}, Buy(payment, user))
	bot.Handle(&tele.Btn{Unique: keyboard.CheckPayment}, CheckPayment(payment, sub, user))

	bot.Handle(&tele.Btn{Unique: keyboard.AddonDevDec}, AddonDevDec(sub))
	bot.Handle(&tele.Btn{Unique: keyboard.AddonDevInc}, AddonDevInc(sub))
	bot.Handle(&tele.Btn{Unique: keyboard.BuyAddonDev}, BuyAddonDevice(payment, user))

	bot.Handle(&tele.Btn{Unique: keyboard.AddonGBDec}, AddonGBDec(sub))
	bot.Handle(&tele.Btn{Unique: keyboard.AddonGBInc}, AddonGBInc(sub))
	bot.Handle(&tele.Btn{Unique: keyboard.BuyAddonGB}, BuyAddonGB(payment, user))

	bot.Handle(&tele.Btn{Unique: keyboard.DeleteDevice}, DeleteDevice(sub))
	bot.Handle(&tele.Btn{Unique: keyboard.Back}, BackToMenu(sub))
	bot.Handle(&tele.Btn{Unique: keyboard.ShowConfig}, ShowConfigCallback(sub))

	adminOnly := middleware.AdminOnly
	bot.Handle(&tele.Btn{Unique: keyboard.AdminStats}, adminOnly(AdminStatsCallback(stats)))
	bot.Handle(&tele.Btn{Unique: keyboard.AdminSubs}, adminOnly(AdminSubsCallback(stats)))
	bot.Handle(&tele.Btn{Unique: keyboard.AdminPay}, adminOnly(AdminPaymentsCallback(stats)))
	bot.Handle(&tele.Btn{Unique: keyboard.AdminOrphaned}, adminOnly(AdminOrphanedCallback(stats)))
	bot.Handle(&tele.Btn{Unique: keyboard.AdminCharts}, adminOnly(AdminChartsCallback(stats)))
	bot.Handle(&tele.Btn{Unique: keyboard.AdminFindUser}, adminOnly(AdminFindUserCallback()))
	bot.Handle(&tele.Btn{Unique: keyboard.AdminBroadcast}, adminOnly(AdminBroadcastCallback()))
	bot.Handle(&tele.Btn{Unique: keyboard.AdminRefund}, adminOnly(AdminRefundCallback()))
	bot.Handle(&tele.Btn{Unique: keyboard.AdminDeleteUser}, adminOnly(AdminDeleteUserCallback()))
	bot.Handle(&tele.Btn{Unique: keyboard.AdminCancelInput}, adminOnly(AdminCancelInputCallback))
	bot.Handle(&tele.Btn{Unique: keyboard.AdminReissue}, adminOnly(AdminReissueCallback(payment, sub, user)))
	bot.Handle(&tele.Btn{Unique: keyboard.AdminBack}, adminOnly(AdminBackCallback))
}
