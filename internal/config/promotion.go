package config

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // 本地运行也使用相同的 Asia/Shanghai 时区规则。
)

const (
	PromotionDiscount = "discount"
	PromotionGift     = "gift"
	// 在 float64 与整数分之间转换的保守上限，避免溢出或精度丢失。
	maxAmountCents int64 = 100_000_000_000_000
)

var ErrInvalidQuote = errors.New("invalid recharge quote")

// RechargeQuote 是一个时刻的报价；所有金额以人民币计，保留两位小数。
type RechargeQuote struct {
	Amount           float64
	PayableCNY       float64
	CreditAmount     float64
	GiftAmount       float64
	DiscountAmount   float64
	PromotionMode    string
	PromotionPercent float64
}

// ValidAmount 拒绝非有限数、非正数、超额及不足分粒度的金额。
func ValidAmount(amount float64) bool {
	return !math.IsNaN(amount) && !math.IsInf(amount, 0) && amount >= 0.01 &&
		amount <= float64(maxAmountCents)/100 &&
		math.Abs(amount*100-math.Round(amount*100)) < 1e-6
}

func (p PromotionConfig) mode() string {
	if p.Mode == "" {
		return PromotionGift
	}
	return p.Mode
}

func parsePromoTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation("2006-01-02 15:04:05", s, loc)
}

func (p PromotionConfig) window() (time.Time, time.Time, error) {
	start, err := parsePromoTime(p.StartAt)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("promotion.start_at: %w", err)
	}
	end, err := parsePromoTime(p.EndAt)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("promotion.end_at: %w", err)
	}
	if !start.IsZero() && !end.IsZero() && start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("promotion.start_at must not be after end_at")
	}
	return start, end, nil
}

func (p PromotionConfig) validate() error {
	if p.mode() != PromotionDiscount && p.mode() != PromotionGift {
		return fmt.Errorf("promotion.mode must be discount or gift")
	}
	if math.IsNaN(p.Percent) || math.IsInf(p.Percent, 0) || p.Percent < 0 ||
		(p.mode() == PromotionDiscount && p.Percent >= 100) {
		return fmt.Errorf("promotion.percent must be finite and non-negative; discount must be below 100")
	}
	_, _, err := p.window()
	return err
}

// ActiveAt 的起止边界包含在内，留空表示该侧不限制；无效配置不会启用活动。
func (p PromotionConfig) ActiveAt(now time.Time) bool {
	if !p.Enabled || p.Percent == 0 || p.validate() != nil {
		return false
	}
	start, end, _ := p.window()
	return (start.IsZero() || !now.Before(start)) && (end.IsZero() || !now.After(end))
}

// WindowLabel 将两端转换为上海时间供菜单展示；空边界仍返回空字符串。
func (p PromotionConfig) WindowLabel() (string, string) {
	start, end, err := p.window()
	if err != nil {
		return "", ""
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	format := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.In(loc).Format("2006-01-02 15:04:05")
	}
	return format(start), format(end)
}

// QuoteAt 是菜单和订单共用的定价入口。折扣先对最终应付金额四舍五入，
// 减免额等于原价减应付；赠额单独四舍五入。模式互斥且不修改配置。
func (c *Config) QuoteAt(amount float64, now time.Time) (RechargeQuote, error) {
	if !ValidAmount(amount) {
		return RechargeQuote{}, ErrInvalidQuote
	}
	cents := int64(math.Round(amount * 100))
	amount = float64(cents) / 100
	q := RechargeQuote{Amount: amount, PayableCNY: amount, CreditAmount: amount}
	if err := c.Promotion.validate(); err != nil {
		return RechargeQuote{}, err
	}
	if !c.Promotion.ActiveAt(now) {
		return q, nil
	}
	p := c.Promotion
	// 从十进制字符串创建有理数，避免 19.99 * 50% 等边界的浮点舍入偏差。
	rate, _ := new(big.Rat).SetString(strconv.FormatFloat(p.Percent, 'f', -1, 64))
	rate.Quo(rate, big.NewRat(100, 1))
	if p.mode() == PromotionDiscount {
		rate.Sub(big.NewRat(1, 1), rate)
	}
	value := new(big.Rat).Mul(big.NewRat(cents, 1), rate)
	value.Add(value, big.NewRat(1, 2))
	rounded := new(big.Int).Quo(value.Num(), value.Denom())
	if !rounded.IsInt64() || rounded.Int64() > maxAmountCents {
		return RechargeQuote{}, ErrInvalidQuote
	}
	adjusted := rounded.Int64()
	q.PromotionMode, q.PromotionPercent = p.mode(), p.Percent
	if p.mode() == PromotionDiscount {
		if adjusted < 1 {
			return RechargeQuote{}, ErrInvalidQuote
		}
		q.PayableCNY = float64(adjusted) / 100
		q.DiscountAmount = float64(cents-adjusted) / 100
	} else {
		if adjusted > maxAmountCents-cents {
			return RechargeQuote{}, ErrInvalidQuote
		}
		q.GiftAmount = float64(adjusted) / 100
		q.CreditAmount = float64(cents+adjusted) / 100
	}
	return q, nil
}

// GiftAmountAt 保留旧调用兼容，折扣模式始终返回零赠额。
func (c *Config) GiftAmountAt(amount float64, now time.Time) float64 {
	q, err := c.QuoteAt(amount, now)
	if err != nil {
		return 0
	}
	return q.GiftAmount
}
