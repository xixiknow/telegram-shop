package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"telegram-shop/internal/i18n"
	"telegram-shop/internal/model"
	"telegram-shop/internal/payment"
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

	// 用户改用按钮操作，清除可能残留的「等待输入自定义金额」状态。
	// （cbCustomAmount 分支会在 switch 内重新设置该状态，顺序正确。）
	b.awaitingAmount.Delete(tgUserID)

	switch {
	case data == cbShop:
		b.editToAmountMenu(tgUserID, chatID, cb.Message.MessageID)

	case data == cbOrders:
		b.showOrders(tgUserID, chatID)

	case data == cbLang:
		b.showLanguageMenu(tgUserID, chatID)

	case data == cbCustomAmount:
		b.promptCustomAmount(tgUserID, chatID)

	case strings.HasPrefix(data, cbLangPrefix):
		b.setLanguage(tgUserID, chatID, i18n.Resolve(strings.TrimPrefix(data, cbLangPrefix)))

	case strings.HasPrefix(data, cbAmountPrefix):
		amountStr := strings.TrimPrefix(data, cbAmountPrefix)
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			return
		}
		b.editToMethodMenu(tgUserID, chatID, cb.Message.MessageID, amount)

	case strings.HasPrefix(data, cbNetPrefix):
		amountStr := strings.TrimPrefix(data, cbNetPrefix)
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			return
		}
		b.editToNetworkMenu(tgUserID, chatID, cb.Message.MessageID, amount)

	case strings.HasPrefix(data, cbMethodPrefix):
		b.handleMethodSelected(ctx, tgUserID, chatID, strings.TrimPrefix(data, cbMethodPrefix))

	case strings.HasPrefix(data, cbCheckPrefix):
		b.handleCheckOrder(ctx, tgUserID, chatID, strings.TrimPrefix(data, cbCheckPrefix))
	}
}

// handleMethodSelected 处理「method:easypay:50」格式的回调，创建订单。
func (b *Bot) handleMethodSelected(ctx context.Context, tgUserID, chatID int64, payload string) {
	lang := b.langOf(tgUserID)
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
		b.sendText(chatID, i18n.T(lang, i18n.MsgNeedBindFirst))
		return
	}

	// 下单冷却：防止刷第三方支付下单接口。
	if !b.orderCooldown.allow(tgUserID) {
		b.sendText(chatID, i18n.T(lang, i18n.MsgTooFast))
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
		// 已有待支付订单：提示用户先完成，并附该订单的刷新按钮。
		if errors.Is(err, service.ErrActivePendingOrder) && order != nil {
			text := fmt.Sprintf(i18n.T(lang, i18n.MsgActivePendingOrder), order.OrderNo)
			out := tgbotapi.NewMessage(chatID, text)
			out.ParseMode = tgbotapi.ModeMarkdown
			out.ReplyMarkup = orderActionKeyboard(lang, order.OrderNo, order.PayURL)
			b.send(out)
			return
		}
		slog.Error("create order failed", "error", err)
		b.sendText(chatID, i18n.T(lang, i18n.MsgCreateOrderFailed))
		return
	}

	b.sendOrderPaymentInfo(lang, chatID, order, result)
}

// handleCheckOrder 主动查询订单状态并反馈。
func (b *Bot) handleCheckOrder(_ context.Context, tgUserID, chatID int64, orderNo string) {
	lang := b.langOf(tgUserID)
	order, err := b.store.GetOrderByNo(orderNo)
	if err != nil {
		b.sendText(chatID, i18n.T(lang, i18n.MsgOrderNotFound))
		return
	}

	switch order.Status {
	case model.OrderStatusCompleted:
		b.sendMarkdown(chatID, fmt.Sprintf(
			i18n.T(lang, i18n.MsgOrderCompleted), order.OrderNo, order.Amount,
		))
	case model.OrderStatusPaid:
		b.sendText(chatID, i18n.T(lang, i18n.MsgOrderPaid))
	case model.OrderStatusPending:
		b.sendText(chatID, i18n.T(lang, i18n.MsgOrderPending))
	case model.OrderStatusExpired:
		b.sendText(chatID, i18n.T(lang, i18n.MsgOrderExpired))
	default:
		b.sendText(chatID, fmt.Sprintf(i18n.T(lang, i18n.MsgOrderStatus), order.Status))
	}
}

