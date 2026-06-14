package bot

import (
	"log/slog"
	"net/mail"
	"strings"

	"telegram-shop/internal/model"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// upsertUser 同步 Telegram 用户基础资料。
func (b *Bot) upsertUser(from *tgbotapi.User) {
	if from == nil {
		return
	}
	_ = b.store.UpsertUser(&model.TGUser{
		TelegramUserID: from.ID,
		Username:       from.UserName,
		FirstName:      from.FirstName,
		LastName:       from.LastName,
		LanguageCode:   from.LanguageCode,
	})
}

// send 统一发送 Chattable，记录错误。
func (b *Bot) send(c tgbotapi.Chattable) {
	if _, err := b.api.Send(c); err != nil {
		slog.Error("telegram send failed", "error", err)
	}
}

func (b *Bot) sendText(chatID int64, text string) {
	b.send(tgbotapi.NewMessage(chatID, text))
}

func (b *Bot) sendMarkdown(chatID int64, text string) {
	out := tgbotapi.NewMessage(chatID, text)
	out.ParseMode = tgbotapi.ModeMarkdown
	b.send(out)
}

func (b *Bot) answerCallback(callbackID, text string) {
	if _, err := b.api.Request(tgbotapi.NewCallback(callbackID, text)); err != nil {
		slog.Debug("answer callback failed", "error", err)
	}
}

// isValidEmail 校验邮箱格式。
func isValidEmail(email string) bool {
	email = strings.TrimSpace(email)
	if email == "" || len(email) > 255 {
		return false
	}
	_, err := mail.ParseAddress(email)
	return err == nil && !strings.ContainsAny(email, " ")
}

// statusLabel 返回订单状态的中文标签。
func statusLabel(status string) string {
	switch status {
	case model.OrderStatusPending:
		return "⏳ 待支付"
	case model.OrderStatusPaid:
		return "💰 充值中"
	case model.OrderStatusCompleted:
		return "✅ 已完成"
	case model.OrderStatusFailed:
		return "❌ 失败"
	case model.OrderStatusExpired:
		return "⌛ 已过期"
	default:
		return status
	}
}

// paymentMethodLabel 返回支付方式的中文标签。
func paymentMethodLabel(method string) string {
	switch method {
	case model.PaymentMethodEasyPay:
		return "易支付"
	case model.PaymentMethodUSDT:
		return "USDT"
	default:
		return method
	}
}
