package bot

import (
	"sync"
	"time"
)

// rateLimiter 是按 key（通常为 Telegram 用户 ID）的滑动窗口限流器。
// 用于防止恶意高频请求：每个 key 在 window 时间窗内最多允许 limit 次。
// 并发安全。内存实现，无需外部依赖；过期记录在访问时惰性清理。
type rateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	hits   map[int64][]time.Time
}

func newRateLimiter(window time.Duration, limit int) *rateLimiter {
	return &rateLimiter{
		window: window,
		limit:  limit,
		hits:   make(map[int64][]time.Time),
	}
}

// allow 记录一次 key 的访问并返回是否放行。
// 返回 false 表示在当前时间窗内已超过 limit，应拒绝处理。
func (r *rateLimiter) allow(key int64) bool {
	now := time.Now()
	cutoff := now.Add(-r.window)

	r.mu.Lock()
	defer r.mu.Unlock()

	// 保留窗口内的访问时间戳，丢弃过期的。
	times := r.hits[key]
	kept := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) >= r.limit {
		r.hits[key] = kept
		return false
	}

	r.hits[key] = append(kept, now)
	return true
}

// cooldown 是按 key 的最小间隔限制器：同一 key 两次操作间隔须 >= interval。
// 用于给「创建订单」等敏感动作加冷却，防止刷第三方支付下单接口。并发安全。
type cooldown struct {
	mu       sync.Mutex
	interval time.Duration
	last     map[int64]time.Time
}

func newCooldown(interval time.Duration) *cooldown {
	return &cooldown{
		interval: interval,
		last:     make(map[int64]time.Time),
	}
}

// allow 在距上次放行已过 interval 时返回 true 并记录本次时间；否则返回 false。
func (c *cooldown) allow(key int64) bool {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if last, ok := c.last[key]; ok && now.Sub(last) < c.interval {
		return false
	}
	c.last[key] = now
	return true
}
