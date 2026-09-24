package bot

import (
	"strings"
	"testing"
	"time"

	"telegram-shop/internal/config"
	"telegram-shop/internal/i18n"
	"telegram-shop/internal/model"
)

func TestPromotionMenus(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-02T00:00:00+08:00")
	for _, lang := range []i18n.Lang{i18n.ZH, i18n.EN} {
		for _, mode := range []string{config.PromotionDiscount, config.PromotionGift} {
			t.Run(string(lang)+"/"+mode, func(t *testing.T) {
				cfg := &config.Config{Amounts: []float64{100, 33.33, 50}, Promotion: config.PromotionConfig{
					Enabled: true, Mode: mode, Percent: 10, StartAt: "2026-10-01 00:00:00", EndAt: "2026-10-07 23:59:59"}}
				b := &Bot{cfg: cfg}
				banner := b.amountMenuText(lang, now)
				for _, want := range []string{"10%", "2026-10-01 00:00:00", "2026-10-07 23:59:59"} {
					if !strings.Contains(banner, want) {
						t.Errorf("missing %q in %s", want, banner)
					}
				}
				kb := amountKeyboard(lang, cfg.Amounts, cfg.QuoteAt, now)
				button := kb.InlineKeyboard[0][0]
				if button.CallbackData == nil || *button.CallbackData != "amount:100" {
					t.Fatalf("callback must keep face value: %+v", button)
				}
				want := "90.00"
				if mode == config.PromotionGift {
					want = "+10"
				}
				if !strings.Contains(button.Text, want) {
					t.Fatalf("unexpected label: %s", button.Text)
				}
				for _, network := range []bool{false, true} {
					text, err := b.paymentMenuText(lang, 33.33, now, network)
					if err != nil {
						t.Fatal(err)
					}
					want = "30.00"
					if mode == config.PromotionGift {
						want = "36.66"
					}
					if !strings.Contains(text, "33.33") || !strings.Contains(text, want) || strings.Contains(text, "%!") {
						t.Fatalf("bad custom quote: %s", text)
					}
				}
				cfg.Promotion.Enabled = false
				if b.amountMenuText(lang, now) != i18n.T(lang, i18n.MsgChooseAmount) {
					t.Fatal("disabled banner still shows promo")
				}
			})
		}
	}
}

func TestOrderAmountTextUsesSnapshot(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.ZH, i18n.EN} {
		for _, order := range []model.TGOrder{
			{Amount: 100, DiscountAmount: 10, PromotionMode: config.PromotionDiscount, PromotionPercent: 10, PayAmount: 12.5, PayCurrency: "USDT"},
			{Amount: 100, GiftAmount: 10}, // 旧订单缺少模式快照，仍显示赠额。
		} {
			text := orderAmountText(lang, &order)
			want := "90.00"
			if order.GiftAmount > 0 {
				want = "110.00"
			}
			if !strings.Contains(text, want) || !strings.Contains(text, "100.00") || strings.Contains(text, "%!") {
				t.Fatalf("bad snapshot display: %s", text)
			}
		}
	}
}

func TestInvalidAmountsAndUnquotableMenu(t *testing.T) {
	for _, s := range []string{"NaN", "Inf", "-Inf", "0", "-1", "1.001", "1e300", "1e-12"} {
		if _, err := parseAmount(s); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
	cfg := &config.Config{Promotion: config.PromotionConfig{Enabled: true, Mode: config.PromotionDiscount, Percent: 99}}
	kb := amountKeyboard(i18n.ZH, []float64{1, 0.01}, cfg.QuoteAt, time.Now())
	if len(kb.InlineKeyboard) != 2 || len(kb.InlineKeyboard[0]) != 1 {
		t.Fatalf("lost last valid row: %+v", kb)
	}
	if *kb.InlineKeyboard[1][0].CallbackData != cbCustomAmount {
		t.Fatal("missing custom amount button")
	}
}
