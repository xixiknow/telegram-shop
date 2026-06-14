// Package config 负责加载与解析应用配置。
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config 是应用的全局配置。
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Telegram TelegramConfig `mapstructure:"telegram"`
	Amounts  []float64      `mapstructure:"amounts"`
	Database DatabaseConfig `mapstructure:"database"`
	Payment  PaymentConfig  `mapstructure:"payment"`
	Sub2API  Sub2APIConfig  `mapstructure:"sub2api"`
}

// ServerConfig 是 HTTP 服务配置。
type ServerConfig struct {
	Addr    string `mapstructure:"addr"`
	BaseURL string `mapstructure:"base_url"`
}

// TelegramConfig 是 Telegram Bot 配置。
type TelegramConfig struct {
	BotToken    string  `mapstructure:"bot_token"`
	UseWebhook  bool    `mapstructure:"use_webhook"`
	WebhookPath string  `mapstructure:"webhook_path"`
	AdminIDs    []int64 `mapstructure:"admin_ids"`
}

// DatabaseConfig 是数据库配置。
type DatabaseConfig struct {
	DSN string `mapstructure:"dsn"`
}

// PaymentConfig 聚合所有支付方式的配置。
type PaymentConfig struct {
	OrderTimeoutMinutes int           `mapstructure:"order_timeout_minutes"`
	EasyPay             EasyPayConfig `mapstructure:"easypay"`
	USDT                USDTConfig    `mapstructure:"usdt"`
}

// EasyPayConfig 是易支付配置。
type EasyPayConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	GatewayURL     string `mapstructure:"gateway_url"`
	MerchantID     string `mapstructure:"merchant_id"`
	MerchantKey    string `mapstructure:"merchant_key"`
	DefaultChannel string `mapstructure:"default_channel"`
}

// USDTConfig 是 USDT 支付配置。
type USDTConfig struct {
	Enabled       bool    `mapstructure:"enabled"`
	Network       string  `mapstructure:"network"`
	WalletAddress string  `mapstructure:"wallet_address"`
	CNYRate       float64 `mapstructure:"cny_rate"`
	UniqueAmount  bool    `mapstructure:"unique_amount"`

	// 内置 TRON 链上轮询配置（TRC20）。启用后无需外部监听服务，
	// bot 进程定时查询 TronGrid，按金额匹配待支付订单并完成充值。
	Poll USDTPollConfig `mapstructure:"poll"`
}

// USDTPollConfig 是 USDT 内置链上轮询配置。
type USDTPollConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	TronGridURL     string `mapstructure:"trongrid_url"`     // 默认 https://api.trongrid.io
	TronGridAPIKey  string `mapstructure:"trongrid_api_key"` // 可选，提升频率限制
	ContractAddress string `mapstructure:"contract_address"` // USDT TRC20 合约地址
	IntervalSeconds int    `mapstructure:"interval_seconds"` // 轮询间隔，默认 30
	LookbackMinutes int    `mapstructure:"lookback_minutes"` // 回溯窗口，默认 = 订单超时 + 10
}

// Sub2APIConfig 是回调 sub2api 的配置。
type Sub2APIConfig struct {
	WebhookURL     string `mapstructure:"webhook_url"`
	WebhookSecret  string `mapstructure:"webhook_secret"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
	MaxRetries     int    `mapstructure:"max_retries"`
}

// Load 从给定路径读取配置文件并解析。
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Telegram.BotToken == "" || c.Telegram.BotToken == "YOUR_BOT_TOKEN_HERE" {
		return fmt.Errorf("telegram.bot_token is required")
	}
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}
	if len(c.Amounts) == 0 {
		return fmt.Errorf("amounts must not be empty")
	}
	if c.Sub2API.WebhookURL == "" {
		return fmt.Errorf("sub2api.webhook_url is required")
	}
	if c.Payment.OrderTimeoutMinutes <= 0 {
		c.Payment.OrderTimeoutMinutes = 30
	}
	if c.Sub2API.TimeoutSeconds <= 0 {
		c.Sub2API.TimeoutSeconds = 15
	}
	if c.Sub2API.MaxRetries <= 0 {
		c.Sub2API.MaxRetries = 5
	}
	if c.Payment.USDT.Poll.Enabled {
		if c.Payment.USDT.Poll.TronGridURL == "" {
			c.Payment.USDT.Poll.TronGridURL = "https://api.trongrid.io"
		}
		if c.Payment.USDT.Poll.ContractAddress == "" {
			// USDT TRC20 主网合约地址
			c.Payment.USDT.Poll.ContractAddress = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
		}
		if c.Payment.USDT.Poll.IntervalSeconds <= 0 {
			c.Payment.USDT.Poll.IntervalSeconds = 30
		}
		if c.Payment.USDT.Poll.LookbackMinutes <= 0 {
			c.Payment.USDT.Poll.LookbackMinutes = c.Payment.OrderTimeoutMinutes + 10
		}
		if c.Payment.USDT.WalletAddress == "" {
			return fmt.Errorf("payment.usdt.wallet_address is required when poll is enabled")
		}
	}
	return nil
}

// IsAdmin 判断给定的 Telegram User ID 是否为管理员。
func (c *Config) IsAdmin(userID int64) bool {
	for _, id := range c.Telegram.AdminIDs {
		if id == userID {
			return true
		}
	}
	return false
}
