package payment

import (
	"context"
	"testing"

	"telegram-shop/internal/config"
)

func testEasyPay() *EasyPay {
	return NewEasyPay(config.EasyPayConfig{
		Enabled:        true,
		GatewayURL:     "https://api.dulupay.com",
		MerchantID:     "10001",
		MerchantKey:    "testkey123",
		DefaultChannel: "alipay",
	}, "https://shop.example.com/api/webhook/easypay", "https://shop.example.com/health")
}

func TestDulupaySignAndVerify(t *testing.T) {
	e := testEasyPay()

	params := map[string]string{
		"pid":          "10001",
		"out_trade_no": "tgshop_20240614abc12345",
		"money":        "50.00",
		"trade_status": "TRADE_SUCCESS",
		"trade_no":     "DLP2024061422001",
		"type":         "alipay",
	}
	sign := e.sign(params)
	if sign == "" {
		t.Fatal("sign should not be empty")
	}

	// 加入签名后验签应通过
	params["sign"] = sign
	params["sign_type"] = "MD5"
	if !e.verifySign(params, sign) {
		t.Fatal("verifySign should pass for correct signature")
	}

	// 篡改金额后验签应失败
	tampered := make(map[string]string, len(params))
	for k, v := range params {
		tampered[k] = v
	}
	tampered["money"] = "500.00"
	if e.verifySign(tampered, sign) {
		t.Fatal("verifySign should fail for tampered amount")
	}
}

func TestDulupayVerifyNotification(t *testing.T) {
	e := testEasyPay()

	params := map[string]string{
		"pid":          "10001",
		"out_trade_no": "tgshop_20240614abc12345",
		"money":        "50.00",
		"trade_status": "TRADE_SUCCESS",
		"trade_no":     "DLP2024061422001",
		"type":         "alipay",
	}
	params["sign"] = e.sign(params)

	n, err := e.VerifyNotification(context.Background(), "", params)
	if err != nil {
		t.Fatalf("VerifyNotification failed: %v", err)
	}
	if n.OrderNo != "tgshop_20240614abc12345" {
		t.Errorf("OrderNo = %q, want tgshop_20240614abc12345", n.OrderNo)
	}
	if n.Amount != 50.00 {
		t.Errorf("Amount = %v, want 50.00", n.Amount)
	}
	if !n.Success {
		t.Error("Success should be true for TRADE_SUCCESS")
	}
}
