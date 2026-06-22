package callbacks

import (
	"context"
	"strconv"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/handlers"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

func DeleteDevice(subSvc *service.Subscription) tele.HandlerFunc {
	return func(c tele.Context) error {
		deviceConnID, err := strconv.ParseInt(c.Data(), 10, 64)
		if err != nil || deviceConnID <= 0 {
			return c.Respond(&tele.CallbackResponse{Text: texts.T("error.provision")})
		}
		if err := subSvc.DeleteDevice(context.Background(), c.Sender().ID, deviceConnID); err != nil {
			logger.L().Error().Err(err).Int64("user_id", c.Sender().ID).Msg("deleteDevice: failed")
			return c.Respond(&tele.CallbackResponse{Text: texts.T("error.provision")})
		}
		_ = c.Respond(&tele.CallbackResponse{Text: texts.T("subscription.device_deleted")})
		return handlers.SendDevices(c, subSvc)
	}
}
