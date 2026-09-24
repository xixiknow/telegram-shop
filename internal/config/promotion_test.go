package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGiftAmountAt(t *testing.T) {
	cfg := &Config{Promotion: PromotionConfig{
		Enabled: true, Percent: 20,
		StartAt: "2026-06-15 00:00:00",
		EndAt:   "2026-06-30 23:59:59",
	}}
	in := func(s string) time.Time {
		loc, _ := time.LoadLocation("Asia/Shanghai")
		tm, _ := time.ParseInLocation("2006-01-02 15:04:05", s, loc)
		return tm
	}

	// 活动期内：100 * 20% = 20
	if g := cfg.GiftAmountAt(100, in("2026-06-20 12:00:00")); g != 20 {
		t.Errorf("in-window gift = %v, want 20", g)
	}
	// 两位小数：33 * 20% = 6.6
	if g := cfg.GiftAmountAt(33, in("2026-06-20 12:00:00")); g != 6.6 {
		t.Errorf("rounding gift = %v, want 6.6", g)
	}
	// 活动前：0
	if g := cfg.GiftAmountAt(100, in("2026-06-10 12:00:00")); g != 0 {
		t.Errorf("before-window gift = %v, want 0", g)
	}
	// 活动后：0
	if g := cfg.GiftAmountAt(100, in("2026-07-01 12:00:00")); g != 0 {
		t.Errorf("after-window gift = %v, want 0", g)
	}
	// 未启用：0
	off := &Config{Promotion: PromotionConfig{Enabled: false, Percent: 20}}
	if g := off.GiftAmountAt(100, in("2026-06-20 12:00:00")); g != 0 {
		t.Errorf("disabled gift = %v, want 0", g)
	}
}

func promoTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestQuoteAtAmounts(t *testing.T) {
	now := promoTime(t, "2026-10-02T12:00:00+08:00")
	for _, tc := range []struct {
		name, mode                                   string
		amount, percent, pay, credit, discount, gift float64
	}{
		{"discount", PromotionDiscount, 100, 10, 90, 100, 10, 0},
		{"gift", PromotionGift, 100, 10, 100, 110, 0, 10},
		{"legacy mode", "", 100, 20, 100, 120, 0, 20},
		{"custom discount", PromotionDiscount, 33.33, 12.5, 29.16, 33.33, 4.17, 0},
		{"custom gift", PromotionGift, 33.33, 12.5, 33.33, 37.5, 0, 4.17},
		{"half cent discount", PromotionDiscount, 19.99, 50, 10, 19.99, 9.99, 0},
		{"half cent gift", PromotionGift, 19.99, 50, 19.99, 29.99, 0, 10},
		{"half cent exact decimal", PromotionGift, 1, 0.5, 1, 1.01, 0, 0.01},
		{"fractional percent", PromotionDiscount, 100, 10.125, 89.88, 100, 10.12, 0},
		{"zero", PromotionDiscount, 100, 0, 100, 100, 0, 0},
		{"large gift percent", PromotionGift, 1, 200, 1, 3, 0, 2},
		{"one cent", PromotionDiscount, 0.01, 10, 0.01, 0.01, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{Promotion: PromotionConfig{Enabled: true, Mode: tc.mode, Percent: tc.percent}}
			q, err := cfg.QuoteAt(tc.amount, now)
			if err != nil {
				t.Fatal(err)
			}
			if q.PayableCNY != tc.pay || q.CreditAmount != tc.credit || q.DiscountAmount != tc.discount || q.GiftAmount != tc.gift {
				t.Fatalf("unexpected quote: %+v", q)
			}
			if tc.percent > 0 && (q.PromotionMode != cfg.Promotion.mode() || q.PromotionPercent != tc.percent) {
				t.Fatalf("missing snapshot: %+v", q)
			}
		})
	}
}

