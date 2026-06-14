package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"telegram-shop/internal/config"
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
}

// New 创建 Bot。
func New(cfg *config.Config, st *store.Store, orderService *service.OrderService) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Telegram.BotToken)
	if err != nil {
		return nil, fmt.Errorf("init telegram bot: %w", err)
	}
	slog.Info("telegram bot authorized", "username", api.Self.UserName)
	return &Bot{api: api, cfg: cfg, store: st, orderService: orderService}, nil
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
		b.handleCallback(ctx, update.CallbackQuery)
	case update.Message != nil:
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
		"✅ *支付成功！*\n\n订单号：`%s`\n充值额度：*%.2f 元*\n账号：`%s`\n\n余额已到账，感谢您的购买！",
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

	text := strings.TrimSpace(msg.Text)

	// 命令
	if msg.IsCommand() {
		b.handleCommand(ctx, msg)
		return
	}

	// reply keyboard 文本按钮
	switch text {
	case "🛒 充值":
		b.showAmountMenu(msg.Chat.ID)
	case "📋 我的订单":
		b.showOrders(msg.From.ID, msg.Chat.ID)
	case "👤 我的账号":
		b.showAccount(msg.From.ID, msg.Chat.ID)
	case "❓ 帮助":
		b.showHelp(msg.Chat.ID)
	default:
		b.sendText(msg.Chat.ID, "请使用下方菜单操作，或发送 /help 查看帮助。")
	}
}

func (b *Bot) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start":
		b.handleStart(msg)
	case "bind":
		b.handleBindCommand(msg)
	case "shop", "recharge":
		b.showAmountMenu(msg.Chat.ID)
	case "orders":
		b.showOrders(msg.From.ID, msg.Chat.ID)
	case "account":
		b.showAccount(msg.From.ID, msg.Chat.ID)
	case "help":
		b.showHelp(msg.Chat.ID)
	default:
		b.sendText(msg.Chat.ID, "未知命令，发送 /help 查看可用命令。")
	}
	_ = ctx
}

func (b *Bot) handleStart(msg *tgbotapi.Message) {
	user, err := b.store.GetUserByTelegramID(msg.From.ID)
	bound := err == nil && user.Sub2APIEmail != ""

	welcome := "👋 *欢迎使用充值机器人！*\n\n"
	if bound {
		welcome += fmt.Sprintf("当前绑定账号：`%s`\n\n点击下方 *🛒 充值* 开始购买。", user.Sub2APIEmail)
	} else {
		welcome += "首次使用，请先绑定您的账号邮箱：\n发送 `/bind 您的邮箱` 完成绑定。\n\n例如：`/bind user@example.com`"
	}

	out := tgbotapi.NewMessage(msg.Chat.ID, welcome)
	out.ParseMode = tgbotapi.ModeMarkdown
	out.ReplyMarkup = mainMenuKeyboard()
	b.send(out)
}

func (b *Bot) handleBindCommand(msg *tgbotapi.Message) {
	email := strings.TrimSpace(msg.CommandArguments())
	if email == "" {
		// 进入等待输入状态
		b.awaitingEmail.Store(msg.From.ID, true)
		b.sendText(msg.Chat.ID, "请输入您要绑定的账号邮箱：")
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
	if !isValidEmail(email) {
		b.sendText(chatID, "❌ 邮箱格式不正确，请重新发送 /bind 您的邮箱")
		return
	}
	if err := b.store.BindEmail(tgUserID, email); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// 用户记录尚不存在则补建
			_ = b.store.UpsertUser(&model.TGUser{TelegramUserID: tgUserID, Sub2APIEmail: email})
		} else {
			slog.Error("bind email failed", "error", err)
			b.sendText(chatID, "❌ 绑定失败，请稍后重试")
			return
		}
	}
	b.sendMarkdown(chatID, fmt.Sprintf("✅ 绑定成功！\n当前账号：`%s`\n\n点击 *🛒 充值* 开始购买。", email))
}
