package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"telegram-shop/internal/model"
	"telegram-shop/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// --- 回调处理 ---

func (b *Bot) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	data := cb.Data
	chatID := cb.Message.Chat.ID
	tgUserID := cb.From.ID

	// 立即应答，消除按钮 loading 态
	b.answerCallback(cb.ID, "")

	switch {
	case data == cbShop:
		b.editToAmountMenu(chatID, cb.Message.MessageID)

	case data == cbOrders:
		b.showOrders(tgUserID, chatID)

	case strings.HasPrefix(data, cbAmountPrefix):
		amountStr := strings.TrimPrefix(data, cbAmountPrefix)
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			return
		}
		b.editToMethodMenu(chatID, cb.Message.MessageID, amount)

	case strings.HasPrefix(data, cbMethodPrefix):
		b.handleMethodSelected(ctx, tgUserID, chatID, strings.TrimPrefix(data, cbMethodPrefix))

	case strings.HasPrefix(data, cbCheckPrefix):
		b.handleCheckOrder(ctx, chatID, strings.TrimPrefix(data, cbCheckPrefix))
	}
}

// handleMethodSelected 处理「method:easypay:50」格式的回调，创建订单。
func (b *Bot) handleMethodSelected(ctx context.Context, tgUserID, chatID int64, payload string) {
	parts := strings.SplitN(payload, ":", 2)
	if len(parts) != 2 {
		return
	}
	method := parts[0]
	amount, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return
	}

	user, err := b.store.GetUserByTelegramID(tgUserID)
	if err != nil || user.Sub2APIEmail == "" {
		b.sendText(chatID, "⚠️ 请先绑定账号邮箱：/bind 您的邮箱")
		return
	}

	order, result, err := b.orderService.CreateOrder(ctx, service.CreateOrderInput{
		TelegramUserID:   tgUserID,
		TelegramUsername: user.Username,
		Sub2APIEmail:     user.Sub2APIEmail,
		AmountCNY:        amount,
		PaymentMethod:    method,
	})
	if err != nil {
		slog.Error("create order failed", "error", err)
		b.sendText(chatID, "❌ 创建订单失败，请稍后重试")
		return
	}

	b.sendOrderPaymentInfo(chatID, order, result)
}

// handleCheckOrder 主动查询订单状态并反馈。
func (b *Bot) handleCheckOrder(_ context.Context, chatID int64, orderNo string) {
	order, err := b.store.GetOrderByNo(orderNo)
	if err != nil {
		b.sendText(chatID, "未找到该订单")
		return
	}

	switch order.Status {
	case model.OrderStatusCompleted:
		b.sendMarkdown(chatID, fmt.Sprintf(
			"✅ 订单 `%s` 已完成，*%.2f 元* 已到账。", order.OrderNo, order.Amount,
		))
	case model.OrderStatusPaid:
		b.sendText(chatID, "⏳ 已收到支付，正在为您充值，请稍候…")
	case model.OrderStatusPending:
		b.sendText(chatID, "⏳ 订单待支付。完成支付后通常会自动到账；若已支付请稍候。")
	case model.OrderStatusExpired:
		b.sendText(chatID, "⌛ 订单已过期，请重新下单。")
	default:
		b.sendText(chatID, "订单状态："+order.Status)
	}
}

// --- 菜单展示 ---

func (b *Bot) showAmountMenu(chatID int64) {
	out := tgbotapi.NewMessage(chatID, "请选择充值额度：")
	out.ReplyMarkup = amountKeyboard(b.cfg.Amounts)
	b.send(out)
}

func (b *Bot) editToAmountMenu(chatID int64, messageID int) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, "请选择充值额度：")
	kb := amountKeyboard(b.cfg.Amounts)
	edit.ReplyMarkup = &kb
	b.send(edit)
}

func (b *Bot) editToMethodMenu(chatID int64, messageID int, amount float64) {
	text := fmt.Sprintf("充值额度：*%g 元*\n\n请选择支付方式：", amount)
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = tgbotapi.ModeMarkdown
	kb := methodKeyboard(amount, b.cfg.Payment.EasyPay.Enabled, b.cfg.Payment.USDT.Enabled)
	edit.ReplyMarkup = &kb
	b.send(edit)
}

func (b *Bot) showOrders(tgUserID, chatID int64) {
	orders, err := b.store.ListOrdersByUser(tgUserID, 10)
	if err != nil {
		b.sendText(chatID, "查询订单失败，请稍后重试")
		return
	}
	if len(orders) == 0 {
		b.sendText(chatID, "您还没有订单。点击 🛒 充值 开始购买。")
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 *最近订单*\n\n")
	for _, o := range orders {
		sb.WriteString(fmt.Sprintf(
			"`%s`\n额度：%.2f 元 | %s | %s\n\n",
			o.OrderNo, o.Amount, paymentMethodLabel(o.PaymentMethod), statusLabel(o.Status),
		))
	}
	b.sendMarkdown(chatID, sb.String())
}

func (b *Bot) showAccount(tgUserID, chatID int64) {
	user, err := b.store.GetUserByTelegramID(tgUserID)
	if err != nil || user.Sub2APIEmail == "" {
		b.sendText(chatID, "您还未绑定账号。发送 /bind 您的邮箱 进行绑定。")
		return
	}
	b.sendMarkdown(chatID, fmt.Sprintf(
		"👤 *我的账号*\n\n绑定邮箱：`%s`\n\n如需更换，发送 /bind 新邮箱",
		user.Sub2APIEmail,
	))
}

func (b *Bot) showHelp(chatID int64) {
	help := "*使用帮助*\n\n" +
		"/start - 开始使用\n" +
		"/bind 邮箱 - 绑定充值账号\n" +
		"/shop - 选择充值额度\n" +
		"/orders - 查看我的订单\n" +
		"/account - 查看绑定账号\n\n" +
		"流程：绑定邮箱 → 选择额度 → 选择支付方式 → 完成支付 → 自动到账"
	b.sendMarkdown(chatID, help)
}

// sendOrderPaymentInfo 发送订单支付信息（易支付给链接，USDT 给地址+金额）。
func (b *Bot) sendOrderPaymentInfo(chatID int64, order *model.TGOrder, result *service.PaymentCreateResult) {
	var sb strings.Builder
	sb.WriteString("🧾 *订单已创建*\n\n")
	sb.WriteString(fmt.Sprintf("订单号：`%s`\n", order.OrderNo))
	sb.WriteString(fmt.Sprintf("充值额度：*%.2f 元*\n", order.Amount))

	switch order.PaymentMethod {
	case model.PaymentMethodEasyPay:
		sb.WriteString("\n请点击下方按钮完成支付 👇")
	case model.PaymentMethodUSDT:
		sb.WriteString(fmt.Sprintf("\n支付网络：*%s*\n", result.Extra["network"]))
		sb.WriteString(fmt.Sprintf("应付金额：*%s USDT*\n", result.Extra["pay_amount"]))
		sb.WriteString(fmt.Sprintf("收款地址：\n`%s`\n", result.Extra["wallet_address"]))
		sb.WriteString("\n⚠️ 请务必按*精确金额*转账，金额用于自动匹配订单。")
	}
	sb.WriteString(fmt.Sprintf("\n\n订单 %d 分钟内有效。", b.cfg.Payment.OrderTimeoutMinutes))

	out := tgbotapi.NewMessage(chatID, sb.String())
	out.ParseMode = tgbotapi.ModeMarkdown
	kb := orderActionKeyboard(order.OrderNo, order.PayURL)
	out.ReplyMarkup = kb
	b.send(out)
}
