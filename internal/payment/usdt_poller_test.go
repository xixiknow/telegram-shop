package payment

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"telegram-shop/internal/config"
)

// stubMatcher 记录被匹配到的转账，便于断言。
type stubMatcher struct {
	got       []USDTTransfer
	returnNo  string
	returnErr error
}

func (m *stubMatcher) MatchAndFulfillUSDT(_ context.Context, t USDTTransfer) (string, error) {
	m.got = append(m.got, t)
	return m.returnNo, m.returnErr
}

const trongridSample = `{
  "success": true,
  "data": [
    {
      "transaction_id": "tx_aaa",
      "from": "TSenderAddr1",
      "to": "TWalletAddr",
      "value": "7234000",
      "block_timestamp": 1718000000000,
      "token_info": {"address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", "decimals": 6}
    },
    {
      "transaction_id": "tx_bbb",
      "from": "TSenderAddr2",
      "to": "TWalletAddr",
      "value": "100000000",
      "block_timestamp": 1718000001000,
      "token_info": {"address": "TOtherTokenContractXXXXXXXXXXXXXXX", "decimals": 6}
    }
  ]
}`

func TestUSDTPollerFetchAndMatch(t *testing.T) {
	var gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("TRON-PRO-API-KEY")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, trongridSample)
	}))
	defer srv.Close()

	cfg := config.USDTPollConfig{
		Enabled:         true,
		TronGridURL:     srv.URL,
		TronGridAPIKey:  "test-key",
		ContractAddress: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		IntervalSeconds: 30,
		LookbackMinutes: 40,
	}
	matcher := &stubMatcher{returnNo: "tgshop_test"}
	poller := NewUSDTPoller(cfg, "TWalletAddr", matcher)

	poller.pollOnce(context.Background())

	// 只有匹配合约 + 转入本钱包的 tx_aaa 应被处理；tx_bbb 是其它代币，应过滤。
	if len(matcher.got) != 1 {
		t.Fatalf("expected 1 matched transfer, got %d", len(matcher.got))
	}
	got := matcher.got[0]
	if got.TxID != "tx_aaa" {
		t.Errorf("txid = %s, want tx_aaa", got.TxID)
	}
	// 7234000 / 10^6 = 7.234
	if got.Amount < 7.2339 || got.Amount > 7.2341 {
		t.Errorf("amount = %v, want ~7.234", got.Amount)
	}
	if gotAPIKey != "test-key" {
		t.Errorf("api key header = %q, want test-key", gotAPIKey)
	}

	// 第二次轮询：tx_aaa 已 seen，不应重复处理。
	poller.pollOnce(context.Background())
	if len(matcher.got) != 1 {
		t.Errorf("after second poll expected still 1 (dedup), got %d", len(matcher.got))
	}
}

func TestSeenCacheEviction(t *testing.T) {
	c := newSeenCache(2)
	c.add("a")
	c.add("b")
	c.add("c") // 应淘汰最早的 "a"

	if c.has("a") {
		t.Error("expected 'a' to be evicted")
	}
	if !c.has("b") || !c.has("c") {
		t.Error("expected 'b' and 'c' to remain")
	}
}