func TestQuoteAtWindowAndTimezone(t *testing.T) {
	// 故意使用 UTC 作为宿主时区：无时区配置依然按照上海时间处理。
	old := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = old })
	cfg := Config{Promotion: PromotionConfig{Enabled: true, Mode: PromotionDiscount, Percent: 10,
		StartAt: "2026-10-01 00:00:00", EndAt: "2026-10-07 23:59:59"}}
	for _, tc := range []struct {
		ts   string
		want float64
	}{
		{"2026-09-30T15:59:59.999999999Z", 100},
		{"2026-09-30T16:00:00Z", 90},
		{"2026-10-01T00:00:00+08:00", 90},
		{"2026-10-07T15:59:59Z", 90},
		{"2026-10-07T15:59:59.000000001Z", 100},
	} {
		q, err := cfg.QuoteAt(100, promoTime(t, tc.ts))
		if err != nil || q.PayableCNY != tc.want {
			t.Errorf("%s: %+v, %v", tc.ts, q, err)
		}
	}
	cfg.Promotion.StartAt = "2026-09-30T16:00:00Z"
	start, end := cfg.Promotion.WindowLabel()
	if start != "2026-10-01 00:00:00" || end != "2026-10-07 23:59:59" {
		t.Fatalf("window: %s / %s", start, end)
	}
	cfg.Promotion.StartAt = ""
	if !cfg.Promotion.ActiveAt(promoTime(t, "2020-01-01T00:00:00Z")) {
		t.Fatal("empty start should be unbounded")
	}
	cfg.Promotion.EndAt = ""
	if !cfg.Promotion.ActiveAt(promoTime(t, "2030-01-01T00:00:00Z")) {
		t.Fatal("empty end should be unbounded")
	}
	cfg.Promotion.Enabled = false
	q, err := cfg.QuoteAt(100, time.Now())
	if err != nil || q.PayableCNY != 100 || q.PromotionMode != "" {
		t.Fatalf("disabled quote: %+v %v", q, err)
	}
}

func TestPromotionValidationAndInvalidQuotes(t *testing.T) {
	for _, p := range []PromotionConfig{
		{Mode: "typo"}, {Mode: PromotionDiscount, Percent: 100}, {Mode: PromotionDiscount, Percent: 101},
		{Percent: -1}, {Percent: math.NaN()}, {Percent: math.Inf(1)},
		{StartAt: "2026-02-30 00:00:00"}, {EndAt: "not-a-date"},
		{StartAt: "2026-10-08 00:00:00", EndAt: "2026-10-07 23:59:59"},
	} {
		if p.validate() == nil {
			t.Errorf("accepted invalid config: %+v", p)
		}
	}
	cfg := Config{Promotion: PromotionConfig{Enabled: true, Mode: PromotionDiscount, Percent: 99.99}}
	for _, amount := range []float64{0.01, 0, -1, 0.001, 1e-12, math.NaN(), math.Inf(1), math.MaxFloat64} {
		if _, err := cfg.QuoteAt(amount, time.Now()); err == nil {
			t.Errorf("accepted amount %v", amount)
		}
	}
	cfg.Promotion = PromotionConfig{Enabled: true, Mode: PromotionGift, Percent: math.MaxFloat64}
	if _, err := cfg.QuoteAt(100, time.Now()); err == nil {
		t.Fatal("overflow gift accepted")
	}
}

func TestLoadRejectsInvalidPromotion(t *testing.T) {
	base := "telegram:\n  bot_token: test-token\ndatabase:\n  dsn: test-dsn\namounts: [100]\nsub2api:\n  webhook_url: http://example.test\npromotion:\n  enabled: true\n"
	for _, tc := range []struct {
		name, fields string
		invalid      bool
	}{
		{"discount", "  mode: discount\n  percent: 10\n  start_at: '2026-10-01 00:00:00'\n  end_at: '2026-10-07 23:59:59'\n", false},
		{"legacy", "  percent: 10\n", false},
		{"bad start", "  start_at: bad\n", true},
		{"bad end", "  end_at: bad\n", true},
		{"negative", "  percent: -1\n", true},
		{"free order", "  mode: discount\n  percent: 100\n", true},
		{"nan", "  percent: .nan\n", true},
		{"inf", "  percent: .inf\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(base+tc.fields), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if (err != nil) != tc.invalid {
				t.Fatalf("Load error=%v, invalid=%v", err, tc.invalid)
			}
			if err != nil && !strings.Contains(err.Error(), "promotion") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
