package payment

import (
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"telegram-shop/internal/config"
)

// Dulupay 标准协议常量。
const (
	tradeStatusSuccess = "TRADE_SUCCESS"
	easyPaySuccessResp = "success"
)

// EasyPay 实现 Dulupay 易支付（mapi.php 接口，MD5 签名）。
type EasyPay struct {
	cfg       config.EasyPayConfig
	notifyURL string
	returnURL string
}

// NewEasyPay 创建 Dulupay provider。
// notifyURL / returnURL 由上层根据 server.base_url 拼接传入。
func NewEasyPay(cfg config.EasyPayConfig, notifyURL, returnURL string) *EasyPay {
	return &EasyPay{cfg: cfg, notifyURL: notifyURL, returnURL: returnURL}
}

// Method 返回支付方式标识。
func (e *EasyPay) Method() string { return "easypay" }

// SuccessResponse 返回 Dulupay 要求的成功应答。
func (e *EasyPay) SuccessResponse() string { return easyPaySuccessResp }

// Create 调用 Dulupay mapi.php 创建订单，返回支付链接。
func (e *EasyPay) Create(ctx context.Context, orderNo string, amountCNY float64) (*CreateResult, error) {
	params := map[string]string{
		"pid":          e.cfg.MerchantID,
		"type":         e.cfg.DefaultChannel, // alipay / wxpay
		"out_trade_no": orderNo,
		"notify_url":   e.notifyURL,
		"return_url":   e.returnURL,
		"name":         fmt.Sprintf("余额充值 %.2f 元", amountCNY),
		"money":        strconv.FormatFloat(amountCNY, 'f', 2, 64),
		"clientip":     "127.0.0.1", // Dulupay 要求必传，telegram 场景无真实 IP
	}
	params["sign"] = e.sign(params)
	params["sign_type"] = "MD5"

	apiURL := strings.TrimRight(e.cfg.GatewayURL, "/") + "/mapi.php"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(buildForm(params)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request dulupay: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var result struct {
		Code   int    `json:"code"`
		Msg    string `json:"msg"`
		PayURL string `json:"payurl"`
		QRCode string `json:"qrcode"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse dulupay response: %w", err)
	}
	if result.Code != 1 {
		return nil, fmt.Errorf("dulupay error: %s", result.Msg)
	}

	payURL := result.PayURL
	if payURL == "" {
		payURL = result.QRCode // 部分场景只返回二维码
	}

	return &CreateResult{
		PayURL:      payURL,
		QRCode:      result.QRCode,
		PayAmount:   amountCNY,
		PayCurrency: "CNY",
	}, nil
}

// VerifyNotification 解析并验签 Dulupay 异步回调（GET query string）。
func (e *EasyPay) VerifyNotification(_ context.Context, rawBody string, query map[string]string) (*Notification, error) {
	params := query
	if len(params) == 0 {
		values, err := url.ParseQuery(rawBody)
		if err != nil {
			return nil, fmt.Errorf("parse notify body: %w", err)
		}
		params = make(map[string]string, len(values))
		for k := range values {
			params[k] = values.Get(k)
		}
	}

	sign := params["sign"]
	if sign == "" {
		return nil, fmt.Errorf("missing sign")
	}
	if !e.verifySign(params, sign) {
		return nil, fmt.Errorf("invalid signature")
	}

	amount, _ := strconv.ParseFloat(params["money"], 64)
	return &Notification{
		OrderNo: params["out_trade_no"],
		TradeNo: params["trade_no"],
		Amount:  amount,
		Success: params["trade_status"] == tradeStatusSuccess,
	}, nil
}

// sign 按 Dulupay 规则生成 MD5 签名：
// 对非空、非 sign/sign_type 的参数按 key 升序拼接 k=v&...，末尾追加 KEY，取 MD5（小写）。
func (e *EasyPay) sign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
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
	buf.WriteString(e.cfg.MerchantKey)

	hash := md5.Sum([]byte(buf.String()))
	return hex.EncodeToString(hash[:])
}

func (e *EasyPay) verifySign(params map[string]string, sign string) bool {
	expected := e.sign(params)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(sign)) == 1
}

// buildForm 将 map 转为 application/x-www-form-urlencoded 格式（稳定排序便于调试）。
func buildForm(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	values := url.Values{}
	for _, k := range keys {
		values.Set(k, params[k])
	}
	return values.Encode()
}
