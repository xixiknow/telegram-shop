package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"telegram-shop/internal/config"
	"telegram-shop/internal/model"
	"telegram-shop/internal/payment"
	"telegram-shop/internal/store"
	"telegram-shop/internal/sub2api"
)

// 每个用例独占随机 schema；不读取部署配置，也不对已有 schema 执行迁移。
func integrationStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_DSN to an isolated PostgreSQL test database")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatal("TEST_DATABASE_DSN must be a PostgreSQL URL")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	schema := fmt.Sprintf("promo_test_%d", time.Now().UnixNano())
	if _, err := db.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DROP SCHEMA "` + schema + `" CASCADE`) })
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	st, err := store.New(u.String())
	if err != nil {
		t.Fatal(err)
	}
	conn, err := st.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return st
}

type quoteProvider struct {
	method, currency string
	requested        []float64
	afterCreate      func()
	err              error
}

func (p *quoteProvider) Method() string          { return p.method }
func (p *quoteProvider) SuccessResponse() string { return "success" }
func (p *quoteProvider) VerifyNotification(context.Context, string, map[string]string) (*payment.Notification, error) {
	return nil, nil
}
func (p *quoteProvider) Create(_ context.Context, _ string, amount float64) (*payment.CreateResult, error) {
	p.requested = append(p.requested, amount)
	if p.afterCreate != nil {
		p.afterCreate()
	}
	if p.err != nil {
		return nil, p.err
	}
	pay := amount
	if p.currency == "USDT" {
		pay = 12.345678
	}
	return &payment.CreateResult{PayAmount: pay, PayCurrency: p.currency}, nil
}

func TestPromotionOrderLifecycle(t *testing.T) {
	st := integrationStore(t)
	for i, tc := range []struct {
		name, mode, method, currency                 string
		amount, percent, pay, credit, gift, discount float64
	}{
		{"discount CNY", config.PromotionDiscount, payment.MethodEasyPay, "CNY", 100, 10, 90, 100, 0, 10},
		{"discount TRC20", config.PromotionDiscount, payment.MethodUSDTTRC20, "USDT", 100, 10, 90, 100, 0, 10},
		{"discount BEP20", config.PromotionDiscount, payment.MethodUSDTBEP20, "USDT", 33.33, 12.5, 29.16, 33.33, 0, 4.17},
		{"gift CNY", config.PromotionGift, payment.MethodEasyPay, "CNY", 100, 10, 100, 110, 10, 0},
		{"gift USDT", config.PromotionGift, payment.MethodUSDTBEP20, "USDT", 33.33, 12.5, 33.33, 37.5, 4.17, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			cfg := &config.Config{Amounts: []float64{100}, MinAmount: 1, MaxAmount: 1000,
				Payment: config.PaymentConfig{OrderTimeoutMinutes: 30},
				Promotion: config.PromotionConfig{Enabled: true, Mode: tc.mode, Percent: tc.percent,
					StartAt: now.Add(-time.Hour).Format(time.RFC3339Nano), EndAt: now.Add(time.Second).Format(time.RFC3339Nano)}}
			requests := make(chan sub2api.RechargeRequest, 4)
			var fail atomic.Bool
			fail.Store(true)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req sub2api.RechargeRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				requests <- req
				if fail.Load() {
					w.WriteHeader(500)
					return
				}
				_, _ = w.Write([]byte("success"))
			}))
			defer srv.Close()
			p := &quoteProvider{method: tc.method, currency: tc.currency}
			svc := NewOrderService(cfg, st, map[string]payment.Provider{tc.method: p}, sub2api.New(srv.URL, "", "test", 2, 0))
			svc.now = func() time.Time { return now }
			// 模拟网关响应跨过活动结束，快照必须采用下单前的时刻。
			p.afterCreate = func() { svc.now = func() time.Time { return now.Add(time.Minute) } }
			in := CreateOrderInput{TelegramUserID: int64(100 + i), Sub2APIEmail: "user@example.test", AmountCNY: tc.amount, PaymentMethod: tc.method}
			o, result, err := svc.CreateOrder(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			if len(p.requested) != 1 || p.requested[0] != tc.pay {
				t.Fatalf("gateway charge: %v", p.requested)
			}
			saved, err := st.GetOrderByNo(o.OrderNo)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Amount != tc.amount || saved.PayableCNY() != tc.pay || saved.CreditAmount() != tc.credit ||
				saved.GiftAmount != tc.gift || saved.DiscountAmount != tc.discount || saved.PromotionMode != tc.mode || saved.PromotionPercent != tc.percent || saved.PayAmount != result.PayAmount {
				t.Fatalf("wrong persisted snapshot: %+v", saved)
			}
			// 活动结束后旧按钮也不能绕过已有待付款订单限制。
			if _, _, err := svc.CreateOrder(context.Background(), in); !errors.Is(err, ErrActivePendingOrder) {
				t.Fatalf("expected pending guard, got %v", err)
			}
			if len(p.requested) != 1 {
				t.Fatal("pending guard called gateway")
			}
			cfg.Promotion = config.PromotionConfig{Enabled: true, Mode: config.PromotionGift, Percent: 99}
			if _, err := svc.HandlePaymentSuccess(context.Background(), o.OrderNo, "trade-test", saved.PayAmount); err == nil {
				t.Fatal("expected simulated fulfillment failure")
			}
			check := func(req sub2api.RechargeRequest) {
				if req.OrderNo != o.OrderNo || req.Amount != tc.credit || req.BaseAmount != tc.pay || req.TradeNo != "trade-test" {
					t.Fatalf("incorrect recharge: %+v", req)
				}
			}
			check(<-requests)
			fail.Store(false)
			if orders := svc.RetryStuckOrders(context.Background()); len(orders) != 1 || orders[0] != o.OrderNo {
				t.Fatalf("retry result: %v", orders)
			}
			check(<-requests)
			if _, err := svc.HandlePaymentSuccess(context.Background(), o.OrderNo, "trade-test", saved.PayAmount); err != nil {
				t.Fatal(err)
			}
			select {
			case req := <-requests:
				t.Fatalf("duplicate recharge: %+v", req)
			default:
			}
			saved, err = st.GetOrderByNo(o.OrderNo)
			if err != nil || saved.Status != model.OrderStatusCompleted {
				t.Fatalf("completion: %+v %v", saved, err)
			}
		})
	}
}

func TestPromotionOrderBoundaryAndFailures(t *testing.T) {
	st := integrationStore(t)
	start, _ := time.Parse(time.RFC3339, "2026-10-01T00:00:00+08:00")
	end := start.Add(time.Hour)
	cfg := &config.Config{Amounts: []float64{100}, MinAmount: 0.01, MaxAmount: 1000,
		Payment: config.PaymentConfig{OrderTimeoutMinutes: 30}, Promotion: config.PromotionConfig{
			Enabled: true, Mode: config.PromotionDiscount, Percent: 10, StartAt: start.Format(time.RFC3339), EndAt: end.Format(time.RFC3339)}}
	p := &quoteProvider{method: payment.MethodEasyPay, currency: "CNY"}
	svc := NewOrderService(cfg, st, map[string]payment.Provider{p.method: p}, nil)
	for i, tc := range []struct {
		at  time.Time
		pay float64
	}{
		{start.Add(-time.Nanosecond), 100}, {start, 90}, {end, 90}, {end.Add(time.Nanosecond), 100},
	} {
		svc.now = func() time.Time { return tc.at }
		o, _, err := svc.CreateOrder(context.Background(), CreateOrderInput{TelegramUserID: int64(200 + i), Sub2APIEmail: "u@example.test", AmountCNY: 100, PaymentMethod: p.method})
		if err != nil || o.PayableCNY() != tc.pay {
			t.Fatalf("boundary: %+v %v", o, err)
		}
	}
	svc.now = func() time.Time { return start }
	before := len(p.requested)
	cfg.Promotion.Percent = 99.99
	if _, _, err := svc.CreateOrder(context.Background(), CreateOrderInput{TelegramUserID: 300, Sub2APIEmail: "u@example.test", AmountCNY: 0.01, PaymentMethod: p.method}); !errors.Is(err, config.ErrInvalidQuote) {
		t.Fatalf("expected invalid quote, got %v", err)
	}
	if len(p.requested) != before {
		t.Fatal("invalid quote reached gateway")
	}
	cfg.Promotion.Percent = 10
	p.err = errors.New("gateway unavailable")
	if _, _, err := svc.CreateOrder(context.Background(), CreateOrderInput{TelegramUserID: 301, Sub2APIEmail: "u@example.test", AmountCNY: 100, PaymentMethod: p.method}); err == nil {
		t.Fatal("expected gateway error")
	}
	orders, err := st.ListOrdersByUser(301, 10)
	if err != nil || len(orders) != 0 {
		t.Fatalf("failed gateway persisted order: %v %v", orders, err)
	}
}

func TestPromotionMigrationPreservesLegacyOrder(t *testing.T) {
	st := integrationStore(t)
	db := st.DB()
	for _, field := range []string{"GiftAmount", "DiscountAmount", "PromotionMode", "PromotionPercent"} {
		if err := db.Migrator().DropColumn(&model.TGOrder{}, field); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO tg_orders (order_no,telegram_user_id,amount,pay_amount,pay_currency,payment_method,sub2_api_email,status) VALUES ('legacy',1,100,13.25,'USDT','usdt_bep20','old@example.test','pending')`).Error; err != nil {
		t.Fatal(err)
	}
	// 手动增量迁移可重复执行，AutoMigrate 后读取旧记录也保持原金额。
	script, err := os.ReadFile("../../migrations/002_promotion_snapshot.sql")
	if err != nil {
		t.Fatal(err)
	}
	for repeat := 0; repeat < 2; repeat++ {
		for _, stmt := range strings.Split(string(script), ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if err := db.Exec(stmt).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.AutoMigrate(&model.TGOrder{}); err != nil {
		t.Fatal(err)
	}
	o, err := st.GetOrderByNo("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if o.PayableCNY() != 100 || o.CreditAmount() != 100 || o.PayAmount != 13.25 || o.PromotionMode != "" {
		t.Fatalf("legacy record changed: %+v", o)
	}
	// 单独验证运行时 AutoMigrate 可以自动补齐新列，无需手工迁移。
	for _, field := range []string{"DiscountAmount", "PromotionMode", "PromotionPercent"} {
		if err := db.Migrator().DropColumn(&model.TGOrder{}, field); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("UPDATE tg_orders SET gift_amount = 10 WHERE order_no = 'legacy'").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.TGOrder{}); err != nil {
		t.Fatal(err)
	}
	o, err = st.GetOrderByNo("legacy")
	if err != nil || o.CreditAmount() != 110 || o.PayableCNY() != 100 {
		t.Fatalf("legacy gift changed: %+v %v", o, err)
	}
}
