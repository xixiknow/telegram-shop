package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEasyPayCreateDiscountedAmount(t *testing.T) {
	e := testEasyPay(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/api/pay/create" || r.Form.Get("money") != "29.16" || r.Form.Get("out_trade_no") != "discount-order" {
			t.Errorf("unexpected request: %s %v", r.URL.Path, r.Form)
		}
		params := map[string]string{}
		for key := range r.Form {
			params[key] = r.Form.Get(key)
		}
		if err := e.verify(params, params["sign"]); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"code":0,"pay_type":"qrcode","pay_info":"https://example.test/qr"}`))
	}))
	defer srv.Close()
	e.cfg.GatewayURL = srv.URL
	result, err := e.Create(context.Background(), "discount-order", 29.16)
	if err != nil {
		t.Fatal(err)
	}
	if result.PayAmount != 29.16 || result.PayCurrency != "CNY" {
		t.Fatalf("result: %+v", result)
	}
}

func TestBEpusdtCreateDiscountedAmount(t *testing.T) {
	for _, tradeType := range []string{"usdt.trc20", "usdt.bep20"} {
		t.Run(tradeType, func(t *testing.T) {
			p := newTestProvider()
			p.tradeType = tradeType
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.URL.Path != bepusdtCreatePath || body["amount"] != 29.16 || body["fiat"] != "CNY" || body["trade_type"] != tradeType {
					t.Errorf("unexpected payload: %v", body)
				}
				params := map[string]string{}
				for key, value := range body {
					params[key] = stringifyJSONValue(value)
				}
				if body["signature"] != bepusdtSign(params, "tok123") {
					t.Error("invalid request signature")
				}
				_, _ = w.Write([]byte(`{"status_code":200,"data":{"token":"test-wallet","actual_amount":"4.123456","amount":29.16}}`))
			}))
			defer srv.Close()
			p.baseURL = srv.URL
			result, err := p.Create(context.Background(), "discount-order", 29.16)
			if err != nil {
				t.Fatal(err)
			}
			if result.PayAmount != 4.123456 || result.PayCurrency != "USDT" {
				t.Fatalf("result: %+v", result)
			}
		})
	}
}
