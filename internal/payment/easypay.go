package payment

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
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

// Dulupay V2（彩虹易支付 2.0）协议常量。
const (
	tradeStatusSuccess = "TRADE_SUCCESS"
	easyPaySuccessResp = "success"
	easyPaySignType    = "RSA"
	// easyPaySignWindow 是回调/响应 timestamp 的有效窗口，与 SDK 的 300 秒一致。
	easyPaySignWindow = 300 * time.Second
)

// EasyPay 实现 Dulupay V2 易支付。
//
// 采用「API 下单」模式（api/pay/create）：下单时用商户私钥对参数做
// SHA256WithRSA 签名，POST 到网关，返回 JSON 含支付二维码内容（pay_info），
// 由 telegram-shop 在 Telegram 内渲染二维码。支付结果由异步回调确认，用平台公钥验签。
type EasyPay struct {
	cfg        config.EasyPayConfig
	notifyURL  string
	returnURL  string
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	httpClient *http.Client
}

// NewEasyPay 创建 Dulupay V2 provider。
// 解析失败（密钥格式错误）会 panic，应在启动装配阶段尽早暴露配置问题。
func NewEasyPay(cfg config.EasyPayConfig, notifyURL, returnURL string) *EasyPay {
	priv, err := parseRSAPrivateKey(cfg.MerchantPrivateKey)
	if err != nil {
		panic(fmt.Sprintf("easypay: parse merchant private key: %v", err))
	}
	pub, err := parseRSAPublicKey(cfg.PlatformPublicKey)
	if err != nil {
		panic(fmt.Sprintf("easypay: parse platform public key: %v", err))
	}
	return &EasyPay{
		cfg:        cfg,
		notifyURL:  notifyURL,
		returnURL:  returnURL,
		privateKey: priv,
		publicKey:  pub,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Method 返回支付方式标识。
func (e *EasyPay) Method() string { return "easypay" }

// SuccessResponse 返回 Dulupay 要求的成功应答。
func (e *EasyPay) SuccessResponse() string { return easyPaySuccessResp }

// easyPayCreateResp 是 api/pay/create 的响应结构。
type easyPayCreateResp struct {
	Code      int    `json:"code"`
	Msg       string `json:"msg"`
	TradeNo   string `json:"trade_no"`
	PayType   string `json:"pay_type"` // qrcode / jump / html ...
	PayInfo   string `json:"pay_info"` // qrcode: 二维码内容URL；jump: 跳转URL
	Timestamp string `json:"timestamp"`
	SignType  string `json:"sign_type"`
	Sign      string `json:"sign"`
}

// Create 通过 Dulupay V2 API 下单（api/pay/create），返回支付二维码内容。
func (e *EasyPay) Create(ctx context.Context, orderNo string, amountCNY float64) (*CreateResult, error) {
	params := map[string]string{
		"pid":          e.cfg.MerchantID,
		"type":         e.cfg.DefaultChannel, // alipay / wxpay / qqpay / bank
		"out_trade_no": orderNo,
		"notify_url":   e.notifyURL,
		"return_url":   e.returnURL,
		"name":         fmt.Sprintf("余额充值 %.2f 元", amountCNY),
		"money":        strconv.FormatFloat(amountCNY, 'f', 2, 64),
		"clientip":     "127.0.0.1",
		"device":       "pc",
		"timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
	}
	sign, err := e.sign(params)
	if err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}
	params["sign"] = sign
	params["sign_type"] = easyPaySignType

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(e.cfg.GatewayURL, "/")+"/api/pay/create",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request dulupay: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed easyPayCreateResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse dulupay response: %w (body=%s)", err, strings.TrimSpace(string(body)))
	}
	if parsed.Code != 0 {
		return nil, fmt.Errorf("dulupay error: %s", parsed.Msg)
	}
	if parsed.PayInfo == "" {
		return nil, fmt.Errorf("dulupay response missing pay_info (pay_type=%s)", parsed.PayType)
	}

	res := &CreateResult{
		PayAmount:   amountCNY,
		PayCurrency: "CNY",
		Extra: map[string]string{
			"pay_type": parsed.PayType,
			"channel":  e.cfg.DefaultChannel,
			"trade_no": parsed.TradeNo,
		},
	}
	// qrcode：pay_info 为二维码内容（如支付宝收款码 URL），在 Telegram 内渲染二维码。
	// jump/其它：pay_info 为跳转 URL，作为"去支付"按钮。
	if parsed.PayType == "jump" {
		res.PayURL = parsed.PayInfo
	} else {
		res.QRCode = parsed.PayInfo
	}
	return res, nil
}

// VerifyNotification 解析并验签 Dulupay V2 异步回调（GET query string）。
// 用平台公钥验 sign，并校验 timestamp 时间窗，防重放。
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

	// timestamp 时间窗校验（与 SDK 的 abs(time-timestamp)>300 一致）。
	if ts := strings.TrimSpace(params["timestamp"]); ts != "" {
		tsInt, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp")
		}
		if d := time.Since(time.Unix(tsInt, 0)); d > easyPaySignWindow || d < -easyPaySignWindow {
			return nil, fmt.Errorf("timestamp out of window")
		}
	}

	if err := e.verify(params, sign); err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}

	amount, _ := strconv.ParseFloat(params["money"], 64)
	return &Notification{
		OrderNo: params["out_trade_no"],
		TradeNo: params["trade_no"],
		Amount:  amount,
		Success: params["trade_status"] == tradeStatusSuccess,
	}, nil
}

// signContent 按 Dulupay V2 规则生成待签名字符串：
// 对非空、非 sign/sign_type 的参数按 key 升序拼接 k=v，用 & 连接（末尾不追加密钥）。
func signContent(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || strings.TrimSpace(v) == "" {
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
	return buf.String()
}

// sign 用商户私钥对待签名字符串做 SHA256WithRSA 签名，返回 base64。
func (e *EasyPay) sign(params map[string]string) (string, error) {
	digest := sha256.Sum256([]byte(signContent(params)))
	sig, err := rsa.SignPKCS1v15(rand.Reader, e.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// verify 用平台公钥验证签名。
func (e *EasyPay) verify(params map[string]string, sign string) error {
	sig, err := base64.StdEncoding.DecodeString(sign)
	if err != nil {
		return fmt.Errorf("decode sign: %w", err)
	}
	digest := sha256.Sum256([]byte(signContent(params)))
	return rsa.VerifyPKCS1v15(e.publicKey, crypto.SHA256, digest[:], sig)
}

// parseRSAPrivateKey 解析裸 base64 的 PKCS#8 商户私钥（SDK 配置格式）。
// 兼容 PKCS#1（BEGIN RSA PRIVATE KEY）作为回退。
func parseRSAPrivateKey(b64 string) (*rsa.PrivateKey, error) {
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA private key")
		}
		return rsaKey, nil
	}
	// 回退尝试 PKCS#1
	rsaKey, err := x509.ParsePKCS1PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse private key (tried PKCS#8 and PKCS#1): %w", err)
	}
	return rsaKey, nil
}

// parseRSAPublicKey 解析裸 base64 的 PKIX 平台公钥（SDK 配置格式）。
func parseRSAPublicKey(b64 string) (*rsa.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	return rsaPub, nil
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
