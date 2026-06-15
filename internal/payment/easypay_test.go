package payment

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"strconv"
	"testing"
	"time"

	"telegram-shop/internal/config"
)

// newTestKeys 生成一对测试用 RSA 密钥，返回裸 base64 的私钥(PKCS#8)与公钥(PKIX)。
func newTestKeys(t *testing.T) (privB64, pubB64 string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public: %v", err)
	}
	return base64.StdEncoding.EncodeToString(privDER), base64.StdEncoding.EncodeToString(pubDER)
}

func testEasyPay(t *testing.T) *EasyPay {
	t.Helper()
	// 商户与平台在测试中共用同一对密钥：私钥签名、公钥验签可往返验证。
	priv, pub := newTestKeys(t)
	return NewEasyPay(config.EasyPayConfig{
		Enabled:            true,
		GatewayURL:         "https://api.dulupay.com",
		MerchantID:         "10001",
		PlatformPublicKey:  pub,
		MerchantPrivateKey: priv,
		DefaultChannel:     "alipay",
	}, "https://shop.example.com/api/webhook/easypay", "https://shop.example.com/health")
}

func TestDulupayV2SignAndVerify(t *testing.T) {
	e := testEasyPay(t)

	params := map[string]string{
		"pid":          "10001",
		"out_trade_no": "tgshop_20240614abc12345",
		"money":        "50.00",
		"trade_status": "TRADE_SUCCESS",
		"trade_no":     "DLP2024061422001",
		"type":         "alipay",
		"timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
	}
	sign, err := e.sign(params)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	if sign == "" {
		t.Fatal("sign should not be empty")
	}

	// 正确签名验签应通过
	if err := e.verify(params, sign); err != nil {
		t.Fatalf("verify should pass for correct signature: %v", err)
	}

	// 篡改金额后验签应失败
	tampered := make(map[string]string, len(params))
	for k, v := range params {
		tampered[k] = v
	}
	tampered["money"] = "500.00"
	if err := e.verify(tampered, sign); err == nil {
		t.Fatal("verify should fail for tampered amount")
	}
}

func TestDulupayV2VerifyNotification(t *testing.T) {
	e := testEasyPay(t)

	params := map[string]string{
		"pid":          "10001",
		"out_trade_no": "tgshop_20240614abc12345",
		"money":        "50.00",
		"trade_status": "TRADE_SUCCESS",
		"trade_no":     "DLP2024061422001",
		"type":         "alipay",
		"timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
	}
	sign, err := e.sign(params)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	params["sign"] = sign
	params["sign_type"] = easyPaySignType

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

func TestDulupayV2RejectsStaleTimestamp(t *testing.T) {
	e := testEasyPay(t)

	params := map[string]string{
		"pid":          "10001",
		"out_trade_no": "tgshop_20240614abc12345",
		"money":        "50.00",
		"trade_status": "TRADE_SUCCESS",
		"trade_no":     "DLP2024061422001",
		"type":         "alipay",
		// 超出 300 秒窗口
		"timestamp": strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10),
	}
	sign, err := e.sign(params)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	params["sign"] = sign

	if _, err := e.VerifyNotification(context.Background(), "", params); err == nil {
		t.Fatal("VerifyNotification should reject stale timestamp")
	}
}