// --- 菜单展示 ---

// amountMenuText 返回充值额度菜单文案，活动期内附带赠送提示。
func (b *Bot) amountMenuText(lang i18n.Lang) string {
	gift := b.cfg.GiftAmountAt(100, time.Now())
	if gift > 0 {
		return fmt.Sprintf(
			i18n.T(lang, i18n.MsgGiftBanner),
			b.cfg.Promotion.Percent, 100+gift,
		)
	}
	return i18n.T(lang, i18n.MsgChooseAmount)
}

func (b *Bot) showAmountMenu(tgUserID, chatID int64) {
	lang := b.langOf(tgUserID)
	out := tgbotapi.NewMessage(chatID, b.amountMenuText(lang))
	out.ParseMode = tgbotapi.ModeMarkdown
	out.ReplyMarkup = amountKeyboard(lang, b.cfg.Amounts, b.cfg.GiftAmountAt, time.Now())
	b.send(out)
}

func (b *Bot) editToAmountMenu(tgUserID, chatID int64, messageID int) {
	lang := b.langOf(tgUserID)
	edit := tgbotapi.NewEditMessageText(chatID, messageID, b.amountMenuText(lang))
	edit.ParseMode = tgbotapi.ModeMarkdown
	kb := amountKeyboard(lang, b.cfg.Amounts, b.cfg.GiftAmountAt, time.Now())
	edit.ReplyMarkup = &kb
	b.send(edit)
}

func (b *Bot) editToMethodMenu(tgUserID, chatID int64, messageID int, amount float64) {
	lang := b.langOf(tgUserID)
	text := fmt.Sprintf(i18n.T(lang, i18n.MsgChooseMethod), amount)
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = tgbotapi.ModeMarkdown
	kb := methodKeyboard(lang, amount, b.cfg.Payment.EasyPay.Enabled, b.cfg.Payment.USDT.Enabled)
	edit.ReplyMarkup = &kb
	b.send(edit)
}

// showMethodMenu 以新消息形式展示支付方式菜单（用于自定义金额输入后，无可编辑的原消息）。
func (b *Bot) showMethodMenu(tgUserID, chatID int64, amount float64) {
	lang := b.langOf(tgUserID)
	text := fmt.Sprintf(i18n.T(lang, i18n.MsgChooseMethod), amount)
	out := tgbotapi.NewMessage(chatID, text)
	out.ParseMode = tgbotapi.ModeMarkdown
	out.ReplyMarkup = methodKeyboard(lang, amount, b.cfg.Payment.EasyPay.Enabled, b.cfg.Payment.USDT.Enabled)
	b.send(out)
}

// promptCustomAmount 进入「等待输入自定义金额」状态并提示用户输入。
func (b *Bot) promptCustomAmount(tgUserID, chatID int64) {
	lang := b.langOf(tgUserID)
	b.awaitingAmount.Store(tgUserID, true)
	b.sendText(chatID, fmt.Sprintf(i18n.T(lang, i18n.MsgCustomAmountPrompt), b.cfg.MinAmount, b.cfg.MaxAmount))
}

// handleAmountInput 解析用户输入的自定义金额，校验范围后进入支付方式选择。
func (b *Bot) handleAmountInput(msg *tgbotapi.Message) {
	tgUserID := msg.From.ID
	chatID := msg.Chat.ID
	lang := b.langOf(tgUserID)

	// 输入命令则取消等待状态，交回正常命令流程。
	if msg.IsCommand() {
		b.awaitingAmount.Delete(tgUserID)
		b.handleCommand(context.Background(), msg)
		return
	}

	amount, err := parseAmount(msg.Text)
	if err != nil || amount < b.cfg.MinAmount || amount > b.cfg.MaxAmount {
		// 保持等待状态，提示重输。
		b.sendText(chatID, fmt.Sprintf(i18n.T(lang, i18n.MsgAmountInvalid), b.cfg.MinAmount, b.cfg.MaxAmount))
		return
	}

	b.awaitingAmount.Delete(tgUserID)
	b.showMethodMenu(tgUserID, chatID, amount)
}

