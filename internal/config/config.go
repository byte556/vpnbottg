package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Constructor struct {
	DevicesStep         int             `yaml:"devices_step"`
	PlanGBStep          int             `yaml:"plan_gb_step"`
	FixPrice            float64         `yaml:"fix_price"`
	BaseMultiplier      float64         `yaml:"base_multiplier"`
	WhitelistMultiplier float64         `yaml:"whitelist_multiplier"`
	MonthDiscounts      map[int]float64 `yaml:"month_discounts"`
}
type YooKassa struct {
	ShopID     string `yaml:"shop_id"`
	SecretKey  string `yaml:"secret_key"`
	WebhookURL string `yaml:"webhook_url"`
}
type Bot struct {
	Token       string      `yaml:"token"`
	Constructor Constructor `yaml:"constructor"`
}

type Config struct {
	Bot      Bot      `yaml:"bot"`
	YooKassa YooKassa `yaml:"yookassa"`
}

var Cfg Config

func Load(path string) error {
	f, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(f, &Cfg)
}
