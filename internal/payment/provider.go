// Package payment 提供支付方式抽象与各服务商实现。
package payment

import "context"

// 支付方式标识常量。
const (
	MethodEasyPay = "easypay"
	// MethodUSDTTRC20 / MethodUSDTBEP20 是按链区分的 USDT 支付方式标识，
	// 分别映射到 BEpusdt 的 trade_type usdt.trc20 / usdt.bep20。
	MethodUSDTTRC20 = "usdt_trc20"
	MethodUSDTBEP20 = "usdt_bep20"
)

// CreateResult 是创建支付的结果。
type CreateResult struct {
	PayURL      string  // 跳转支付的 URL（易支付）
	QRCode      string  // 二维码内容（USDT 钱包地址 / 易支付二维码内容）
	PayAmount   float64 // 实际应支付金额
	PayCurrency string  // 支付币种：CNY / USDT
	Extra       map[string]string
}

// Notification 是解析并验签后的支付通知。
type Notification struct {
	OrderNo string  // 商户订单号
	TradeNo string  // 第三方交易号
	Amount  float64 // 实付金额
	Success bool    // 是否支付成功
}

// Provider 是支付方式的统一接口。
type Provider interface {
	// Method 返回支付方式标识（easypay / usdt）。
	Method() string
	// Create 发起支付。amountCNY 是优惠后的应付人民币金额，未必等于到账额度。
	Create(ctx context.Context, orderNo string, amountCNY float64) (*CreateResult, error)
	// VerifyNotification 解析并验签回调。GET 回调传 query string，POST 传 body。
	// 返回 nil notification 表示无关事件（调用方应回 200）。
	VerifyNotification(ctx context.Context, rawBody string, query map[string]string) (*Notification, error)
	// SuccessResponse 返回应答给服务商的成功响应文本。
	SuccessResponse() string
}
