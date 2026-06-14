// Package store 提供数据库连接与数据访问层。
package store

import (
	"errors"
	"fmt"
	"time"

	"telegram-shop/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ErrNotFound 表示记录不存在。
var ErrNotFound = errors.New("record not found")

// Store 封装数据库连接与数据访问方法。
type Store struct {
	db *gorm.DB
}

// New 创建 Store 并自动迁移表结构。
func New(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.AutoMigrate(&model.TGUser{}, &model.TGOrder{}); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	return &Store{db: db}, nil
}

// DB 暴露底层 *gorm.DB（用于高级查询）。
func (s *Store) DB() *gorm.DB { return s.db }

// --- TGUser ---

// UpsertUser 根据 TelegramUserID 创建或更新用户基础信息（不覆盖已绑定邮箱）。
func (s *Store) UpsertUser(u *model.TGUser) error {
	var existing model.TGUser
	err := s.db.Where("telegram_user_id = ?", u.TelegramUserID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.Create(u).Error
	}
	if err != nil {
		return err
	}
	// 仅更新资料字段，保留已绑定的 sub2api_email
	return s.db.Model(&existing).Updates(map[string]any{
		"username":      u.Username,
		"first_name":    u.FirstName,
		"last_name":     u.LastName,
		"language_code": u.LanguageCode,
	}).Error
}

// GetUserByTelegramID 按 Telegram User ID 查询用户。
func (s *Store) GetUserByTelegramID(tgUserID int64) (*model.TGUser, error) {
	var u model.TGUser
	err := s.db.Where("telegram_user_id = ?", tgUserID).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// BindEmail 绑定 sub2api 邮箱到指定 Telegram 用户。
func (s *Store) BindEmail(tgUserID int64, email string) error {
	res := s.db.Model(&model.TGUser{}).
		Where("telegram_user_id = ?", tgUserID).
		Update("sub2api_email", email)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// --- TGOrder ---

// CreateOrder 创建订单。
func (s *Store) CreateOrder(o *model.TGOrder) error {
	return s.db.Create(o).Error
}

// GetOrderByNo 按订单号查询。
func (s *Store) GetOrderByNo(orderNo string) (*model.TGOrder, error) {
	var o model.TGOrder
	err := s.db.Where("order_no = ?", orderNo).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ListOrdersByUser 查询用户最近的订单。
func (s *Store) ListOrdersByUser(tgUserID int64, limit int) ([]model.TGOrder, error) {
	var orders []model.TGOrder
	err := s.db.Where("telegram_user_id = ?", tgUserID).
		Order("created_at DESC").
		Limit(limit).
		Find(&orders).Error
	return orders, err
}

// UpdateOrderFields 更新订单的指定字段。
func (s *Store) UpdateOrderFields(orderNo string, fields map[string]any) error {
	return s.db.Model(&model.TGOrder{}).
		Where("order_no = ?", orderNo).
		Updates(fields).Error
}

// MarkOrderPaid 原子地将订单从 pending 置为 paid（幂等：仅当前状态为 pending 才生效）。
// 返回 true 表示本次调用成功完成了状态转换。
func (s *Store) MarkOrderPaid(orderNo, tradeNo string, paidAt time.Time) (bool, error) {
	res := s.db.Model(&model.TGOrder{}).
		Where("order_no = ? AND status = ?", orderNo, model.OrderStatusPending).
		Updates(map[string]any{
			"status":           model.OrderStatusPaid,
			"payment_trade_no": tradeNo,
			"paid_at":          paidAt,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ExpirePendingOrders 将所有过期且仍处于 pending 的订单置为 expired，返回受影响行数。
func (s *Store) ExpirePendingOrders(now time.Time) (int64, error) {
	res := s.db.Model(&model.TGOrder{}).
		Where("status = ? AND expires_at < ?", model.OrderStatusPending, now).
		Update("status", model.OrderStatusExpired)
	return res.RowsAffected, res.Error
}

// FindPaidOrderByUSDTAmount 用于 USDT 金额匹配：按支付方式 + 精确金额查找待支付订单。
func (s *Store) FindPendingOrderByUSDTAmount(payAmount float64) (*model.TGOrder, error) {
	var o model.TGOrder
	err := s.db.Where(
		"payment_method = ? AND pay_currency = ? AND status = ? AND pay_amount = ?",
		model.PaymentMethodUSDT, "USDT", model.OrderStatusPending, payAmount,
	).Order("created_at ASC").First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// FindPendingOrderByUSDTAmountRange 在 [low, high] 金额区间内匹配最早的待支付 USDT 订单。
// 用于链上轮询：到账金额与应付金额可能有极小的浮点/精度差异，按区间匹配更稳健。
func (s *Store) FindPendingOrderByUSDTAmountRange(low, high float64) (*model.TGOrder, error) {
	var o model.TGOrder
	err := s.db.Where(
		"payment_method = ? AND pay_currency = ? AND status = ? AND pay_amount >= ? AND pay_amount <= ?",
		model.PaymentMethodUSDT, "USDT", model.OrderStatusPending, low, high,
	).Order("created_at ASC").First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}
