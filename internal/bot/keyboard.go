// Package bot 实现 Telegram Bot 的菜单交互。
package bot

import (
	"fmt"
	"time"

	"telegram-shop/internal/config"
	"telegram-shop/internal/i18n"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// callback data 前缀（用于 inline keyboard 回调路由）。
const (
	cbAmountPrefix = "amount:" // amount:50 —— 选择充值额度
	cbMethodPrefix = "method:" // method:easypay:50 —— 选择支付方式
	cbNetPrefix    = "net:"    // net:50 —— 选择 USDT 链（展开 TRC20/BEP20）
	cbShop         = "shop"    // 返回额度选择
	cbOrders       = "orders"  // 我的订单
	cbCheckPrefix  = "check:"  // check:tgshop_xxx —— 主动查询订单
	cbLang         = "lang"    // 展开语言选择
	cbLangPrefix   = "lang:"   // lang:en —— 设置语言
	cbCustomAmount = "custom"  // 自定义金额 —— 进入等待输入状态
)

// mainMenuKeyboard 返回主菜单（reply keyboard，常驻底部）。
func mainMenuKeyboard(lang i18n.Lang) tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(i18n.T(lang, i18n.BtnShop)),
			tgbotapi.NewKeyboardButton(i18n.T(lang, i18n.BtnOrders)),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(i18n.T(lang, i18n.BtnBalance)),
			tgbotapi.NewKeyboardButton(i18n.T(lang, i18n.BtnAccount)),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(i18n.T(lang, i18n.BtnHelp)),
			tgbotapi.NewKeyboardButton(i18n.T(lang, i18n.BtnLanguage)),
		),
	)
	kb.ResizeKeyboard = true
	return kb
}

// languageKeyboard 返回语言选择 inline 键盘。
func languageKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				i18n.T(i18n.ZH, i18n.BtnLangZH), cbLangPrefix+string(i18n.ZH),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				i18n.T(i18n.EN, i18n.BtnLangEN), cbLangPrefix+string(i18n.EN),
			),
		),
	)
}

// amountKeyboard 根据配置的额度生成 inline 键盘，每行 2 个按钮。
// 报价与订单共用；callback 始终携带原面额，不信任客户端的优惠金额。
func amountKeyboard(lang i18n.Lang, amounts []float64, quoteFn func(float64, time.Time) (config.RechargeQuote, error), now time.Time) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton

	for _, amount := range amounts {
		label := fmt.Sprintf(i18n.T(lang, i18n.LabelAmountUnit), amount)
		if quoteFn != nil {
			q, err := quoteFn(amount, now)
			if err != nil { // 例如极小面额折后不足一分，不展示不可购买的档位。
				continue
			}
			if q.PromotionMode == config.PromotionDiscount {
				label = fmt.Sprintf(i18n.T(lang, i18n.LabelAmountDiscount), amount, q.PayableCNY)
			} else if q.GiftAmount > 0 {
				label = fmt.Sprintf(i18n.T(lang, i18n.LabelAmountGift), amount, q.GiftAmount)
			}
		}
		data := fmt.Sprintf("%s%g", cbAmountPrefix, amount)
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(label, data))

		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	// 自定义金额按钮，独占一行。
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, i18n.BtnCustomAmount), cbCustomAmount),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// methodKeyboard 返回支付方式选择键盘。
func methodKeyboard(lang i18n.Lang, amount float64, easypayEnabled, usdtEnabled bool) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	if easypayEnabled {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				i18n.T(lang, i18n.BtnEasyPay),
				fmt.Sprintf("%seasypay:%g", cbMethodPrefix, amount),
			),
		))
	}
	if usdtEnabled {
		// USDT 先选链：进入网络选择子菜单。
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				i18n.T(lang, i18n.BtnUSDT),
				fmt.Sprintf("%s%g", cbNetPrefix, amount),
			),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, i18n.BtnBack), cbShop),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// networkKeyboard 返回 USDT 链选择键盘（按配置启用项显示）。
func networkKeyboard(lang i18n.Lang, amount float64, trc20Enabled, bep20Enabled bool) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	if trc20Enabled {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				i18n.T(lang, i18n.BtnTRC20),
				fmt.Sprintf("%susdt_trc20:%g", cbMethodPrefix, amount),
			),
		))
	}
	if bep20Enabled {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				i18n.T(lang, i18n.BtnBEP20),
				fmt.Sprintf("%susdt_bep20:%g", cbMethodPrefix, amount),
			),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, i18n.BtnBack), fmt.Sprintf("%s%g", cbAmountPrefix, amount)),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// orderActionKeyboard 返回订单操作键盘（含支付链接与刷新）。
func orderActionKeyboard(lang i18n.Lang, orderNo, payURL string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	if payURL != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(i18n.T(lang, i18n.BtnPay), payURL),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, i18n.BtnRefresh), cbCheckPrefix+orderNo),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
