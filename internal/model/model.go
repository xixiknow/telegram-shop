// Package model 定义数据库实体与状态常量。
package model

import (
	"math"
	"time"
)

// 订单状态常量。
const (
	OrderStatusPending   = "pending"   // 待支付
	OrderStatusPaid      = "paid"      // 已支付，待回调充值
	OrderStatusCompleted = "completed" // 已完成（sub2api 充值成功）
	OrderStatusFailed    = "failed"    // 失败
	OrderStatusExpired   = "expired"   // 已过期
)

// 支付方式常量。
const (
	PaymentMethodEasyPay   = "easypay"
	PaymentMethodUSDTTRC20 = "usdt_trc20"
	PaymentMethodUSDTBEP20 = "usdt_bep20"
)

// IsUSDTMethod 判断支付方式是否为 USDT（任一链）。
func IsUSDTMethod(method string) bool {
	return method == PaymentMethodUSDTTRC20 || method == PaymentMethodUSDTBEP20
}

// sub2api 回调状态常量。
const (
	CallbackStatusPending = "pending"
	CallbackStatusSuccess = "success"
	CallbackStatusFailed  = "failed"
)

// TGUser 是 Telegram 用户，记录其绑定的 sub2api 账号。
type TGUser struct {
	ID             int64     `gorm:"primaryKey" json:"id"`
	TelegramUserID int64     `gorm:"uniqueIndex;not null" json:"telegram_user_id"`
	Username       string    `gorm:"size:255" json:"username"`
	FirstName      string    `gorm:"size:255" json:"first_name"`
	LastName       string    `gorm:"size:255" json:"last_name"`
	LanguageCode   string    `gorm:"size:10" json:"language_code"`
	Sub2APIEmail   string    `gorm:"size:255;index" json:"sub2api_email"` // 绑定的 sub2api 邮箱
	IsBlocked      bool      `gorm:"default:false" json:"is_blocked"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (TGUser) TableName() string { return "tg_users" }

// TGOrder 是 Telegram 商城订单。
type TGOrder struct {
	ID      int64  `gorm:"primaryKey" json:"id"`
	OrderNo string `gorm:"uniqueIndex;size:64;not null" json:"order_no"` // tgshop_20240614xxxxxxxx

	TelegramUserID   int64  `gorm:"index;not null" json:"telegram_user_id"`
	TelegramUsername string `gorm:"size:255" json:"telegram_username"`

	// 金额信息
	DiscountAmount   float64 `gorm:"type:decimal(20,2);not null;default:0" json:"discount_amount"` // 减免金额（CNY）
	PromotionMode    string  `gorm:"size:20;not null;default:''" json:"promotion_mode"`
	PromotionPercent float64 `gorm:"not null;default:0" json:"promotion_percent"`
	Amount           float64 `gorm:"type:decimal(20,2);not null" json:"amount"`       // 充值额度（CNY）
	GiftAmount       float64 `gorm:"type:decimal(20,2);default:0" json:"gift_amount"` // 活动赠送金额（CNY），合入余额一起充
	PayAmount        float64 `gorm:"type:decimal(20,8);not null" json:"pay_amount"`   // 实际支付金额（按支付方式币种）
	PayCurrency      string  `gorm:"size:10;default:'CNY'" json:"pay_currency"`       // CNY / USDT
	PaymentMethod    string  `gorm:"size:30;not null" json:"payment_method"`          // easypay / usdt

	// 状态
	Status string `gorm:"size:30;default:'pending';index" json:"status"`

	// 第三方支付信息
	PaymentTradeNo string `gorm:"size:128" json:"payment_trade_no"` // 第三方交易号
	PayURL         string `gorm:"type:text" json:"pay_url"`
	QRCode         string `gorm:"type:text" json:"qr_code"`

	// sub2api 关联
	Sub2APIEmail          string `gorm:"size:255;not null" json:"sub2api_email"`
	Sub2APICallbackStatus string `gorm:"size:30;default:'pending'" json:"sub2api_callback_status"`
	Sub2APICallbackError  string `gorm:"type:text" json:"sub2api_callback_error"`

	// 时间戳
	ExpiresAt   time.Time  `json:"expires_at"`
	PaidAt      *time.Time `json:"paid_at"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (TGOrder) TableName() string { return "tg_orders" }

// CreditAmount 返回到账总额；旧赠送订单不依赖新增快照字段。
func (o *TGOrder) CreditAmount() float64 {
	return math.Round((o.Amount+o.GiftAmount)*100) / 100
}

// PayableCNY 返回锁定的人民币实付基数；不能用 USDT PayAmount 作为返利基数。
func (o *TGOrder) PayableCNY() float64 {
	return math.Round((o.Amount-o.DiscountAmount)*100) / 100
}

// IsFinal 返回订单是否已进入终态（不可再变更支付状态）。
func (o *TGOrder) IsFinal() bool {
	switch o.Status {
	case OrderStatusCompleted, OrderStatusFailed, OrderStatusExpired:
		return true
	default:
		return false
	}
}
