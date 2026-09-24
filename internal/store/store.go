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
	// 仅更新资料字段，保留已绑定的 sub2api_email 与用户手动选择的 language_code。
	return s.db.Model(&existing).Updates(map[string]any{
		"username":   u.Username,
		"first_name": u.FirstName,
		"last_name":  u.LastName,
	}).Error
}

// SetLanguage 设置用户的语言偏好（手动切换时调用）。
func (s *Store) SetLanguage(tgUserID int64, lang string) error {
	res := s.db.Model(&model.TGUser{}).
		Where("telegram_user_id = ?", tgUserID).
		Update("language_code", lang)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
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
		Update("Sub2APIEmail", email)
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

// FindActivePendingOrder 返回用户当前「待支付且未过期」的订单（如有）。
// 用于限制每个用户同时只能有一笔有效未付款订单；已过期/已终态的不计入。
// 无匹配时返回 ErrNotFound。
func (s *Store) FindActivePendingOrder(tgUserID int64, now time.Time) (*model.TGOrder, error) {
	var o model.TGOrder
	err := s.db.
		Where("telegram_user_id = ? AND status = ? AND expires_at > ?",
			tgUserID, model.OrderStatusPending, now).
		Order("created_at DESC").
		First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
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

// ListStuckPaidOrders 返回已支付但充值未完成的订单（status=paid 且回调失败/未完成），
// 用于后台自动补单重试。按创建时间升序，最多返回 limit 条。
func (s *Store) ListStuckPaidOrders(limit int) ([]model.TGOrder, error) {
	var orders []model.TGOrder
	err := s.db.
		Where("status = ? AND sub2_api_callback_status IN ?",
			model.OrderStatusPaid,
			[]string{model.CallbackStatusFailed, model.CallbackStatusPending}).
		Order("created_at ASC").
		Limit(limit).
		Find(&orders).Error
	return orders, err
}
