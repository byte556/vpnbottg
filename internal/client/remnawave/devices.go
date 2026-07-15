package remnawave

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"vpnbottg/internal/infra/logger"
)

// Device — HWID-устройство пользователя из панели Remnawave.
type Device struct {
	HWID        string
	Platform    string
	OSVersion   string
	DeviceModel string
	UserAgent   string
	RequestIP   string
	CreatedAt   int64 // unix
	UpdatedAt   int64 // unix (последняя активность)
}

type hwidDevice struct {
	HWID        string `json:"hwid"`
	Platform    string `json:"platform"`
	OSVersion   string `json:"osVersion"`
	DeviceModel string `json:"deviceModel"`
	UserAgent   string `json:"userAgent"`
	RequestIP   string `json:"requestIp"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func parseTS(s string) int64 {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix()
	}
	return 0
}

// ListDevices возвращает HWID-устройства пользователя (по email/username).
// Устройства регистрирует сама панель при запросе подписки с заголовком x-hwid.
func (c *Client) ListDevices(ctx context.Context, email string) ([]*Device, error) {
	cl, err := c.GetClient(ctx, email)
	if err != nil {
		return nil, err
	}
	raw, notFound, err := c.do(ctx, http.MethodGet, "api/hwid/devices/"+cl.UUID, nil)
	if err != nil {
		return nil, fmt.Errorf("listDevices %s: %w", email, err)
	}
	if notFound || len(raw) == 0 {
		return nil, nil
	}
	var wrap struct {
		Total   int          `json:"total"`
		Devices []hwidDevice `json:"devices"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("listDevices %s: unmarshal: %w", email, err)
	}
	out := make([]*Device, 0, len(wrap.Devices))
	for _, d := range wrap.Devices {
		out = append(out, &Device{
			HWID:        d.HWID,
			Platform:    d.Platform,
			OSVersion:   d.OSVersion,
			DeviceModel: d.DeviceModel,
			UserAgent:   d.UserAgent,
			RequestIP:   d.RequestIP,
			CreatedAt:   parseTS(d.CreatedAt),
			UpdatedAt:   parseTS(d.UpdatedAt),
		})
	}
	return out, nil
}

// DeleteDevice отвязывает HWID-устройство пользователя в панели.
func (c *Client) DeleteDevice(ctx context.Context, email, hwid string) error {
	log := logger.L().With().Str("email", email).Str("hwid", hwid).Logger()
	cl, err := c.GetClient(ctx, email)
	if err != nil {
		return err
	}
	body := map[string]any{"userUuid": cl.UUID, "hwid": hwid}
	if _, _, err := c.do(ctx, http.MethodPost, "api/hwid/devices/delete", body); err != nil {
		log.Error().Err(err).Msg("deleteDevice: request failed")
		return fmt.Errorf("deleteDevice %s: %w", email, err)
	}
	log.Info().Msg("deleteDevice: ok")
	return nil
}
