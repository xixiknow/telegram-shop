package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"telegram-shop/internal/config"
)

// USDTTransfer 是一笔链上 TRC20 转入记录（已归一化为人类可读金额）。
type USDTTransfer struct {
	TxID   string
	Amount float64 // USDT（已按 decimals 归一化）
	From   string
	To     string
	TS     time.Time
}

// OrderMatcher 由订单服务实现，供 poller 按金额匹配并完成充值。
type OrderMatcher interface {
	// MatchAndFulfillUSDT 按到账金额匹配一笔待支付 USDT 订单并完成充值。
	// 返回匹配到的订单号；无匹配时返回空字符串、nil error（视为无关转账）。
	MatchAndFulfillUSDT(ctx context.Context, transfer USDTTransfer) (orderNo string, err error)
}

// USDTPoller 定时轮询 TronGrid，发现 USDT 到账并触发充值。
type USDTPoller struct {
	cfg        config.USDTPollConfig
	wallet     string
	matcher    OrderMatcher
	httpClient *http.Client

	seen *seenCache // 进程内去重，避免重复处理同一交易
}

// NewUSDTPoller 创建 USDT 链上轮询器。
func NewUSDTPoller(cfg config.USDTPollConfig, walletAddress string, matcher OrderMatcher) *USDTPoller {
	return &USDTPoller{
		cfg:        cfg,
		wallet:     walletAddress,
		matcher:    matcher,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		seen:       newSeenCache(2000),
	}
}

// Run 启动轮询循环（阻塞，受 ctx 控制）。
func (p *USDTPoller) Run(ctx context.Context) {
	interval := time.Duration(p.cfg.IntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("usdt tron poller started",
		"wallet", p.wallet,
		"interval", interval.String(),
		"lookback_min", p.cfg.LookbackMinutes,
	)

	// 启动即跑一次，缩短首单等待
	p.pollOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			slog.Info("usdt tron poller stopped")
			return
		case <-ticker.C:
			p.pollOnce(ctx)
		}
	}
}

func (p *USDTPoller) pollOnce(ctx context.Context) {
	since := time.Now().Add(-time.Duration(p.cfg.LookbackMinutes) * time.Minute)
	transfers, err := p.fetchTransfers(ctx, since)
	if err != nil {
		slog.Error("usdt poll: fetch transfers failed", "error", err)
		return
	}

	for _, t := range transfers {
		// 仅处理转入本钱包的记录
		if !strings.EqualFold(t.To, p.wallet) {
			continue
		}
		if p.seen.has(t.TxID) {
			continue
		}

		orderNo, err := p.matcher.MatchAndFulfillUSDT(ctx, t)
		if err != nil {
			slog.Error("usdt poll: fulfill failed", "txid", t.TxID, "amount", t.Amount, "error", err)
			continue // 不标记 seen，下轮重试
		}
		p.seen.add(t.TxID)
		if orderNo != "" {
			slog.Info("usdt poll: order fulfilled", "orderNo", orderNo, "txid", t.TxID, "amount", t.Amount)
		}
	}
}

// trc20Response 是 TronGrid TRC20 转账查询的响应结构。
// GET /v1/accounts/{address}/transactions/trc20
type trc20Response struct {
	Data []struct {
		TransactionID  string `json:"transaction_id"`
		From           string `json:"from"`
		To             string `json:"to"`
		Value          string `json:"value"`     // 最小单位字符串
		BlockTimestamp int64  `json:"block_timestamp"` // 毫秒
		TokenInfo      struct {
			Address  string `json:"address"`
			Decimals int    `json:"decimals"`
		} `json:"token_info"`
	} `json:"data"`
	Success bool `json:"success"`
}

func (p *USDTPoller) fetchTransfers(ctx context.Context, since time.Time) ([]USDTTransfer, error) {
	endpoint := fmt.Sprintf("%s/v1/accounts/%s/transactions/trc20",
		strings.TrimRight(p.cfg.TronGridURL, "/"), p.wallet)

	q := url.Values{}
	q.Set("only_to", "true") // 仅转入本地址
	q.Set("limit", "50")
	q.Set("order_by", "block_timestamp,desc")
	q.Set("min_timestamp", strconv.FormatInt(since.UnixMilli(), 10))
	if p.cfg.ContractAddress != "" {
		q.Set("contract_address", p.cfg.ContractAddress)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if p.cfg.TronGridAPIKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", p.cfg.TronGridAPIKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request trongrid: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trongrid returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed trc20Response
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse trongrid response: %w", err)
	}

	out := make([]USDTTransfer, 0, len(parsed.Data))
	for _, d := range parsed.Data {
		// 合约地址过滤（双保险，避免其它 TRC20 代币）
		if p.cfg.ContractAddress != "" && !strings.EqualFold(d.TokenInfo.Address, p.cfg.ContractAddress) {
			continue
		}
		raw, err := strconv.ParseFloat(d.Value, 64)
		if err != nil {
			continue
		}
		decimals := d.TokenInfo.Decimals
		if decimals <= 0 {
			decimals = 6 // USDT-TRC20 默认 6 位
		}
		amount := raw / math.Pow10(decimals)
		out = append(out, USDTTransfer{
			TxID:   d.TransactionID,
			Amount: amount,
			From:   d.From,
			To:     d.To,
			TS:     time.UnixMilli(d.BlockTimestamp),
		})
	}
	return out, nil
}

// seenCache 是一个简单的 FIFO 去重缓存（进程内）。
type seenCache struct {
	max   int
	set   map[string]struct{}
	order []string
}

func newSeenCache(max int) *seenCache {
	return &seenCache{max: max, set: make(map[string]struct{}, max)}
}

func (c *seenCache) has(k string) bool {
	_, ok := c.set[k]
	return ok
}

func (c *seenCache) add(k string) {
	if _, ok := c.set[k]; ok {
		return
	}
	c.set[k] = struct{}{}
	c.order = append(c.order, k)
	if len(c.order) > c.max {
		old := c.order[0]
		c.order = c.order[1:]
		delete(c.set, old)
	}
}
