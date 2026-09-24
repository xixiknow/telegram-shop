// Package sub2api 提供回调 sub2api 完成充值的客户端。
//
// 集成契约（与 sub2api 侧约定）：
//
//	POST {webhook_url}
//	Headers:
//	  Content-Type: application/json
//	  X-TGShop-Timestamp: <unix 秒>
//	  X-TGShop-Nonce: <随机串>
//	  X-TGShop-Signature: hex(HMAC_SHA256(secret, timestamp + "." + nonce + "." + rawBody))
//	Body (JSON):
//	  {
//	    "order_no": "tgshop_20240614abc123",
//	    "trade_no": "第三方交易号",
//	    "email":    "user@example.com",
//	    "amount":   50.00,
//	    "status":   "success"
//	  }
//
// sub2api 侧据此创建/补登订单记录并为该 email 用户充值余额。
// 成功响应：HTTP 200，body 含 "success"。
package sub2api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client 是 sub2api 充值回调客户端。
type Client struct {
	webhookURL string
	balanceURL string
	secret     string
	maxRetries int
	httpClient *http.Client
}

// RechargeRequest 是发往 sub2api 的充值请求体。
type RechargeRequest struct {
	OrderNo string `json:"order_no"`
	TradeNo string `json:"trade_no"`
	Email   string `json:"email"`
	// Amount 为落账总额（充值额度 + 活动赠额），即实际计入用户余额的金额。
	Amount float64 `json:"amount"`
	// BaseAmount 为实付充值额度（不含活动赠额），作为邀请返利的计提基数，
	// 与 sub2api 标准支付路径口径一致（赠送金额不产生返利）。
	BaseAmount float64 `json:"base_amount"`
	Status     string  `json:"status"`
}

// New 创建 sub2api 客户端。balanceURL 为只读余额查询端点（可为空则禁用查询）。
func New(webhookURL, balanceURL, secret string, timeoutSeconds, maxRetries int) *Client {
	return &Client{
		webhookURL: webhookURL,
		balanceURL: balanceURL,
		secret:     secret,
		maxRetries: maxRetries,
		httpClient: &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
	}
}

// Recharge 向 sub2api 发起充值回调，带指数退避重试。
func (c *Client) Recharge(ctx context.Context, req RechargeRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避：1s, 2s, 4s, 8s, ...（上限 30s）
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		_, err := c.doSignedPost(ctx, c.webhookURL, body)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("recharge failed after %d retries: %w", c.maxRetries, lastErr)
}

// BalanceResult 是 sub2api 余额查询的返回。
type BalanceResult struct {
	Balance       float64 `json:"balance"`
	FrozenBalance float64 `json:"frozen_balance"`
}

// QueryBalance 向 sub2api 查询指定 email 用户的余额。只读、单次（不重试），
// 供 Bot 交互式调用，失败即返回，避免用户久等。
func (c *Client) QueryBalance(ctx context.Context, email string) (*BalanceResult, error) {
	if c.balanceURL == "" {
		return nil, fmt.Errorf("balance query not configured")
	}
	body, err := json.Marshal(map[string]string{"email": email})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	respBody, err := c.doSignedPost(ctx, c.balanceURL, body)
	if err != nil {
		return nil, err
	}
	var result BalanceResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &result, nil
}

// doSignedPost 向 url 发起带 HMAC 签名的 POST，返回响应体（成功时）。
func (c *Client) doSignedPost(ctx context.Context, url string, body []byte) ([]byte, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce, err := randomNonce(16)
	if err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	signature := c.sign(timestamp, nonce, body)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-TGShop-Timestamp", timestamp)
	httpReq.Header.Set("X-TGShop-Nonce", nonce)
	httpReq.Header.Set("X-TGShop-Signature", signature)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sub2api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

// sign 生成 HMAC-SHA256 签名：hex(HMAC(secret, timestamp + "." + nonce + "." + body))。
func (c *Client) sign(timestamp, nonce string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(c.secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write([]byte(nonce))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func randomNonce(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
