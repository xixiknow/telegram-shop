package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"telegram-shop/internal/config"
	"telegram-shop/internal/i18n"
	"telegram-shop/internal/model"
	"telegram-shop/internal/service"
	"telegram-shop/internal/store"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot 封装 Telegram Bot 与业务交互。
type Bot struct {
	api          *tgbotapi.BotAPI
	cfg          *config.Config
	store        *store.Store
	orderService *service.OrderService

	// 等待用户输入邮箱的会话状态：telegramUserID -> true
	awaitingEmail sync.Map

	// 等待用户输入自定义金额的会话状态：telegramUserID -> true
	awaitingAmount sync.Map

	// 限流：防恶意高频请求
	limiter       *rateLimiter // 通用：每用户消息/回调操作频率
	orderCooldown *cooldown    // 创建订单的最小间隔
}

// New 创建 Bot。
func New(cfg *config.Config, st *store.Store, orderService *service.OrderService) (*Bot, error) {
	// 自定义 HTTP client：设置超时，避免 Telegram 边缘节点偶发的慢响应
	// （504 Gateway Timeout）导致请求无限期阻塞、堆积，进而拖慢按钮回调与发图。
	// 超时须大于 long-polling 的 30s（见 StartPolling 的 u.Timeout），故取 60s。
	httpClient := &http.Client{Timeout: 60 * time.Second}
	api, err := tgbotapi.NewBotAPIWithClient(cfg.Telegram.BotToken, tgbotapi.APIEndpoint, httpClient)
	if err != nil {
		return nil, fmt.Errorf("init telegram bot: %w", err)
	}
	slog.Info("telegram bot authorized", "username", api.Self.UserName)
	return &Bot{
		api:           api,
		cfg:           cfg,
		store:         st,
		orderService:  orderService,
		limiter:       newRateLimiter(10*time.Second, 10), // 每用户 10 秒内最多 10 次操作
		orderCooldown: newCooldown(5 * time.Second),       // 创建订单最快 5 秒一次
	}, nil
}

// API 暴露底层 BotAPI（webhook 模式需要）。
func (b *Bot) API() *tgbotapi.BotAPI { return b.api }

// StartPolling 以 long polling 模式启动（开发/小规模推荐），阻塞运行。
func (b *Bot) StartPolling(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := b.api.GetUpdatesChan(u)

	slog.Info("bot polling started")
	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return
		case update := <-updates:
			go b.handleUpdate(ctx, update)
		}
	}
}

// HandleUpdate 处理单个 update（webhook 模式调用）。
func (b *Bot) HandleUpdate(ctx context.Context, update tgbotapi.Update) {
	b.handleUpdate(ctx, update)
}

func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in handleUpdate", "recover", r)
		}
	}()

	switch {
	case update.CallbackQuery != nil:
		// 限流：超过频率上限直接丢弃，但仍应答回调以消除按钮 loading 态。
		if !b.limiter.allow(update.CallbackQuery.From.ID) {
			b.answerCallback(update.CallbackQuery.ID, "")
			slog.Warn("rate limited callback", "tgUserID", update.CallbackQuery.From.ID)
			return
		}
		b.handleCallback(ctx, update.CallbackQuery)
	case update.Message != nil:
		// 限流：超过频率上限静默丢弃，不回复（避免提示本身被刷）。
		if update.Message.From != nil && !b.limiter.allow(update.Message.From.ID) {
			slog.Warn("rate limited message", "tgUserID", update.Message.From.ID)
			return
		}
		b.handleMessage(ctx, update.Message)
	}
}

// NotifyOrderCompleted 在订单充值完成后通知用户（由 webhook handler 触发）。
func (b *Bot) NotifyOrderCompleted(orderNo string) {
	order, err := b.store.GetOrderByNo(orderNo)
	if err != nil {
		slog.Error("notify: lookup order failed", "orderNo", orderNo, "error", err)
		return
	}
	text := fmt.Sprintf(
		i18n.T(b.langOf(order.TelegramUserID), i18n.MsgPaySuccess),
		order.OrderNo, order.Amount, order.Sub2APIEmail,
	)
	b.sendMarkdown(order.TelegramUserID, text)
}

// --- 消息处理 ---

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	// 同步用户基础信息
	b.upsertUser(msg.From)

	// 优先处理「等待输入邮箱」状态
	if _, waiting := b.awaitingEmail.Load(msg.From.ID); waiting {
		b.handleEmailInput(msg)
		return
	}

	// 处理「等待输入自定义金额」状态
	if _, waiting := b.awaitingAmount.Load(msg.From.ID); waiting {
		b.handleAmountInput(msg)
		return
	}

	text := strings.TrimSpace(msg.Text)

	// 命令
	if msg.IsCommand() {
		b.handleCommand(ctx, msg)
		return
	}

	// reply keyboard 文本按钮（匹配任意受支持语言的标签）
	switch {
	case matchesBtn(text, i18n.BtnShop):
		b.showAmountMenu(msg.From.ID, msg.Chat.ID)
	case matchesBtn(text, i18n.BtnOrders):
		b.showOrders(msg.From.ID, msg.Chat.ID)
	case matchesBtn(text, i18n.BtnBalance):
		b.showBalance(ctx, msg.From.ID, msg.Chat.ID)
	case matchesBtn(text, i18n.BtnAccount):
		b.showAccount(msg.From.ID, msg.Chat.ID)
	case matchesBtn(text, i18n.BtnHelp):
		b.showHelp(msg.From.ID, msg.Chat.ID)
	case matchesBtn(text, i18n.BtnLanguage):
		b.showLanguageMenu(msg.From.ID, msg.Chat.ID)
	default:
		b.sendText(msg.Chat.ID, i18n.T(b.langOf(msg.From.ID), i18n.MsgUseMenu))
	}
}

