# Telegram Shop

一个独立的 Telegram 充值机器人，支持在 Telegram 内通过 **菜单** 选择固定额度（5/10/20/50/100/200/500 元）下单，使用 **易支付** 或 **USDT** 付款，支付成功后自动回调 [sub2api](../sub2api) 完成余额充值。

## 特性

- 🤖 **纯菜单交互** — 常驻底部菜单 + Inline 键盘，额度/支付方式一键选择
- 💳 **易支付** — Dulupay V2 协议（支付宝/微信），页面跳转（api/pay/submit）+ SHA256WithRSA 签名
- ₮ **USDT** — 自托管 TRC20 收款，**内置 TronGrid 链上轮询** + 唯一金额匹配，无需外部监听服务
- 🔗 **sub2api 集成** — HMAC 签名的 webhook 回调，幂等充值
- 🔁 **可靠性** — 订单状态机 + 自动过期 + 回调重试（指数退避）
- 📦 **独立部署** — 与 sub2api 解耦，独立数据库，Docker 一键启动

## 架构

```
┌──────────────┐   选择额度/支付    ┌─────────────────────┐
│ Telegram 用户 │ ───────────────▶ │   Telegram Shop      │
└──────────────┘                   │  - Bot 菜单交互       │
       ▲                           │  - 订单/支付编排      │
       │ ✅ 到账通知                 │  - 易支付/USDT        │
       │                           └──────────┬──────────┘
       │                                      │ 支付回调
       │                          ┌───────────┴───────────┐
       │                          │  易支付网关 / USDT 监听  │
       │                          └───────────┬───────────┘
       │                                      │ 验签成功
       │                                      ▼
       │                 HMAC 签名   ┌─────────────────────┐
       └───────────────────────────│   sub2api            │
                  充值完成            │  /webhook/tgshop     │
                                    │  → 按 email 充值余额  │
                                    └─────────────────────┘
```

## 交互流程

```
用户: /start
Bot:  👋 欢迎！首次使用请绑定邮箱：/bind user@example.com
      [🛒 充值] [📋 我的订单] [👤 我的账号] [❓ 帮助]

用户: /bind user@example.com
Bot:  ✅ 绑定成功！

用户: 点击 [🛒 充值]
Bot:  请选择充值额度：
      [💰 5 元]   [💰 10 元]
      [💰 20 元]  [💰 50 元]
      [💰 100 元] [💰 200 元]
      [💰 500 元]

用户: 点击 [💰 50 元]
Bot:  充值额度：50 元，请选择支付方式：
      [💳 易支付（支付宝/微信）]
      [₮ USDT 支付]
      [⬅️ 返回]

用户: 点击 [💳 易支付]
Bot:  🧾 订单已创建
      订单号：tgshop_20240614abc12345
      充值额度：50.00 元
      [🔗 去支付] [🔄 我已支付/刷新状态]

(支付完成，sub2api 回调成功后)
Bot:  ✅ 支付成功！50.00 元已到账。
```

## 快速开始

### 1. 配置

复制并编辑 `config.yaml`：

```yaml
telegram:
  bot_token: "从 @BotFather 获取"
  admin_ids: [你的TelegramUserID]

amounts: [5, 10, 20, 50, 100, 200, 500]

database:
  dsn: "host=localhost port=5432 user=postgres password=xxx dbname=telegram_shop sslmode=disable"

payment:
  easypay:
    enabled: true
    gateway_url: "https://api.dulupay.com"   # Dulupay 网关
    merchant_id: "商户ID(PID)"
    platform_public_key: "平台公钥(裸base64)"   # Dulupay 后台「API信息」
    merchant_private_key: "商户私钥(裸base64)"  # Dulupay 后台「API信息」生成的RSA密钥对
    default_channel: "alipay"                 # alipay / wxpay / qqpay / bank
  usdt:
    enabled: true
    wallet_address: "你的USDT-TRC20收款地址"
    cny_rate: 7.2
    unique_amount: true
    poll:                                     # 内置链上轮询（推荐）
      enabled: true
      trongrid_api_key: "可选，提升频率限制"

sub2api:
  webhook_url: "https://你的sub2api域名/api/v1/payment/webhook/tgshop"
  webhook_secret: "与sub2api约定的共享密钥"
```

### 2. 运行

**本地开发（long polling）：**

```bash
go run ./cmd/bot -config config.yaml
```

**Docker：**

```bash
docker compose up -d
```

### 3. sub2api 集成

参见 [`docs/SUB2API_INTEGRATION.md`](docs/SUB2API_INTEGRATION.md)，在 sub2api 侧添加 tgshop webhook 接收端点。

## 目录结构

```
telegram-shop/
├── cmd/bot/main.go              # 启动入口，依赖装配
├── internal/
│   ├── config/                  # 配置加载
│   ├── model/                   # 数据模型 + 状态常量
│   ├── store/                   # GORM 数据访问层
│   ├── payment/                 # 支付抽象 + 易支付 + USDT
│   ├── sub2api/                 # sub2api 充值回调客户端（HMAC）
│   ├── service/                 # 订单编排 + 履约
│   ├── httpserver/              # 支付回调 webhook HTTP 服务
│   └── bot/                     # Telegram Bot 菜单交互
├── migrations/001_init.sql      # 数据库迁移（亦支持 AutoMigrate）
├── docs/SUB2API_INTEGRATION.md  # sub2api 集成指南
├── Dockerfile
└── docker-compose.yml
```

## 订单状态机

```
pending ──支付成功──▶ paid ──sub2api充值成功──▶ completed
   │                                              
   └──超时──▶ expired                            
                                                  
回调失败时停留在 paid，sub2api_callback_status=failed，
下次回调/刷新会重试（幂等）。
```

## 安全说明

- 易支付回调按 Dulupay V2（SHA256WithRSA）平台公钥验签 + timestamp 时间窗防重放
- USDT / sub2api 回调使用 HMAC-SHA256 + 时间戳防重放
- ⚠️ 网络暴露的 webhook 端点务必置于 HTTPS 之后
- ⚠️ `config.yaml` 含密钥，生产环境请用 secrets 管理，勿提交仓库
