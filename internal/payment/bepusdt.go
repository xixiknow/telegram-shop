package payment

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"telegram-shop/internal/config"
)

// BEpusdt 接入常量。
const (
	bepusdtCreatePath = "/api/v1/order/create-transaction"
	// BEpusdt 回调 status 取值。
	bepusdtStatusWaiting = 1
	bepusdtStatusSuccess = 2
	bepusdtStatusTimeout = 3
	// bepusdtSuccessResp 是商户对 status=2 必须回应的文本（不区分大小写）。
	bepusdtSuccessResp = "ok"
)

// BEpusdt 实现基于 BEpusdt 网关的 USDT 收款 provider。
//
// 每个实例绑定一条链（trade_type，如 usdt.trc20 / usdt.bep20）。下单时调用
// BEpusdt 的 create-transaction 接口拿到收款地址与应付 USDT 金额，由 telegram-shop
// 自行在 Telegram 内渲染（不使用 BEpusdt 托管收银台）。支付到账由 BEpusdt 异步
// 回调 /api/webhook/usdt，按 MD5(Token) 签名校验。
type BEpusdt struct {
	method     string // 本 provider 的支付方式标识（usdt_trc20 / usdt_bep20）
	tradeType  string // BEpusdt trade_type（usdt.trc20 / usdt.bep20）
	network    string // 展示用网络名（TRC20 / BEP20）
	baseURL    string
	apiToken   string
	fiat       string
	notifyURL  string
	timeoutSec int
	httpClient *http.Client
}

// NewBEpusdt 创建一个绑定特定链的 BEpusdt provider。
func NewBEpusdt(method, tradeType, network string, cfg config.USDTConfig, notifyURL string) *BEpusdt {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 15
	}
	fiat := cfg.Fiat
	if fiat == "" {
		fiat = "CNY"
	}
	return &BEpusdt{
		method:     method,
		tradeType:  tradeType,
		network:    network,
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiToken:   cfg.APIToken,
		fiat:       fiat,
		notifyURL:  notifyURL,
		timeoutSec: timeout,
		httpClient: &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

// Method 返回支付方式标识。
func (b *BEpusdt) Method() string { return b.method }

// SuccessResponse 返回 BEpusdt 要求的成功应答。
func (b *BEpusdt) SuccessResponse() string { return bepusdtSuccessResp }

// createResp 是 create-transaction 的响应结构。
// 注意：BEpusdt 对 data 内数字/字符串字段的序列化不固定，
// 数值类字段统一用 json.Number 容错（可接受 JSON 数字或字符串）。
type bepusdtCreateResp struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       struct {
		TradeID        string      `json:"trade_id"`
		OrderID        string      `json:"order_id"`
		Amount         json.Number `json:"amount"`
		ActualAmount   json.Number `json:"actual_amount"`
		Token          string      `json:"token"`
		ExpirationTime int64       `json:"expiration_time"`
		Status         json.Number `json:"status"`
		PaymentURL     string      `json:"payment_url"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

// Create 调用 BEpusdt 创建交易，返回收款地址与应付 USDT 金额。
func (b *BEpusdt) Create(ctx context.Context, orderNo string, amountCNY float64) (*CreateResult, error) {
	amountStr := strconv.FormatFloat(amountCNY, 'f', -1, 64)
	// 签名基于字符串形式的参数（BEpusdt 签名规则）。
	params := map[string]string{
		"order_id":     orderNo,
		"amount":       amountStr,
		"notify_url":   b.notifyURL,
		"redirect_url": b.baseURL, // 纯 Telegram 内渲染，不跳转；占位即可
		"trade_type":   b.tradeType,
		"fiat":         b.fiat,
	}
	signature := bepusdtSign(params, b.apiToken)

	// 请求体里 amount 须为数字（BEpusdt 要求 float64），其余为字符串。
	reqBody := map[string]any{
		"order_id":     orderNo,
		"amount":       amountCNY,
		"notify_url":   b.notifyURL,
		"redirect_url": b.baseURL,
		"trade_type":   b.tradeType,
		"fiat":         b.fiat,
		"signature":    signature,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		b.baseURL+bepusdtCreatePath, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request bepusdt: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bepusdt returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed bepusdtCreateResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parse bepusdt response: %w", err)
	}
	if parsed.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bepusdt error: %s", parsed.Message)
	}
	actualAmount := parsed.Data.ActualAmount.String()
	if parsed.Data.Token == "" || actualAmount == "" {
		return nil, fmt.Errorf("bepusdt response missing token/actual_amount")
	}

	payAmount, _ := strconv.ParseFloat(actualAmount, 64)

	return &CreateResult{
		QRCode:      parsed.Data.Token, // 二维码内容 = 收款地址
		PayAmount:   payAmount,
		PayCurrency: "USDT",
		Extra: map[string]string{
			"network":        b.network,
			"wallet_address": parsed.Data.Token,
			"pay_amount":     actualAmount,
			"trade_id":       parsed.Data.TradeID,
		},
	}, nil
}

// bepusdtNotify 是 BEpusdt 回调 body 结构。
type bepusdtNotify struct {
	TradeID            string  `json:"trade_id"`
	OrderID            string  `json:"order_id"`
	Amount             float64 `json:"amount"`
	ActualAmount       string  `json:"actual_amount"`
	Token              string  `json:"token"`
	BlockTransactionID string  `json:"block_transaction_id"`
	Status             int     `json:"status"`
	Signature          string  `json:"signature"`
}

// VerifyNotification 解析并验签 BEpusdt 异步回调（POST JSON body）。
func (b *BEpusdt) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*Notification, error) {
	// 解析为 map 以便按 BEpusdt 算法验签（排除 signature 自身）。
	var raw map[string]any
	if err := json.Unmarshal([]byte(rawBody), &raw); err != nil {
		return nil, fmt.Errorf("parse notify body: %w", err)
	}

	sig, _ := raw["signature"].(string)
	if sig == "" {
		return nil, fmt.Errorf("missing signature")
	}

	signParams := make(map[string]string, len(raw))
	for k, v := range raw {
		if k == "signature" {
			continue
		}
		signParams[k] = stringifyJSONValue(v)
	}
	expected := bepusdtSign(signParams, b.apiToken)
	if !strings.EqualFold(expected, sig) {
		return nil, fmt.Errorf("invalid signature")
	}

	var n bepusdtNotify
	if err := json.Unmarshal([]byte(rawBody), &n); err != nil {
		return nil, fmt.Errorf("parse notify struct: %w", err)
	}

	amount, _ := strconv.ParseFloat(n.ActualAmount, 64)
	return &Notification{
		OrderNo: n.OrderID,
		TradeNo: n.BlockTransactionID,
		Amount:  amount,
		Success: n.Status == bepusdtStatusSuccess,
	}, nil
}

// bepusdtSign 按 BEpusdt 规则签名：
// 取非空且非 signature 的参数 → 按 key ASCII 升序 → k=v 用 & 连接 →
// 末尾直接拼接 API Token（无 &）→ MD5 → 小写 hex。
func bepusdtSign(params map[string]string, token string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "signature" || strings.TrimSpace(v) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(k)
		buf.WriteByte('=')
		buf.WriteString(params[k])
	}
	buf.WriteString(token)

	sum := md5.Sum([]byte(buf.String()))
	return hex.EncodeToString(sum[:])
}

// stringifyJSONValue 把 JSON 反序列化得到的值转为签名所需的字符串形式。
// 数字使用最简表示（与 BEpusdt 序列化一致：整数无小数点，浮点去尾零）。
func stringifyJSONValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}
