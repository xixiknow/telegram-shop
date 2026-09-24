package payment

import (
	"context"
	"encoding/json"
	"testing"

	"telegram-shop/internal/config"
)

// 官方文档 worked example：验证签名算法实现正确。
func TestBEpusdtSignWorkedExample(t *testing.T) {
	params := map[string]string{
		"order_id":     "20220201030210321",
		"amount":       "42",
		"notify_url":   "http://example.com/notify",
		"redirect_url": "http://example.com/redirect",
	}
	got := bepusdtSign(params, "epusdt_password_xasddawqe")
	const want = "1cd4b52df5587cfb1968b0c0c6e156cd"
	if got != want {
		t.Fatalf("sign = %s, want %s", got, want)
	}
}

// 空值与 signature 自身应被排除。
func TestBEpusdtSignExcludesEmptyAndSignature(t *testing.T) {
	a := bepusdtSign(map[string]string{"x": "1", "y": ""}, "tok")
	b := bepusdtSign(map[string]string{"x": "1", "y": "", "signature": "zzz"}, "tok")
	c := bepusdtSign(map[string]string{"x": "1"}, "tok")
	if a != c || b != c {
		t.Fatalf("empty/signature not excluded: a=%s b=%s c=%s", a, b, c)
	}
}

func newTestProvider() *BEpusdt {
	return NewBEpusdt(MethodUSDTTRC20, "usdt.trc20", "TRC20", config.USDTConfig{
		BaseURL:  "http://bepusdt.local",
		APIToken: "tok123",
		Fiat:     "CNY",
	}, "http://shop.local/api/webhook/usdt")
}

// 构造带正确签名的 status=2 回调，应解析为成功。
func TestVerifyNotificationSuccess(t *testing.T) {
	p := newTestProvider()
	body := map[string]any{
		"trade_id":             "uuid-1",
		"order_id":             "tgshop_abc",
		"amount":               float64(50),
		"actual_amount":        "6.94",
		"token":                "TWalletAddr",
		"block_transaction_id": "0xtxhash",
		"status":               float64(2),
	}
	signParams := map[string]string{}
	for k, v := range body {
		signParams[k] = stringifyJSONValue(v)
	}
	body["signature"] = bepusdtSign(signParams, "tok123")
	raw, _ := json.Marshal(body)

	n, err := p.VerifyNotification(context.Background(), string(raw), nil)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if !n.Success {
		t.Error("expected Success=true for status=2")
	}
	if n.OrderNo != "tgshop_abc" {
		t.Errorf("OrderNo = %s, want tgshop_abc", n.OrderNo)
	}
	if n.TradeNo != "0xtxhash" {
		t.Errorf("TradeNo = %s, want 0xtxhash", n.TradeNo)
	}
	if n.Amount < 6.939 || n.Amount > 6.941 {
		t.Errorf("Amount = %v, want ~6.94", n.Amount)
	}
}

// 错误签名应被拒绝。
func TestVerifyNotificationBadSignature(t *testing.T) {
	p := newTestProvider()
	raw := `{"order_id":"x","status":2,"actual_amount":"1","signature":"deadbeef"}`
	if _, err := p.VerifyNotification(context.Background(), raw, nil); err == nil {
		t.Fatal("expected error for bad signature")
	}
}

// status=1/3 应解析为非成功。
func TestVerifyNotificationNonSuccessStatus(t *testing.T) {
	p := newTestProvider()
	for _, st := range []float64{1, 3} {
		body := map[string]any{
			"order_id":      "tgshop_x",
			"actual_amount": "6.94",
			"status":        st,
		}
		signParams := map[string]string{}
		for k, v := range body {
			signParams[k] = stringifyJSONValue(v)
		}
		body["signature"] = bepusdtSign(signParams, "tok123")
		raw, _ := json.Marshal(body)

		n, err := p.VerifyNotification(context.Background(), string(raw), nil)
		if err != nil {
			t.Fatalf("status=%v verify failed: %v", st, err)
		}
		if n.Success {
			t.Errorf("status=%v should not be Success", st)
		}
	}
}
