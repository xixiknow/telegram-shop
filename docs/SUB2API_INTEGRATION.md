# sub2api 集成说明（已落地）

telegram-shop 支付成功后，会回调 **sub2api** 的 `tgshop` webhook 端点，
按 email 为用户充值余额。该接收端**已在主仓库 sub2api 落地**，本文档说明
其契约、涉及的文件与端到端验证步骤。

## 集成契约

telegram-shop 在支付成功后，向 sub2api 发起：

```
POST {sub2api}/api/v1/payment/webhook/tgshop
Headers:
  Content-Type: application/json
  X-TGShop-Timestamp: <unix 秒>
  X-TGShop-Nonce:     <随机串>
  X-TGShop-Signature: hex(HMAC_SHA256(secret, timestamp + "." + nonce + "." + rawBody))
Body:
  {
    "order_no": "tgshop_20240614abc12345",
    "trade_no": "第三方交易号",
    "email":    "user@example.com",
    "amount":   50.00,
    "base_amount": 45.00,
    "status":   "success"
  }
```

sub2api 校验签名（HMAC-SHA256 + 5 分钟时间窗防重放）后，按 `email` 找到用户，
为其充值 `amount` 元余额，返回 `HTTP 200` + body `"success"`。
**幂等**：同一 `order_no` 重复回调只充值一次。

`amount` 是到账总额（面额 + 赠额），`base_amount` 是邀请返利的人民币实付基数（面额 − 减免额）。上述示例为九折：付 45 元到账 50 元。赠送示例：付 50 元赠 5 元时，`amount=55`、`base_amount=50`。USDT 订单仍传人民币实付基数。两个金额均使用持久化订单快照，补单不重新计算活动。

## 主仓库已新增/改动的文件

| 文件 | 改动 |
|------|------|
| `backend/internal/config/config.go` | 新增 `TGShopConfig{ WebhookSecret }`，挂在 `Config.TGShop`（mapstructure `tgshop`）；默认值 `tgshop.webhook_secret=""` |
| `backend/internal/service/tgshop_service.go` | 新增 `TGShopService`：以 `tgshop-{order_no}` 派生兑换码，创建后立即兑换实现幂等充值；复用 `RedeemService` + `ContextSkipRedeemAffiliate` 避免重复返利 |
| `backend/internal/handler/tgshop_webhook_handler.go` | 新增 `TGShopWebhookHandler`：HMAC 验签 + 解析 + 调用 service；secret 取自 `cfg.TGShop.WebhookSecret` |
| `backend/internal/handler/handler.go` | `Handlers` 结构体新增 `TGShopWebhook` 字段 |
| `backend/internal/handler/wire.go` | `ProviderSet` 注册 `NewTGShopWebhookHandler`；`ProvideHandlers` 注入该 handler |
| `backend/internal/service/wire.go` | `ProviderSet` 注册 `NewTGShopService` |
| `backend/internal/server/routes/payment.go` | webhook 分组新增 `POST /payment/webhook/tgshop`；`RegisterPaymentRoutes` 增加 `tgshopWebhookHandler` 入参 |
| `backend/internal/server/router.go` | 调用处传入 `h.TGShopWebhook` |
| `backend/cmd/server/wire_gen.go` | `go generate ./cmd/server` 重新生成 |

幂等实现核心（`tgshop_service.go`）：

- 幂等键 = `tgshop-{order_no}`，兑换码 `code` 字段唯一约束兜底
- 已存在且 `IsUsed()` → 直接返回成功
- 兑换时若返回 `ErrRedeemCodeUsed`（并发竞态）→ 视为已充值，返回成功
- 充值走 `RedeemTypeBalance` 兑换码，自动复用余额入账 / 审计逻辑

## 配置共享密钥

两侧 secret 必须完全一致：

- telegram-shop：`config.yaml` → `sub2api.webhook_secret`
- sub2api：`config.yaml` → `tgshop.webhook_secret`（或环境变量 `TGSHOP_WEBHOOK_SECRET`）

sub2api 侧未配置 secret 时，该 webhook 返回 `503`，拒绝处理（安全默认）。

## 端到端验证

1. 两侧配置相同的 `webhook_secret`，启动 sub2api 与 telegram-shop
2. 在 Telegram 中 `/bind` 一个 sub2api 已存在的用户邮箱
3. 选择额度 → 易支付 / USDT → 完成支付
4. 观察：
   - telegram-shop 日志出现 `order completed`
   - sub2api 日志出现 `[TGShop] recharge success`
   - 该用户 sub2api 余额增加对应额度
   - Telegram 收到 `✅ 支付成功` 通知
5. 重放同一回调（重复推送 / 重启），确认余额**不会重复增加**（幂等）

## 故障排查

| 现象 | 排查 |
|------|------|
| 401 invalid signature | 两侧 `webhook_secret` 是否一致；服务器时钟是否同步（5 分钟窗） |
| 503 tgshop webhook disabled | sub2api 侧 `tgshop.webhook_secret` 未配置 |
| user not found | 确认 `/bind` 的邮箱在 sub2api 中存在 |
| 余额重复增加 | 检查幂等键 `tgshop-{order_no}` 与兑换码唯一约束是否生效 |
| 回调一直重试 | telegram-shop 指数退避重试；确认 sub2api 返回 200 + "success" |
