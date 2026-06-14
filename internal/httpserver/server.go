// Package httpserver 提供支付回调与健康检查的 HTTP 服务。
package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

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
		// USDT 链上监听服务回调（内部 HMAC 签名）
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

// usdtNotifyPayload 是 USDT 链上监听服务推送的到账通知。
type usdtNotifyPayload struct {
	TxHash    string  `json:"tx_hash"`
	Amount    float64 `json:"amount"`     // 到账 USDT 金额
	ToAddress string  `json:"to_address"` // 收款地址
	OrderNo   string  `json:"order_no"`   // 可选：监听服务已匹配的订单号
}

// handleUSDT 处理 USDT 到账回调。
// 通过 HMAC 签名校验后，按订单号或金额匹配待支付订单并完成充值。
func (s *Server) handleUSDT(c *gin.Context) {
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, maxBodySize))

	if !s.verifyInternalSignature(c, body) {
		slog.Warn("usdt webhook signature invalid")
		c.String(http.StatusUnauthorized, "invalid signature")
		return
	}

	var payload usdtNotifyPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.String(http.StatusBadRequest, "invalid payload")
		return
	}

	orderNo := payload.OrderNo
	if orderNo == "" {
		// 监听服务未匹配订单号时，按金额匹配
		order, err := s.orderService.Store().FindPendingOrderByUSDTAmount(payload.Amount)
		if err != nil {
			slog.Warn("usdt no matching order", "amount", payload.Amount, "tx", payload.TxHash)
			// 回 200 避免重复推送（金额不匹配可能是无关转账）
			c.String(http.StatusOK, "success")
			return
		}
		orderNo = order.OrderNo
	}

	if _, err := s.orderService.HandlePaymentSuccess(
		c.Request.Context(), orderNo, payload.TxHash, payload.Amount,
	); err != nil {
		slog.Error("usdt fulfillment failed", "orderNo", orderNo, "error", err)
		c.String(http.StatusInternalServerError, "handle failed")
		return
	}

	s.notifyCompleted(orderNo)
	c.String(http.StatusOK, "success")
}

// verifyInternalSignature 校验内部 HMAC 签名（复用 sub2api.webhook_secret）。
// 签名格式：hex(HMAC_SHA256(secret, timestamp + "." + body))，5 分钟有效。
func (s *Server) verifyInternalSignature(c *gin.Context, body []byte) bool {
	timestamp := c.GetHeader("X-Signature-Timestamp")
	signature := c.GetHeader("X-Signature")
	if timestamp == "" || signature == "" {
		return false
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if time.Since(time.Unix(ts, 0)) > 5*time.Minute {
		return false
	}

	mac := hmac.New(sha256.New, []byte(s.cfg.Sub2API.WebhookSecret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (s *Server) notifyCompleted(orderNo string) {
	if s.onCompleted != nil {
		go s.onCompleted(orderNo)
	}
}
