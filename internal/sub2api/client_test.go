package sub2api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRechargeSignatureAndDelivery(t *testing.T) {
	const secret = "test-secret"

	var gotBody []byte
	var gotTS, gotNonce, gotSig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotTS = r.Header.Get("X-TGShop-Timestamp")
		gotNonce = r.Header.Get("X-TGShop-Nonce")
		gotSig = r.Header.Get("X-TGShop-Signature")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer srv.Close()

	c := New(srv.URL, "", secret, 5, 0)
	err := c.Recharge(context.Background(), RechargeRequest{
		OrderNo: "tgshop_20240614abc12345",
		TradeNo: "trade-1",
		Email:   "user@example.com",
		Amount:  50,
		Status:  "success",
	})
	if err != nil {
		t.Fatalf("Recharge failed: %v", err)
	}

	// 服务端按相同算法重算签名，应一致
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(gotTS))
	mac.Write([]byte("."))
	mac.Write([]byte(gotNonce))
	mac.Write([]byte("."))
	mac.Write(gotBody)
	want := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(want), []byte(gotSig)) {
		t.Fatalf("signature mismatch:\n got=%s\nwant=%s", gotSig, want)
	}
}

func TestRechargeRetriesOnFailure(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer srv.Close()

	c := New(srv.URL, "", "s", 5, 3)
	if err := c.Recharge(context.Background(), RechargeRequest{OrderNo: "o", Email: "e@x.com", Amount: 10, Status: "success"}); err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if attempts < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", attempts)
	}
}

func TestQueryBalanceSignatureAndParse(t *testing.T) {
	const secret = "bal-secret"

	var gotBody []byte
	var gotTS, gotNonce, gotSig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotTS = r.Header.Get("X-TGShop-Timestamp")
		gotNonce = r.Header.Get("X-TGShop-Nonce")
		gotSig = r.Header.Get("X-TGShop-Signature")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"email":"user@example.com","balance":12.34,"frozen_balance":5.6,"status":"success"}`))
	}))
	defer srv.Close()

	// balanceURL 走第二个参数；webhookURL 此测试不用。
	c := New("", srv.URL, secret, 5, 0)
	result, err := c.QueryBalance(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("QueryBalance failed: %v", err)
	}
	if result.Balance != 12.34 || result.FrozenBalance != 5.6 {
		t.Fatalf("unexpected result: %+v", result)
	}

	// 服务端按相同算法重算签名，应一致。
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(gotTS))
	mac.Write([]byte("."))
	mac.Write([]byte(gotNonce))
	mac.Write([]byte("."))
	mac.Write(gotBody)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(gotSig)) {
		t.Fatalf("signature mismatch:\n got=%s\nwant=%s", gotSig, want)
	}
}

func TestQueryBalanceNotConfigured(t *testing.T) {
	c := New("https://example.com/webhook/tgshop", "", "s", 5, 0)
	if _, err := c.QueryBalance(context.Background(), "u@x.com"); err == nil {
		t.Fatal("expected error when balance URL not configured")
	}
}

func TestQueryBalanceErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("user not found"))
	}))
	defer srv.Close()

	c := New("", srv.URL, "s", 5, 0)
	if _, err := c.QueryBalance(context.Background(), "missing@x.com"); err == nil {
		t.Fatal("expected error on non-200 response")
	}
}