// parseAmount 解析金额文本：去空格，拒绝非法/超过两位小数的输入。
func parseAmount(s string) (float64, error) {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if v <= 0 {
		return 0, fmt.Errorf("non-positive amount")
	}
	// 拒绝超过两位小数（按分计）。
	cents := math.Round(v * 100)
	if math.Abs(v*100-cents) > 1e-6 {
		return 0, fmt.Errorf("more than 2 decimals")
	}
	return v, nil
}

func (b *Bot) editToNetworkMenu(tgUserID, chatID int64, messageID int, amount float64) {
	lang := b.langOf(tgUserID)
	text := fmt.Sprintf(i18n.T(lang, i18n.MsgChooseNetwork), amount)
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = tgbotapi.ModeMarkdown
	kb := networkKeyboard(lang, amount, b.cfg.Payment.USDT.TRC20Enabled, b.cfg.Payment.USDT.BEP20Enabled)
	edit.ReplyMarkup = &kb
	b.send(edit)
}

func (b *Bot) showOrders(tgUserID, chatID int64) {
	lang := b.langOf(tgUserID)
	orders, err := b.store.ListOrdersByUser(tgUserID, 10)
	if err != nil {
		b.sendText(chatID, i18n.T(lang, i18n.MsgQueryOrdersFailed))
		return
	}
	if len(orders) == 0 {
		b.sendText(chatID, i18n.T(lang, i18n.MsgNoOrders))
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.MsgRecentOrders))
	for _, o := range orders {
		sb.WriteString(fmt.Sprintf(
			i18n.T(lang, i18n.MsgOrderLine),
			o.OrderNo, o.Amount, paymentMethodLabel(lang, o.PaymentMethod), statusLabel(lang, o.Status),
		))
	}
	b.sendMarkdown(chatID, sb.String())
}

func (b *Bot) showAccount(tgUserID, chatID int64) {
	lang := b.langOf(tgUserID)
	user, err := b.store.GetUserByTelegramID(tgUserID)
	if err != nil || user.Sub2APIEmail == "" {
		b.sendText(chatID, i18n.T(lang, i18n.MsgAccountUnbound))
		return
	}
	b.sendMarkdown(chatID, fmt.Sprintf(
		i18n.T(lang, i18n.MsgAccount), user.Sub2APIEmail,
	))
}

// showBalance 查询并展示用户在 sub2api 的可用/冻结余额。
func (b *Bot) showBalance(ctx context.Context, tgUserID, chatID int64) {
	lang := b.langOf(tgUserID)
	user, err := b.store.GetUserByTelegramID(tgUserID)
	if err != nil || user.Sub2APIEmail == "" {
		b.sendText(chatID, i18n.T(lang, i18n.MsgNeedBindFirst))
		return
	}

	result, err := b.orderService.QueryBalance(ctx, user.Sub2APIEmail)
	if err != nil {
		slog.Error("query balance failed", "email", user.Sub2APIEmail, "error", err)
		b.sendText(chatID, i18n.T(lang, i18n.MsgBalanceQueryFailed))
		return
	}

	b.sendMarkdown(chatID, fmt.Sprintf(
		i18n.T(lang, i18n.MsgBalance), user.Sub2APIEmail, result.Balance, result.FrozenBalance,
	))
}

func (b *Bot) showHelp(tgUserID, chatID int64) {
	b.sendMarkdown(chatID, i18n.T(b.langOf(tgUserID), i18n.MsgHelp))
}

// sendOrderPaymentInfo 发送订单支付信息（USDT 与易支付均在 Telegram 内渲染二维码）。
func (b *Bot) sendOrderPaymentInfo(lang i18n.Lang, chatID int64, order *model.TGOrder, result *service.PaymentCreateResult) {
	if model.IsUSDTMethod(order.PaymentMethod) {
		b.sendUSDTPaymentInfo(lang, chatID, order, result)
		return
	}
	b.sendEasyPayPaymentInfo(lang, chatID, order, result)
}

