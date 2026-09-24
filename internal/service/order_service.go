// Package service 提供订单与业务编排逻辑。
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"telegram-shop/internal/config"
	"telegram-shop/internal/model"
	"telegram-shop/internal/payment"
	"telegram-shop/internal/store"
	"telegram-shop/internal/sub2api"
)

// PaymentCreateResult 是 payment.CreateResult 的别名，便于上层（bot）引用而无需直接依赖 payment 包。
type PaymentCreateResult = payment.CreateResult

// OrderService 编排订单创建、支付与履约。
type OrderService struct {
	cfg       *config.Config
	store     *store.Store
	providers map[string]payment.Provider
	sub2api   *sub2api.Client
	now       func() time.Time
}

// NewOrderService 创建订单服务。
func NewOrderService(
	cfg *config.Config,
	st *store.Store,
	providers map[string]payment.Provider,
	s2a *sub2api.Client,
) *OrderService {
	return &OrderService{cfg: cfg, store: st, providers: providers, sub2api: s2a, now: time.Now}
}

// QueryBalance 透传到 sub2api 客户端，查询指定 email 用户的余额。
// 供 Bot 交互式调用；bot 只依赖 OrderService，故经此薄封装转调。
func (s *OrderService) QueryBalance(ctx context.Context, email string) (*sub2api.BalanceResult, error) {
	return s.sub2api.QueryBalance(ctx, email)
}

// CreateOrderInput 是创建订单的入参。
type CreateOrderInput struct {
	TelegramUserID   int64
	TelegramUsername string
	Sub2APIEmail     string
	AmountCNY        float64
	PaymentMethod    string
}

// ErrActivePendingOrder 表示用户已有一笔待支付且未过期的订单，
// 受「每用户同时仅允许一笔未付款订单」限制，不再创建新单。
// 返回此错误时，第一个返回值为已存在的那笔订单。
var ErrActivePendingOrder = errors.New("active pending order exists")

// CreateOrder 创建订单并发起支付。
func (s *OrderService) CreateOrder(ctx context.Context, in CreateOrderInput) (*model.TGOrder, *payment.CreateResult, error) {
	provider, ok := s.providers[in.PaymentMethod]
	if !ok {
		return nil, nil, fmt.Errorf("unsupported payment method: %s", in.PaymentMethod)
	}
	if !s.isAllowedAmount(in.AmountCNY) {
		return nil, nil, fmt.Errorf("amount %.2f not allowed", in.AmountCNY)
	}
	if in.Sub2APIEmail == "" {
		return nil, nil, fmt.Errorf("sub2api email not bound")
	}

	now := s.now()
	quote, err := s.cfg.QuoteAt(in.AmountCNY, now)
	if err != nil {
		return nil, nil, fmt.Errorf("quote recharge: %w", err)
	}

	// 限制：每个用户同时只能有一笔待支付且未过期的订单（已过期/已终态不计）。
	// 在向支付方下单之前检查，避免产生无谓的第三方订单。
	if existing, err := s.store.FindActivePendingOrder(in.TelegramUserID, now); err == nil {
		return existing, nil, ErrActivePendingOrder
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, nil, fmt.Errorf("check active pending order: %w", err)
	}

	orderNo := generateOrderNo()
	expiresAt := now.Add(time.Duration(s.cfg.Payment.OrderTimeoutMinutes) * time.Minute)

	result, err := provider.Create(ctx, orderNo, quote.PayableCNY)
	if err != nil {
		return nil, nil, fmt.Errorf("create payment: %w", err)
	}

	// 将下单前的报价保存为快照；付款和补单不再读取实时活动配置。
	order := &model.TGOrder{
		OrderNo:               orderNo,
		TelegramUserID:        in.TelegramUserID,
		TelegramUsername:      in.TelegramUsername,
		Amount:                quote.Amount,
		GiftAmount:            quote.GiftAmount,
		DiscountAmount:        quote.DiscountAmount,
		PromotionMode:         quote.PromotionMode,
		PromotionPercent:      quote.PromotionPercent,
		PayAmount:             result.PayAmount,
		PayCurrency:           result.PayCurrency,
		PaymentMethod:         in.PaymentMethod,
		Status:                model.OrderStatusPending,
		PayURL:                result.PayURL,
		QRCode:                result.QRCode,
		Sub2APIEmail:          in.Sub2APIEmail,
		Sub2APICallbackStatus: model.CallbackStatusPending,
		ExpiresAt:             expiresAt,
	}
	if err := s.store.CreateOrder(order); err != nil {
		return nil, nil, fmt.Errorf("persist order: %w", err)
	}

	slog.Info("order created",
		"orderNo", orderNo,
		"tgUserID", in.TelegramUserID,
		"amount", in.AmountCNY,
		"payable_cny", quote.PayableCNY,
		"credit_amount", quote.CreditAmount,
		"promotion_mode", quote.PromotionMode,
		"method", in.PaymentMethod,
	)
	return order, result, nil
}

