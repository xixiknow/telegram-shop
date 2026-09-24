package bot

import (
	"log/slog"
	"net/mail"
	"strings"

	"telegram-shop/internal/i18n"
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

// langOf 返回指定 Telegram 用户的语言偏好（取持久化的 language_code，缺失时回退默认）。
func (b *Bot) langOf(tgUserID int64) i18n.Lang {
	user, err := b.store.GetUserByTelegramID(tgUserID)
	if err != nil {
		return i18n.Default
	}
	return i18n.Resolve(user.LanguageCode)
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

// statusLabel 返回订单状态的本地化标签。
func statusLabel(lang i18n.Lang, status string) string {
	switch status {
	case model.OrderStatusPending:
		return i18n.T(lang, i18n.StatusPending)
	case model.OrderStatusPaid:
		return i18n.T(lang, i18n.StatusPaid)
	case model.OrderStatusCompleted:
		return i18n.T(lang, i18n.StatusCompleted)
	case model.OrderStatusFailed:
		return i18n.T(lang, i18n.StatusFailed)
	case model.OrderStatusExpired:
		return i18n.T(lang, i18n.StatusExpired)
	default:
		return status
	}
}

// paymentMethodLabel 返回支付方式的本地化标签。
func paymentMethodLabel(lang i18n.Lang, method string) string {
	switch method {
	case model.PaymentMethodEasyPay:
		return i18n.T(lang, i18n.MethodEasyPay)
	case model.PaymentMethodUSDTTRC20:
		return i18n.T(lang, i18n.MethodUSDTTRC20)
	case model.PaymentMethodUSDTBEP20:
		return i18n.T(lang, i18n.MethodUSDTBEP20)
	default:
		return method
	}
}