// sendEasyPayPaymentInfo 渲染易支付：有二维码内容则在 Telegram 内发支付宝/微信收款码图片，
// 否则（跳转类型）回退为"去支付"按钮。
func (b *Bot) sendEasyPayPaymentInfo(lang i18n.Lang, chatID int64, order *model.TGOrder, result *service.PaymentCreateResult) {
	channelLabel := i18n.T(lang, i18n.ChannelAlipay)
	if result.Extra["channel"] == "wxpay" {
		channelLabel = i18n.T(lang, i18n.ChannelWxpay)
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.MsgOrderCreated))
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelOrderNo), order.OrderNo))
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelRechargeAmt), order.Amount))
	if order.GiftAmount > 0 {
		sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelGiftLine), order.GiftAmount, order.Amount+order.GiftAmount))
	}

	// 有二维码内容（qrcode 模式）→ Telegram 内发图，用户用对应 App 扫码付款。
	if result.QRCode != "" {
		sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelChannel), channelLabel))
		sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.MsgScanToPay), channelLabel))
		sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.MsgOrderValidNote), b.cfg.Payment.OrderTimeoutMinutes))
		caption := sb.String()
		kb := orderActionKeyboard(lang, order.OrderNo, "")

		png, err := payment.GenerateQRCode(result.QRCode, 320)
		if err != nil {
			slog.Error("generate easypay qrcode failed", "error", err)
			out := tgbotapi.NewMessage(chatID, caption)
			out.ParseMode = tgbotapi.ModeMarkdown
			out.ReplyMarkup = kb
			b.send(out)
			return
		}
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{Name: order.OrderNo + ".png", Bytes: png})
		photo.Caption = caption
		photo.ParseMode = tgbotapi.ModeMarkdown
		photo.ReplyMarkup = kb
		b.send(photo)
		return
	}

	// 跳转类型：给"去支付"按钮。
	sb.WriteString(i18n.T(lang, i18n.MsgClickToPay))
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.MsgOrderValidShort), b.cfg.Payment.OrderTimeoutMinutes))
	out := tgbotapi.NewMessage(chatID, sb.String())
	out.ParseMode = tgbotapi.ModeMarkdown
	out.ReplyMarkup = orderActionKeyboard(lang, order.OrderNo, order.PayURL)
	b.send(out)
}

// sendUSDTPaymentInfo 在 Telegram 内渲染 USDT 收款信息：地址二维码图片 + 金额 + 提示。
func (b *Bot) sendUSDTPaymentInfo(lang i18n.Lang, chatID int64, order *model.TGOrder, result *service.PaymentCreateResult) {
	wallet := result.Extra["wallet_address"]
	payAmount := result.Extra["pay_amount"]
	network := result.Extra["network"]

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.MsgOrderCreated))
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelOrderNo), order.OrderNo))
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelRechargeAmt), order.Amount))
	if order.GiftAmount > 0 {
		sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelGiftLine), order.GiftAmount, order.Amount+order.GiftAmount))
	}
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelPayNetwork), network))
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelPayAmount), payAmount))
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.LabelWallet), wallet))
	sb.WriteString(i18n.T(lang, i18n.MsgUSDTExactWarn))
	sb.WriteString(fmt.Sprintf(i18n.T(lang, i18n.MsgOrderValidNote), b.cfg.Payment.OrderTimeoutMinutes))
	caption := sb.String()

	kb := orderActionKeyboard(lang, order.OrderNo, "")

	// 生成收款地址二维码图片，作为带 caption 的图片消息发送。
	png, err := payment.GenerateQRCode(wallet, 320)
	if err != nil {
		slog.Error("generate qrcode failed", "error", err)
		// 退化为纯文本消息。
		out := tgbotapi.NewMessage(chatID, caption)
		out.ParseMode = tgbotapi.ModeMarkdown
		out.ReplyMarkup = kb
		b.send(out)
		return
	}

	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{Name: order.OrderNo + ".png", Bytes: png})
	photo.Caption = caption
	photo.ParseMode = tgbotapi.ModeMarkdown
	photo.ReplyMarkup = kb
	b.send(photo)
}