// HandlePaymentSuccess 处理支付成功：置 paid -> 回调 sub2api 充值 -> 置 completed。
// 幂等：重复调用安全。返回更新后的订单。
func (s *OrderService) HandlePaymentSuccess(ctx context.Context, orderNo, tradeNo string, paidAmount float64) (*model.TGOrder, error) {
	order, err := s.store.GetOrderByNo(orderNo)
	if err != nil {
		return nil, fmt.Errorf("lookup order %s: %w", orderNo, err)
	}

	// 已完成：幂等返回
	if order.Status == model.OrderStatusCompleted {
		return order, nil
	}

	// 原子置 paid（仅 pending -> paid 生效）
	transitioned, err := s.store.MarkOrderPaid(orderNo, tradeNo, time.Now())
	if err != nil {
		return nil, fmt.Errorf("mark paid: %w", err)
	}
	if transitioned {
		slog.Info("order marked paid", "orderNo", orderNo, "tradeNo", tradeNo, "paid", paidAmount)
	}

	// 重新读取最新状态
	order, err = s.store.GetOrderByNo(orderNo)
	if err != nil {
		return nil, err
	}

	// 回调 sub2api 完成充值（即使之前已 paid 但未 completed，也允许重试）
	if order.Sub2APICallbackStatus != model.CallbackStatusSuccess {
		if err := s.fulfill(ctx, order); err != nil {
			_ = s.store.UpdateOrderFields(orderNo, map[string]any{
				"sub2_api_callback_status": model.CallbackStatusFailed,
				"sub2_api_callback_error":  err.Error(),
			})
			return order, fmt.Errorf("fulfill via sub2api: %w", err)
		}
	}

	completedAt := time.Now()
	_ = s.store.UpdateOrderFields(orderNo, map[string]any{
		"status":                   model.OrderStatusCompleted,
		"sub2_api_callback_status": model.CallbackStatusSuccess,
		"sub2_api_callback_error":  "",
		"completed_at":             completedAt,
	})
	order.Status = model.OrderStatusCompleted
	order.Sub2APICallbackStatus = model.CallbackStatusSuccess
	order.CompletedAt = &completedAt

	slog.Info("order completed", "orderNo", orderNo, "email", order.Sub2APIEmail, "amount", order.Amount)
	return order, nil
}

func (s *OrderService) fulfill(ctx context.Context, order *model.TGOrder) error {
	// 落账金额 = 充值额度 + 活动赠额（合入余额一起充）。
	// 返利基数 = 原价 - 减免金额；使用快照且不含赠额。
	return s.sub2api.Recharge(ctx, sub2api.RechargeRequest{
		OrderNo:    order.OrderNo,
		TradeNo:    order.PaymentTradeNo,
		Email:      order.Sub2APIEmail,
		Amount:     order.CreditAmount(),
		BaseAmount: order.PayableCNY(),
		Status:     "success",
	})
}

// Provider 按支付方式返回 provider。
func (s *OrderService) Provider(method string) (payment.Provider, bool) {
	p, ok := s.providers[method]
	return p, ok
}

// USDTProvider 返回任一已注册的 USDT（BEpusdt）provider，用于回调验签。
// 两条链的 provider 共享同一 API Token，验签逻辑一致，取任一即可。
func (s *OrderService) USDTProvider() (payment.Provider, bool) {
	if p, ok := s.providers[payment.MethodUSDTTRC20]; ok {
		return p, true
	}
	if p, ok := s.providers[payment.MethodUSDTBEP20]; ok {
		return p, true
	}
	return nil, false
}

// Store 暴露底层 store（供 webhook handler 等使用）。
func (s *OrderService) Store() *store.Store { return s.store }

// ExpireOrders 将过期未支付订单标记为 expired。
func (s *OrderService) ExpireOrders(ctx context.Context) {
	n, err := s.store.ExpirePendingOrders(time.Now())
	if err != nil {
		slog.Error("expire orders failed", "error", err)
		return
	}
	if n > 0 {
		slog.Info("expired pending orders", "count", n)
	}
}

// RetryStuckOrders 扫描卡在 paid 状态（支付成功但充值未完成）的订单并重试充值，
// 返回本轮补单成功（变为 completed）的 orderNo 列表，供上层通知用户。
// 兜底场景：支付方回调重试窗口耗尽后，订单仍可由本任务自动补单。
func (s *OrderService) RetryStuckOrders(ctx context.Context) []string {
	orders, err := s.store.ListStuckPaidOrders(100)
	if err != nil {
		slog.Error("retry stuck orders: list failed", "error", err)
		return nil
	}
	var succeeded []string
	for i := range orders {
		o := orders[i]
		updated, err := s.HandlePaymentSuccess(ctx, o.OrderNo, o.PaymentTradeNo, o.PayAmount)
		if err != nil {
			slog.Warn("retry stuck order still failing", "orderNo", o.OrderNo, "error", err)
			continue
		}
		if updated.Status == model.OrderStatusCompleted {
			slog.Info("stuck order auto-recovered", "orderNo", o.OrderNo)
			succeeded = append(succeeded, o.OrderNo)
		}
	}
	return succeeded
}

// isAllowedAmount 校验金额是否可下单：命中配置档位，或落在自定义金额区间 [MinAmount, MaxAmount] 内。
// 自定义金额按两位小数（分）粒度校验，避免浮点误差。
func (s *OrderService) isAllowedAmount(amount float64) bool {
	if !config.ValidAmount(amount) {
		return false
	}
	for _, a := range s.cfg.Amounts {
		if a == amount {
			return true
		}
	}
	// 自定义金额：范围校验 + 必须是合法的两位小数（不接受 0.001 这种）。
	if amount < s.cfg.MinAmount || amount > s.cfg.MaxAmount {
		return false
	}
	cents := math.Round(amount * 100)
	return math.Abs(amount*100-cents) < 1e-6 && cents > 0
}

// generateOrderNo 生成订单号：tgshop_20240614 + 8位随机。
func generateOrderNo() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return "tgshop_" + time.Now().Format("20060102") + string(b)
}
