// Package bot 实现 Telegram Bot 的菜单交互。
package bot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// callback data 前缀（用于 inline keyboard 回调路由）。
const (
	cbAmountPrefix  = "amount:"  // amount:50 —— 选择充值额度
	cbMethodPrefix  = "method:"  // method:easypay:50 —— 选择支付方式
	cbShop          = "shop"     // 返回额度选择
	cbOrders        = "orders"   // 我的订单
	cbCheckPrefix   = "check:"   // check:tgshop_xxx —— 主动查询订单
)

// mainMenuKeyboard 返回主菜单（reply keyboard，常驻底部）。
func mainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🛒 充值"),
			tgbotapi.NewKeyboardButton("📋 我的订单"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("👤 我的账号"),
			tgbotapi.NewKeyboardButton("❓ 帮助"),
		),
	)
	kb.ResizeKeyboard = true
	return kb
}

// amountKeyboard 根据配置的额度生成 inline 键盘，每行 2 个按钮。
func amountKeyboard(amounts []float64) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton

	for i, amount := range amounts {
		label := fmt.Sprintf("💰 %g 元", amount)
		data := fmt.Sprintf("%s%g", cbAmountPrefix, amount)
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(label, data))

		if len(row) == 2 || i == len(amounts)-1 {
			rows = append(rows, row)
			row = nil
		}
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// methodKeyboard 返回支付方式选择键盘。
func methodKeyboard(amount float64, easypayEnabled, usdtEnabled bool) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	if easypayEnabled {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				"💳 易支付（支付宝/微信）",
				fmt.Sprintf("%seasypay:%g", cbMethodPrefix, amount),
			),
		))
	}
	if usdtEnabled {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				"₮ USDT 支付",
				fmt.Sprintf("%susdt:%g", cbMethodPrefix, amount),
			),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⬅️ 返回", cbShop),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// orderActionKeyboard 返回订单操作键盘（含支付链接与刷新）。
func orderActionKeyboard(orderNo, payURL string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	if payURL != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🔗 去支付", payURL),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 我已支付/刷新状态", cbCheckPrefix+orderNo),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
