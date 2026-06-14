// Command bot 是 Telegram Shop 的启动入口。
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"telegram-shop/internal/bot"
	"telegram-shop/internal/config"
	"telegram-shop/internal/httpserver"
	"telegram-shop/internal/payment"
	"telegram-shop/internal/service"
	"telegram-shop/internal/store"
	"telegram-shop/internal/sub2api"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	st, err := store.New(cfg.Database.DSN)
	if err != nil {
		slog.Error("init store failed", "error", err)
		os.Exit(1)
	}

	// 组装支付 providers
	providers := buildProviders(cfg)

	// sub2api 充值回调客户端
	s2a := sub2api.New(
		cfg.Sub2API.WebhookURL,
		cfg.Sub2API.WebhookSecret,
		cfg.Sub2API.TimeoutSeconds,
		cfg.Sub2API.MaxRetries,
	)

	orderService := service.NewOrderService(cfg, st, providers, s2a)

	// Telegram Bot
	tgBot, err := bot.New(cfg, st, orderService)
	if err != nil {
		slog.Error("init bot failed", "error", err)
		os.Exit(1)
	}

	// HTTP 服务（支付回调），订单完成后通知用户
	httpSrv := httpserver.New(cfg, orderService, tgBot.NotifyOrderCompleted)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动订单过期清理
	go runExpiryLoop(ctx, orderService)

	// 启动 USDT 内置 TRON 链上轮询（启用时）
	if cfg.Payment.USDT.Enabled && cfg.Payment.USDT.Poll.Enabled {
		poller := payment.NewUSDTPoller(cfg.Payment.USDT.Poll, cfg.Payment.USDT.WalletAddress, orderService)
		go poller.Run(ctx)
		slog.Info("usdt tron poller enabled")
	}

	// 启动 HTTP 服务
	go func() {
		if cfg.Telegram.UseWebhook {
			registerTelegramWebhook(cfg, tgBot, httpSrv, ctx)
		}
		if err := httpSrv.Run(); err != nil {
			slog.Error("http server stopped", "error", err)
		}
	}()

	// 启动 Bot（polling 模式）
	if !cfg.Telegram.UseWebhook {
		go tgBot.StartPolling(ctx)
	}

	slog.Info("telegram-shop started")

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	slog.Info("shutting down...")
	cancel()
	time.Sleep(500 * time.Millisecond)
}

func buildProviders(cfg *config.Config) map[string]payment.Provider {
	providers := make(map[string]payment.Provider)
	if cfg.Payment.EasyPay.Enabled {
		notifyURL := cfg.Server.BaseURL + "/api/webhook/easypay"
		returnURL := cfg.Server.BaseURL + "/health"
		providers[payment.MethodEasyPay] = payment.NewEasyPay(cfg.Payment.EasyPay, notifyURL, returnURL)
		slog.Info("easypay provider enabled")
	}
	if cfg.Payment.USDT.Enabled {
		providers[payment.MethodUSDT] = payment.NewUSDT(cfg.Payment.USDT)
		slog.Info("usdt provider enabled")
	}
	return providers
}

func runExpiryLoop(ctx context.Context, orderService *service.OrderService) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			orderService.ExpireOrders(ctx)
		}
	}
}

// registerTelegramWebhook 在 webhook 模式下设置 Telegram webhook 并注册路由。
func registerTelegramWebhook(cfg *config.Config, tgBot *bot.Bot, httpSrv *httpserver.Server, ctx context.Context) {
	wh, err := tgbotapi.NewWebhook(cfg.Server.BaseURL + cfg.Telegram.WebhookPath)
	if err != nil {
		slog.Error("create webhook failed", "error", err)
		return
	}
	if _, err := tgBot.API().Request(wh); err != nil {
		slog.Error("set webhook failed", "error", err)
		return
	}
	httpSrv.Engine().POST(cfg.Telegram.WebhookPath, func(c *gin.Context) {
		update, err := tgBot.API().HandleUpdate(c.Request)
		if err != nil {
			c.Status(400)
			return
		}
		tgBot.HandleUpdate(ctx, *update)
		c.Status(200)
	})
	slog.Info("telegram webhook registered", "path", cfg.Telegram.WebhookPath)
}
