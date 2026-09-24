package bot

import (
	"fmt"
	"strings"
	"time"

	"telegram-shop/internal/config"
	"telegram-shop/internal/i18n"
	"telegram-shop/internal/model"
)

func (b *Bot) amountMenuText(lang i18n.Lang, now time.Time) string {
	p := b.cfg.Promotion
	if !p.ActiveAt(now) {
		return i18n.T(lang, i18n.MsgChooseAmount)
	}
	var sb strings.Builder
	q, err := b.cfg.QuoteAt(100, now)
	if err == nil {
		if q.PromotionMode == config.PromotionDiscount {
			sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.MsgDiscountBanner), p.Percent, q.PayableCNY))
		} else {
			sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.MsgGiftBanner), p.Percent, q.CreditAmount))
		}
	}
	start, end := p.WindowLabel()
	if start != "" {
		sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelPromoStart), start))
	}
	if end != "" {
		sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelPromoEnd), end))
	}
	sb.WriteString(i18n.T(lang, i18n.MsgQuoteTiming))
	sb.WriteString("\n" + i18n.T(lang, i18n.MsgChooseAmount))
	return sb.String()
}

func (b *Bot) paymentMenuText(lang i18n.Lang, amount float64, now time.Time, network bool) (string, error) {
	q, err := b.cfg.QuoteAt(amount, now)
	if err != nil {
		return "", err
	}
	key := i18n.MsgChooseMethod
	if network {
		key = i18n.MsgChooseNetwork
	}
	return quoteAmountText(lang, q) + i18n.T(lang, i18n.MsgQuoteTiming) + "\n" + i18n.T(lang, key), nil
}

func orderAmountText(lang i18n.Lang, o *model.TGOrder) string {
	return quoteAmountText(lang, config.RechargeQuote{
		Amount: o.Amount, PayableCNY: o.PayableCNY(), CreditAmount: o.CreditAmount(),
		GiftAmount: o.GiftAmount, DiscountAmount: o.DiscountAmount,
		PromotionMode: o.PromotionMode, PromotionPercent: o.PromotionPercent,
	})
}

func quoteAmountText(lang i18n.Lang, q config.RechargeQuote) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelRechargeAmt), q.Amount))
	if q.PromotionMode == config.PromotionDiscount {
		sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelDiscountLine), q.PromotionPercent, q.DiscountAmount))
	}
	if q.GiftAmount > 0 {
		sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelGiftLine), q.GiftAmount))
	}
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelPayCNY), q.PayableCNY))
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelCreditAmount), q.CreditAmount))
	return sb.String()
}
