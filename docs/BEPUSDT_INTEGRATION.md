# BEpusdt 网关接入说明

telegram-shop 的 USDT 收款通过 **BEpusdt**（https://github.com/v03413/BEpusdt）网关完成。
BEpusdt 负责多链（TRC20 / BEP20 等）收款地址生成、链上确认、汇率换算；telegram-shop
仅作为它的商户：下单时调用其创建交易接口，支付到账后接收其回调，再回调 sub2api 给用户加余额。

## 架构

```
Telegram 用户 → telegram-shop（选额度/选链/渲染地址二维码）
                   │ create-transaction（MD5+Token 签名）
                   ▼
                BEpusdt（生成收款地址 + 链上扫描确认 + 汇率）
                   │ 到账回调 status=2（MD5+Token 签名）
                   ▼
                telegram-shop /api/webhook/usdt → 回调 sub2api 充值
```

> 用户全程在 Telegram 内完成：bot 直接发收款地址二维码图片 + 应付 USDT 金额，不跳转 BEpusdt 收银台网页。

## 部署 BEpusdt

```bash
docker run -d -p 8080:8080 --name bepusdt v03413/bepusdt:latest
```

在 BEpusdt 后台完成：
- 各链（TRON / BSC）的**收款地址**与 **RPC 节点**配置（BSC 用公共 RPC 即可，免费）
- 「系统管理 → 基础设置 → API 设置」获取 **Integration Token**

## telegram-shop 侧配置（config.yaml）

```yaml
payment:
  usdt:
    enabled: true
    base_url: "https://pay.yourdomain.com"   # BEpusdt 地址
    api_token: "<BEpusdt Integration Token>"
    fiat: "CNY"
    timeout_seconds: 15
    trc20_enabled: true
    bep20_enabled: true
```

> `base_url` 是 telegram-shop 后端访问 BEpusdt 的地址，可用内网地址。
> 回调地址自动拼为 `{server.base_url}/api/webhook/usdt`，需保证 BEpusdt 能访问到（公网或内网互通）。

## 对接契约

### 创建交易
`POST {base_url}/api/v1/order/create-transaction`（JSON）
- 请求：`order_id`、`amount`、`notify_url`、`redirect_url`、`trade_type`(usdt.trc20/usdt.bep20)、`fiat`、`signature`
- 响应 `data`：`token`(收款地址)、`actual_amount`(应付 USDT)、`trade_id`、`expiration_time`

### 支付回调
BEpusdt `POST` 到 `notify_url`（JSON）：
- 字段：`trade_id`、`order_id`、`amount`、`actual_amount`、`token`、`block_transaction_id`、`status`、`signature`
- `status`：1=待支付，2=成功，3=超时
- telegram-shop 对 `status=2` 验签通过后回调 sub2api，并返回 HTTP 200 + body `ok`

### 签名算法（请求与回调对称）
取所有非空且非 `signature` 的参数 → 按 key ASCII 升序 → `k=v` 用 `&` 连接 →
末尾**直接拼接** API Token（无 `&`）→ MD5 → 小写 hex。

## 注意事项

- **回调验签的数字序列化**：回调 body 里 `amount`/`status` 为数字。telegram-shop 验签时把 JSON 数字按最简形式（整数无小数点、浮点去尾零）还原为字符串参与签名。若 BEpusdt 版本对数字的序列化方式不同，导致验签失败，需对照其 `utils.EpusdtSign` 实现调整 `stringifyJSONValue`（见 `internal/payment/bepusdt.go`）。建议接入时先用真实回调验证一次。
- **幂等**：同一 `order_id` 重复回调由 `HandlePaymentSuccess` + sub2api `tgshop-{order_no}` 幂等键保证只充值一次。
- BSC 收款用公共 RPC 节点即可（BEpusdt 内置区块扫描），无需 Etherscan 付费 key。
