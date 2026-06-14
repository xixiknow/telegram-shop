package payment

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"sync"

	"telegram-shop/internal/config"
)

// USDT 实现基于「金额唯一匹配」的 USDT 收款。
//
// 工作原理：所有订单收款到同一钱包地址。为区分订单，在 USDT 金额尾部
// 追加一个微小的随机小数（如 7.23 -> 7.2347），使每笔待支付订单的应付
// 金额唯一。链上到账后，由外部监听服务按「金额 + 地址」匹配订单，并向
// 本服务的 /api/webhook/usdt 推送通知（同样经 HMAC 签名校验，见 handler）。
//
// 该 provider 仅负责生成应付金额与收款地址；到账确认由 webhook handler 完成。
type USDT struct {
	cfg config.USDTConfig

	mu       sync.Mutex
	rng      *rand.Rand
	usedTail map[int]struct{} // 进程内已用尾数去重（尽力而为）
}

// NewUSDT 创建 USDT provider。
func NewUSDT(cfg config.USDTConfig) *USDT {
	return &USDT{
		cfg:      cfg,
		rng:      rand.New(rand.NewSource(randSeed())),
		usedTail: make(map[int]struct{}),
	}
}

// Method 返回支付方式标识。
func (u *USDT) Method() string { return "usdt" }

// SuccessResponse USDT 由内部监听回调，返回标准 success。
func (u *USDT) SuccessResponse() string { return "success" }

// Create 计算应付 USDT 金额并返回收款地址。
func (u *USDT) Create(_ context.Context, _ string, amountCNY float64) (*CreateResult, error) {
	rate := u.cfg.CNYRate
	if rate <= 0 {
		return nil, fmt.Errorf("usdt cny_rate not configured")
	}

	base := amountCNY / rate
	payAmount := math.Round(base*100) / 100 // 先保留 2 位

	if u.cfg.UniqueAmount {
		// 追加 2 位随机尾数（0.0001 ~ 0.0099），使金额唯一
		tail := u.nextTail()
		payAmount = math.Round((payAmount+float64(tail)/10000.0)*10000) / 10000
	}

	return &CreateResult{
		QRCode:      u.cfg.WalletAddress, // 二维码内容 = 钱包地址
		PayAmount:   payAmount,
		PayCurrency: "USDT",
		Extra: map[string]string{
			"network":        u.cfg.Network,
			"wallet_address": u.cfg.WalletAddress,
			"pay_amount":     strconv.FormatFloat(payAmount, 'f', 4, 64),
		},
	}, nil
}

// VerifyNotification 对 USDT 而言，链上监听服务会以内部签名格式回调，
// 这里不做协议级解析（由 webhook handler 统一处理 HMAC + 金额匹配）。
// 保留接口实现以满足 Provider；正常不会被调用。
func (u *USDT) VerifyNotification(_ context.Context, _ string, _ map[string]string) (*Notification, error) {
	return nil, fmt.Errorf("usdt notification handled by dedicated webhook handler")
}

func (u *USDT) nextTail() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i := 0; i < 100; i++ {
		t := u.rng.Intn(99) + 1 // 1..99
		if _, ok := u.usedTail[t]; !ok {
			u.usedTail[t] = struct{}{}
			// 防止 map 无限增长
			if len(u.usedTail) > 90 {
				u.usedTail = map[int]struct{}{t: {}}
			}
			return t
		}
	}
	return u.rng.Intn(99) + 1
}
