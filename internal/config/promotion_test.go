package config

import (
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
		tm, _ := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
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
