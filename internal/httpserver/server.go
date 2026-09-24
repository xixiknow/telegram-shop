// Package httpserver 提供支付回调与健康检查的 HTTP 服务。
package httpserver

import (
	"io"
	"log/slog"
	"net/http"

	"telegram-shop/internal/config"
	"telegram-shop/internal/service"

	"github.com/gin-gonic/gin"
)

const maxBodySize = 1 << 20 // 1MB

// Server 是 HTTP 服务。
type Server struct {
	cfg          *config.Config
	orderService *service.OrderService
	engine       *gin.Engine
	onCompleted  func(orderNo string) // 订单完成回调（用于通知 Telegram 用户）
}

// New 创建 HTTP server。onCompleted 在订单充值完成后被调用（可为 nil）。
func New(cfg *config.Config, orderService *service.OrderService, onCompleted func(orderNo string)) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	s := &Server{cfg: cfg, orderService: orderService, engine: engine, onCompleted: onCompleted}
	s.registerRoutes()
	return s
}

// Engine 暴露 gin engine（webhook 模式下注册 Telegram 回调用）。
func (s *Server) Engine() *gin.Engine { return s.engine }

// Run 启动 HTTP 服务（阻塞）。
func (s *Server) Run() error {
	slog.Info("http server listening", "addr", s.cfg.Server.Addr)
	return s.engine.Run(s.cfg.Server.Addr)
}

func (s *Server) registerRoutes() {
	s.engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	webhook := s.engine.Group("/api/webhook")
	{
		// 易支付通常以 GET 回调（query string）
		webhook.GET("/easypay", s.handleEasyPay)
		webhook.POST("/easypay", s.handleEasyPay)
		// USDT（BEpusdt 网关）异步回调（POST JSON，MD5+Token 验签）
		webhook.POST("/usdt", s.handleUSDT)
	}
}

// handleEasyPay 处理易支付回调。
func (s *Server) handleEasyPay(c *gin.Context) {
	provider, ok := s.orderService.Provider("easypay")
	if !ok {
		c.String(http.StatusServiceUnavailable, "easypay disabled")
		return
	}

	var rawBody string
	query := map[string]string{}
	if c.Request.Method == http.MethodGet {
		for k := range c.Request.URL.Query() {
			query[k] = c.Query(k)
		}
	} else {
		body, _ := io.ReadAll(io.LimitReader(c.Request.Body, maxBodySize))
		rawBody = string(body)
	}

	notification, err := provider.VerifyNotification(c.Request.Context(), rawBody, query)
	if err != nil {
		slog.Error("easypay verify failed", "error", err)
		c.String(http.StatusBadRequest, "verify failed")
		return
	}
	if notification == nil || !notification.Success {
		// 无关事件或未成功，回 success 让服务商停止重试
		c.String(http.StatusOK, provider.SuccessResponse())
		return
	}

	if _, err := s.orderService.HandlePaymentSuccess(
		c.Request.Context(), notification.OrderNo, notification.TradeNo, notification.Amount,
	); err != nil {
		slog.Error("easypay fulfillment failed", "orderNo", notification.OrderNo, "error", err)
		c.String(http.StatusInternalServerError, "handle failed")
		return
	}

	s.notifyCompleted(notification.OrderNo)
	c.String(http.StatusOK, provider.SuccessResponse())
}

// handleUSDT 处理 BEpusdt 网关的 USDT 到账回调（POST JSON）。
// 验签后按 order_id 完成充值；status=2 时须回 "ok"。
func (s *Server) handleUSDT(c *gin.Context) {
	provider, ok := s.orderService.USDTProvider()
	if !ok {
		c.String(http.StatusServiceUnavailable, "usdt disabled")
		return
	}

	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, maxBodySize))

	notification, err := provider.VerifyNotification(c.Request.Context(), string(body), nil)
	if err != nil {
		slog.Error("usdt verify failed", "error", err)
		c.String(http.StatusBadRequest, "verify failed")
		return
	}
	if notification == nil || !notification.Success {
		// status=1(待支付)/3(超时) 或无关事件：回 200 即可（BEpusdt 对非成功回调只需 200）。
		c.String(http.StatusOK, provider.SuccessResponse())
		return
	}

	if _, err := s.orderService.HandlePaymentSuccess(
		c.Request.Context(), notification.OrderNo, notification.TradeNo, notification.Amount,
	); err != nil {
		slog.Error("usdt fulfillment failed", "orderNo", notification.OrderNo, "error", err)
		c.String(http.StatusInternalServerError, "handle failed")
		return
	}

	s.notifyCompleted(notification.OrderNo)
	c.String(http.StatusOK, provider.SuccessResponse())
}

func (s *Server) notifyCompleted(orderNo string) {
	if s.onCompleted != nil {
		go s.onCompleted(orderNo)
	}
}
