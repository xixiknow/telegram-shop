// Package config 负责加载与解析应用配置。
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 是应用的全局配置。
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Telegram  TelegramConfig  `mapstructure:"telegram"`
	Amounts   []float64       `mapstructure:"amounts"`
	MinAmount float64         `mapstructure:"min_amount"` // 自定义金额下限（元），默认 1
	MaxAmount float64         `mapstructure:"max_amount"` // 自定义金额上限（元），默认 1000
	Database  DatabaseConfig  `mapstructure:"database"`
	Payment   PaymentConfig   `mapstructure:"payment"`
	Sub2API   Sub2APIConfig   `mapstructure:"sub2api"`
	Promotion PromotionConfig `mapstructure:"promotion"`
}

// PromotionConfig 是充值活动配置。未填写 Mode 的旧配置按 gift 处理。
type PromotionConfig struct {
	Enabled bool    `mapstructure:"enabled"`
	Mode    string  `mapstructure:"mode"`     // discount / gift，互斥
	Percent float64 `mapstructure:"percent"`  // discount: 减免百分比；gift: 赠送百分比
	StartAt string  `mapstructure:"start_at"` // RFC3339 或 "2006-01-02 15:04:05"（Asia/Shanghai）
	EndAt   string  `mapstructure:"end_at"`   // 活动结束，同上
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

// EasyPayConfig 是易支付配置（Dulupay V2，SHA256WithRSA 签名）。
type EasyPayConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	GatewayURL string `mapstructure:"gateway_url"` // 接口地址，如 https://api.dulupay.com/（带末尾斜杠或不带均可）
	MerchantID string `mapstructure:"merchant_id"` // 商户 ID（pid）
	// PlatformPublicKey 平台公钥（裸 base64，无 PEM 头），用于验签回调与 API 响应。
	PlatformPublicKey string `mapstructure:"platform_public_key"`
	// MerchantPrivateKey 商户私钥（裸 base64，PKCS#8，无 PEM 头），用于请求签名。
	MerchantPrivateKey string `mapstructure:"merchant_private_key"`
	DefaultChannel     string `mapstructure:"default_channel"` // alipay / wxpay / qqpay / bank
}

// USDTConfig 是 USDT 收款配置（通过 BEpusdt 网关）。
// 链上确认、收款地址生成、汇率、多链均由 BEpusdt 负责；本服务仅作为其商户。
type USDTConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	BaseURL        string `mapstructure:"base_url"`        // BEpusdt 部署地址，如 https://pay.example.com
	APIToken       string `mapstructure:"api_token"`       // BEpusdt 后台「API 设置」的 Integration Token
	Fiat           string `mapstructure:"fiat"`            // 计价法币，默认 CNY
	TimeoutSeconds int    `mapstructure:"timeout_seconds"` // 调用 BEpusdt 的 HTTP 超时，默认 15
	TRC20Enabled   bool   `mapstructure:"trc20_enabled"`   // 菜单是否显示 TRC20
	BEP20Enabled   bool   `mapstructure:"bep20_enabled"`   // 菜单是否显示 BEP20
}

// Sub2APIConfig 是回调 sub2api 的配置。
type Sub2APIConfig struct {
	WebhookURL string `mapstructure:"webhook_url"`
	// BalanceURL 为只读余额查询端点。留空时由 validate() 默认派生为 WebhookURL + "/balance"。
	BalanceURL     string `mapstructure:"balance_url"`
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
	for _, amount := range c.Amounts {
		if !ValidAmount(amount) {
			return fmt.Errorf("amounts must contain positive finite amounts with at most two decimals")
		}
	}
	if c.MinAmount <= 0 {
		c.MinAmount = 1
	}
	if c.MaxAmount <= 0 {
		c.MaxAmount = 1000
	}
	if c.MinAmount > c.MaxAmount {
		return fmt.Errorf("min_amount (%.2f) must not exceed max_amount (%.2f)", c.MinAmount, c.MaxAmount)
	}
	if !ValidAmount(c.MinAmount) || !ValidAmount(c.MaxAmount) {
		return fmt.Errorf("min_amount and max_amount must be valid amounts")
	}
	if c.Sub2API.WebhookURL == "" {
		return fmt.Errorf("sub2api.webhook_url is required")
	}
	if c.Sub2API.BalanceURL == "" {
		c.Sub2API.BalanceURL = strings.TrimRight(c.Sub2API.WebhookURL, "/") + "/balance"
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
	if c.Payment.USDT.Enabled {
		if c.Payment.USDT.BaseURL == "" {
			return fmt.Errorf("payment.usdt.base_url is required when usdt is enabled")
		}
		if c.Payment.USDT.APIToken == "" {
			return fmt.Errorf("payment.usdt.api_token is required when usdt is enabled")
		}
		if !c.Payment.USDT.TRC20Enabled && !c.Payment.USDT.BEP20Enabled {
			return fmt.Errorf("payment.usdt: at least one of trc20_enabled / bep20_enabled must be true")
		}
		if c.Payment.USDT.Fiat == "" {
			c.Payment.USDT.Fiat = "CNY"
		}
		if c.Payment.USDT.TimeoutSeconds <= 0 {
			c.Payment.USDT.TimeoutSeconds = 15
		}
	}
	return c.Promotion.validate()
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
