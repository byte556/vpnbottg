package callbacks

import (
	"fmt"
	"vpnbottg/internal/client/yookassa"
	"vpnbottg/internal/config"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/fsm"
	"vpnbottg/internal/telegram/handlers"

	tele "gopkg.in/telebot.v3"
)

func MonthsDec(c tele.Context) error {
	sess := fsm.GetStore().Get(c.Sender().ID)
	sess.Constructor.PrevMonths()
	fsm.GetStore().Save(c.Sender().ID, sess)
	return handlers.Constructor(c)
}

func MonthsInc(c tele.Context) error {
	sess := fsm.GetStore().Get(c.Sender().ID)
	sess.Constructor.NextMonths()
	fsm.GetStore().Save(c.Sender().ID, sess)
	return handlers.Constructor(c)
}
func DevicesDec(c tele.Context) error {
	sess := fsm.GetStore().Get(c.Sender().ID)
	sess.Constructor.AddDevices(-config.Cfg.Bot.Constructor.DevicesStep)
	fsm.GetStore().Save(c.Sender().ID, sess)
	return handlers.Constructor(c)
}

func DevicesInc(c tele.Context) error {
	sess := fsm.GetStore().Get(c.Sender().ID)
	sess.Constructor.AddDevices(config.Cfg.Bot.Constructor.DevicesStep)
	fsm.GetStore().Save(c.Sender().ID, sess)
	return handlers.Constructor(c)
}

func GbDec(c tele.Context) error {
	sess := fsm.GetStore().Get(c.Sender().ID)
	sess.Constructor.AddPlanGB(-config.Cfg.Bot.Constructor.PlanGBStep)
	fsm.GetStore().Save(c.Sender().ID, sess)
	return handlers.Constructor(c)
}

func GbInc(c tele.Context) error {
	sess := fsm.GetStore().Get(c.Sender().ID)
	sess.Constructor.AddPlanGB(config.Cfg.Bot.Constructor.PlanGBStep)
	fsm.GetStore().Save(c.Sender().ID, sess)
	return handlers.Constructor(c)
}

func Buy(paymentServ *service.PaymentService) tele.HandlerFunc {
	return func(c tele.Context) error {
		sess := fsm.GetStore().Get(c.Sender().ID)
		price := sess.Constructor.CalcPrice()

		description := fmt.Sprintf(
			"VPN %d GB / %d устройств / %d мес",
			sess.Constructor.GetPlanGB(),
			sess.Constructor.GetDevices(),
			sess.Constructor.GetMonths(),
		)

		ykPayment, _, err := paymentServ.InitiatePayment(c.Sender().ID, yookassa.CreatePaymentReq{
			AmountRub:   price,
			Description: description,
			SaveMethod:  true,
			Metadata: map[string]string{
				"tg_id":   fmt.Sprintf("%d", c.Sender().ID),
				"plan_gb": fmt.Sprintf("%d", sess.Constructor.GetPlanGB()),
				"devices": fmt.Sprintf("%d", sess.Constructor.GetDevices()),
				"months":  fmt.Sprintf("%d", sess.Constructor.GetMonths()),
			},
		})
		if err != nil {
			return handlers.PaymentError(c)
		}

		sess.PaymentID = ykPayment.ID
		fsm.GetStore().Save(c.Sender().ID, sess)

		return handlers.Payment(c)
	}
}
func CheckPayment(yk *service.PaymentService) tele.HandlerFunc {
	return func(c tele.Context) error {
		paymentID := c.Data()

		payment, err := yk.GetYkClient().FetchPayment(paymentID)
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "Ошибка проверки"})
		}

		if payment.Status != "succeeded" {
			return c.Respond(&tele.CallbackResponse{Text: "❌ Оплата не найдена"})
		}

		return c.Send("✅ Оплата прошла! Сейчас выдам конфиг.")
	}
}