// matchesBtn 判断文本是否等于某按钮在任一受支持语言下的标签。
func matchesBtn(text, key string) bool {
	return text == i18n.T(i18n.ZH, key) || text == i18n.T(i18n.EN, key)
}

func (b *Bot) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start":
		b.handleStart(msg)
	case "bind":
		b.handleBindCommand(msg)
	case "shop", "recharge":
		b.showAmountMenu(msg.From.ID, msg.Chat.ID)
	case "orders":
		b.showOrders(msg.From.ID, msg.Chat.ID)
	case "balance":
		b.showBalance(ctx, msg.From.ID, msg.Chat.ID)
	case "account":
		b.showAccount(msg.From.ID, msg.Chat.ID)
	case "help":
		b.showHelp(msg.From.ID, msg.Chat.ID)
	case "lang", "language":
		b.showLanguageMenu(msg.From.ID, msg.Chat.ID)
	default:
		b.sendText(msg.Chat.ID, i18n.T(b.langOf(msg.From.ID), i18n.MsgUnknownCmd))
	}
	_ = ctx
}

func (b *Bot) handleStart(msg *tgbotapi.Message) {
	lang := b.langOf(msg.From.ID)
	user, err := b.store.GetUserByTelegramID(msg.From.ID)
	bound := err == nil && user.Sub2APIEmail != ""

	welcome := i18n.T(lang, i18n.MsgWelcomeTitle)
	if bound {
		welcome += fmt.Sprintf(i18n.T(lang, i18n.MsgWelcomeBound), user.Sub2APIEmail)
	} else {
		welcome += i18n.T(lang, i18n.MsgWelcomeUnbound)
	}

	out := tgbotapi.NewMessage(msg.Chat.ID, welcome)
	out.ParseMode = tgbotapi.ModeMarkdown
	out.ReplyMarkup = mainMenuKeyboard(lang)
	b.send(out)
}

// showLanguageMenu 展示语言选择 inline 键盘。
func (b *Bot) showLanguageMenu(tgUserID, chatID int64) {
	out := tgbotapi.NewMessage(chatID, i18n.T(b.langOf(tgUserID), i18n.MsgChooseLanguage))
	out.ReplyMarkup = languageKeyboard()
	b.send(out)
}

// setLanguage 持久化用户语言选择，并以新语言刷新主菜单。
func (b *Bot) setLanguage(tgUserID, chatID int64, lang i18n.Lang) {
	if err := b.store.SetLanguage(tgUserID, string(lang)); err != nil {
		slog.Error("set language failed", "error", err)
	}
	out := tgbotapi.NewMessage(chatID, i18n.T(lang, i18n.MsgLanguageSet))
	out.ReplyMarkup = mainMenuKeyboard(lang)
	b.send(out)
}

func (b *Bot) handleBindCommand(msg *tgbotapi.Message) {
	email := strings.TrimSpace(msg.CommandArguments())
	if email == "" {
		// 进入等待输入状态
		b.awaitingEmail.Store(msg.From.ID, true)
		b.sendText(msg.Chat.ID, i18n.T(b.langOf(msg.From.ID), i18n.MsgBindPrompt))
		return
	}
	b.bindEmail(msg.From.ID, msg.Chat.ID, email)
}

func (b *Bot) handleEmailInput(msg *tgbotapi.Message) {
	b.awaitingEmail.Delete(msg.From.ID)
	email := strings.TrimSpace(msg.Text)
	b.bindEmail(msg.From.ID, msg.Chat.ID, email)
}

func (b *Bot) bindEmail(tgUserID, chatID int64, email string) {
	lang := b.langOf(tgUserID)
	if !isValidEmail(email) {
		b.sendText(chatID, i18n.T(lang, i18n.MsgBindInvalid))
		return
	}
	if err := b.store.BindEmail(tgUserID, email); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// 用户记录尚不存在则补建
			_ = b.store.UpsertUser(&model.TGUser{TelegramUserID: tgUserID, Sub2APIEmail: email})
		} else {
			slog.Error("bind email failed", "error", err)
			b.sendText(chatID, i18n.T(lang, i18n.MsgBindFailed))
			return
		}
	}
	b.sendMarkdown(chatID, fmt.Sprintf(i18n.T(lang, i18n.MsgBindSuccess), email))
}
