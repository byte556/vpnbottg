package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// envOr возвращает значение переменной окружения если задано, иначе fallback.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type Trial struct {
	PriceRub int `yaml:"price_rub"`
	GB       int `yaml:"gb"`
	Devices  int `yaml:"devices"`
	Days     int `yaml:"days"`
}

type Constructor struct {
	GBStep         int             `yaml:"gb_step"`
	MinGB          int             `yaml:"min_gb"`
	DefaultGB      int             `yaml:"default_gb"`
	PricePerGB     int             `yaml:"price_per_gb"`
	PricePerDevice int             `yaml:"price_per_device"`
	DevicesStep    int             `yaml:"devices_step"`
	DefaultDevices int             `yaml:"default_devices"`
	MonthDiscounts map[int]float64 `yaml:"month_discounts"`
	Trial          Trial           `yaml:"trial"`
}

type YooKassa struct {
	ShopID      string `yaml:"shop_id"`
	SecretKey   string `yaml:"secret_key"`
	WebhookURL  string `yaml:"webhook_url"`
	WebhookPort int    `yaml:"webhook_port"`
	// ValidateIPs — проверять ли IP отправителя вебхука по списку YooKassa.
	// Включать только если бот стоит за reverse proxy с X-Real-IP.
	ValidateIPs bool `yaml:"validate_ips"`
}

type XUI struct {
	Host           string `yaml:"host"`
	Path           string `yaml:"path"`
	Token          string `yaml:"token"`
	InboundsDirect []int  `yaml:"inbound_ids_direct"`
	InboundsRelay  []int  `yaml:"inbound_ids_relay"`
	SubURLTemplate string `yaml:"sub_url_template"`
}

type Bot struct {
	Token              string      `yaml:"token"`
	AdminID            int64       `yaml:"admin_id"`
	Support            string      `yaml:"support"`
	ReferralRewardDays int         `yaml:"referral_reward_days"`
	Constructor        Constructor `yaml:"constructor"`
}

// SubServer — отдельный HTTP-сервер выдачи конфигов happ-клиенту.
type SubServer struct {
	Port             int    `yaml:"port"`              // порт sub-сервера (0 = выключен)
	PublicBaseURL    string `yaml:"public_base_url"`   // внешний адрес сервера, напр. "https://vpn.example.com:8081"
	UpstreamTemplate string `yaml:"upstream_template"` // fmt-шаблон URL подписки в панели, напр. "https://panel/sub/%s"
}

type Config struct {
	Bot       Bot       `yaml:"bot"`
	YooKassa  YooKassa  `yaml:"yookassa"`
	XUI       XUI       `yaml:"xui"`
	SubServer SubServer `yaml:"sub_server"`
}

var Cfg Config

func Load(path string) error {
	f, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(f, &Cfg); err != nil {
		return err
	}
	// Переменные окружения перекрывают config.yaml (удобно для деплоя без секретов в файле).
	Cfg.Bot.Token = envOr("BOT_TOKEN", Cfg.Bot.Token)
	Cfg.YooKassa.ShopID = envOr("YK_SHOP_ID", Cfg.YooKassa.ShopID)
	Cfg.YooKassa.SecretKey = envOr("YK_SECRET_KEY", Cfg.YooKassa.SecretKey)
	Cfg.XUI.Token = envOr("XUI_TOKEN", Cfg.XUI.Token)
	return nil
}
