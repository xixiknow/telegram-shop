// Package service 提供订单与业务编排逻辑。
package service

import (
	"context"
	"fmt"
	"log/slog"
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
}

// NewOrderService 创建订单服务。
func NewOrderService(
	cfg *config.Config,
	st *store.Store,
	providers map[string]payment.Provider,
	s2a *sub2api.Client,
) *OrderService {
	return &OrderService{cfg: cfg, store: st, providers: providers, sub2api: s2a}
}

// CreateOrderInput 是创建订单的入参。
type CreateOrderInput struct {
	TelegramUserID   int64
	TelegramUsername string
	Sub2APIEmail     string
	AmountCNY        float64
	PaymentMethod    string
}

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

	orderNo := generateOrderNo()
	now := time.Now()
	expiresAt := now.Add(time.Duration(s.cfg.Payment.OrderTimeoutMinutes) * time.Minute)

	result, err := provider.Create(ctx, orderNo, in.AmountCNY)
	if err != nil {
		return nil, nil, fmt.Errorf("create payment: %w", err)
	}

	order := &model.TGOrder{
		OrderNo:               orderNo,
		TelegramUserID:        in.TelegramUserID,
		TelegramUsername:      in.TelegramUsername,
		Amount:                in.AmountCNY,
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
				"sub2api_callback_status": model.CallbackStatusFailed,
				"sub2api_callback_error":  err.Error(),
			})
			return order, fmt.Errorf("fulfill via sub2api: %w", err)
		}
	}

	completedAt := time.Now()
	_ = s.store.UpdateOrderFields(orderNo, map[string]any{
		"status":                  model.OrderStatusCompleted,
		"sub2api_callback_status": model.CallbackStatusSuccess,
		"sub2api_callback_error":  "",
		"completed_at":            completedAt,
	})
	order.Status = model.OrderStatusCompleted
	order.Sub2APICallbackStatus = model.CallbackStatusSuccess
	order.CompletedAt = &completedAt

	slog.Info("order completed", "orderNo", orderNo, "email", order.Sub2APIEmail, "amount", order.Amount)
	return order, nil
}

func (s *OrderService) fulfill(ctx context.Context, order *model.TGOrder) error {
	return s.sub2api.Recharge(ctx, sub2api.RechargeRequest{
		OrderNo: order.OrderNo,
		TradeNo: order.PaymentTradeNo,
		Email:   order.Sub2APIEmail,
		Amount:  order.Amount,
		Status:  "success",
	})
}

// Provider 按支付方式返回 provider。
func (s *OrderService) Provider(method string) (payment.Provider, bool) {
	p, ok := s.providers[method]
	return p, ok
}

// Store 暴露底层 store（供 webhook handler 做 USDT 金额匹配等）。
func (s *OrderService) Store() *store.Store { return s.store }

// MatchAndFulfillUSDT 实现 payment.OrderMatcher：按链上到账金额匹配一笔待支付
// USDT 订单并完成充值。返回匹配到的订单号；无匹配时返回空字符串、nil error
// （视为无关转账，由调用方标记 seen 不再重试）。
func (s *OrderService) MatchAndFulfillUSDT(ctx context.Context, transfer payment.USDTTransfer) (string, error) {
	// 金额按 ±0.005 USDT 容差区间匹配，吸收浮点/精度差异。
	const tolerance = 0.005
	order, err := s.store.FindPendingOrderByUSDTAmountRange(transfer.Amount-tolerance, transfer.Amount+tolerance)
	if err != nil {
		// 无匹配订单：视为无关转账，返回空订单号、nil error。
		return "", nil
	}

	if _, err := s.HandlePaymentSuccess(ctx, order.OrderNo, transfer.TxID, transfer.Amount); err != nil {
		return "", err
	}
	return order.OrderNo, nil
}

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

func (s *OrderService) isAllowedAmount(amount float64) bool {
	for _, a := range s.cfg.Amounts {
		if a == amount {
			return true
		}
	}
	return false
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
