package model

import "testing"

func TestOrderAmountsCompatibility(t *testing.T) {
	for _, tc := range []struct {
		order       TGOrder
		pay, credit float64
	}{
		{TGOrder{Amount: 100}, 100, 100},
		{TGOrder{Amount: 100, GiftAmount: 10}, 100, 110},
		{TGOrder{Amount: 100, DiscountAmount: 10, PayAmount: 12.5, PayCurrency: "USDT"}, 90, 100},
		{TGOrder{Amount: 33.33, DiscountAmount: 4.17}, 29.16, 33.33},
		{TGOrder{Amount: 33.33, GiftAmount: 4.17}, 33.33, 37.5},
	} {
		if tc.order.PayableCNY() != tc.pay || tc.order.CreditAmount() != tc.credit {
			t.Fatalf("incorrect amounts for %+v", tc.order)
		}
	}
}
