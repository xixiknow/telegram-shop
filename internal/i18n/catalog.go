package i18n

// 文案 key 常量。新增文案时同时在 catalog 的每种语言下补齐。
const (
	// 主菜单 reply 按钮
	BtnShop     = "btn.shop"
	BtnOrders   = "btn.orders"
	BtnBalance  = "btn.balance"
	BtnAccount  = "btn.account"
	BtnHelp     = "btn.help"
	BtnLanguage = "btn.language"

	// inline 按钮
	BtnBack             = "btn.back"
	BtnEasyPay          = "btn.easypay"
	BtnUSDT             = "btn.usdt"
	BtnTRC20            = "btn.trc20"
	BtnBEP20            = "btn.bep20"
	BtnPay              = "btn.pay"
	BtnRefresh          = "btn.refresh"
	BtnLangZH           = "btn.lang.zh"
	BtnLangEN           = "btn.lang.en"
	BtnCustomAmount     = "btn.custom_amount"
	LabelAmountUnit     = "label.amount_unit" // "%g 元" 单位，仅用于额度按钮格式
	LabelAmountGift     = "label.amount_gift" // 带赠送的额度按钮
	LabelAmountDiscount = "label.amount_discount"
	MsgDiscountBanner   = "msg.discount_banner"
	LabelPromoStart     = "label.promo_start"
	LabelPromoEnd       = "label.promo_end"
	MsgQuoteTiming      = "msg.quote_timing"
	MsgQuoteUnavailable = "msg.quote_unavailable"
	LabelDiscountLine   = "label.discount_line"
	LabelPayCNY         = "label.pay_cny"
	LabelCreditAmount   = "label.credit_amount"

	// 通用提示
	MsgUseMenu       = "msg.use_menu"
	MsgUnknownCmd    = "msg.unknown_cmd"
	MsgNeedBindFirst = "msg.need_bind_first"

	// 绑定
	MsgBindPrompt  = "msg.bind_prompt"
	MsgBindInvalid = "msg.bind_invalid"
	MsgBindFailed  = "msg.bind_failed"
	MsgBindSuccess = "msg.bind_success"

	// start
	MsgWelcomeTitle   = "msg.welcome_title"
	MsgWelcomeBound   = "msg.welcome_bound"
	MsgWelcomeUnbound = "msg.welcome_unbound"

	// help
	MsgHelp = "msg.help"

	// 额度 / 支付菜单
	MsgChooseAmount       = "msg.choose_amount"
	MsgGiftBanner         = "msg.gift_banner"
	MsgChooseMethod       = "msg.choose_method"
	MsgChooseNetwork      = "msg.choose_network"
	MsgCustomAmountPrompt = "msg.custom_amount_prompt"
	MsgAmountInvalid      = "msg.amount_invalid"

	// 订单
	MsgQueryOrdersFailed = "msg.query_orders_failed"
	MsgNoOrders          = "msg.no_orders"
	MsgRecentOrders      = "msg.recent_orders"
	MsgOrderLine         = "msg.order_line"

	// 账号
	MsgAccountUnbound = "msg.account_unbound"
	MsgAccount        = "msg.account"

	// 余额
	MsgBalance            = "msg.balance"
	MsgBalanceQueryFailed = "msg.balance_query_failed"

	// 订单查询反馈
	MsgOrderNotFound  = "msg.order_not_found"
	MsgOrderCompleted = "msg.order_completed"
	MsgOrderPaid      = "msg.order_paid"
	MsgOrderPending   = "msg.order_pending"
	MsgOrderExpired   = "msg.order_expired"
	MsgOrderStatus    = "msg.order_status"

	// 下单失败
	MsgCreateOrderFailed  = "msg.create_order_failed"
	MsgActivePendingOrder = "msg.active_pending_order"
	MsgTooFast            = "msg.too_fast"

	// 支付信息
	MsgOrderCreated    = "msg.order_created"
	LabelOrderNo       = "label.order_no"
	LabelRechargeAmt   = "label.recharge_amt"
	LabelGiftLine      = "label.gift_line"
	LabelChannel       = "label.channel"
	MsgScanToPay       = "msg.scan_to_pay"
	MsgOrderValidNote  = "msg.order_valid_note"
	MsgClickToPay      = "msg.click_to_pay"
	MsgOrderValidShort = "msg.order_valid_short"
	LabelPayNetwork    = "label.pay_network"
	LabelPayAmount     = "label.pay_amount"
	LabelWallet        = "label.wallet"
	MsgUSDTExactWarn   = "msg.usdt_exact_warn"

	// 充值完成通知
	MsgPaySuccess = "msg.pay_success"

	// 渠道名
	ChannelAlipay = "channel.alipay"
	ChannelWxpay  = "channel.wxpay"

	// 状态标签
	StatusPending   = "status.pending"
	StatusPaid      = "status.paid"
	StatusCompleted = "status.completed"
	StatusFailed    = "status.failed"
	StatusExpired   = "status.expired"

	// 支付方式标签
	MethodEasyPay   = "method.easypay"
	MethodUSDTTRC20 = "method.usdt_trc20"
	MethodUSDTBEP20 = "method.usdt_bep20"

	// 语言切换反馈
	MsgChooseLanguage = "msg.choose_language"
	MsgLanguageSet    = "msg.language_set"
)

var catalog = map[Lang]map[string]string{
	ZH: {
		LabelAmountDiscount: "💰 %g 元 · 付 %.2f",
		MsgDiscountBanner:   "🎉 *限时折扣*：充值立减 *%g%%*！\n例：到账 100 元，仅付 %.2f 元。\n",
		LabelPromoStart:     "开始：%s（上海时间）\n",
		LabelPromoEnd:       "截止：%s（上海时间）\n",
		MsgQuoteTiming:      "优惠以创建订单时为准，已创建订单在有效期内保留优惠。\n",
		MsgQuoteUnavailable: "当前金额无法生成有效报价（折后应付至少 0.01 元）。请返回充值菜单，选择其他金额。",
		LabelDiscountLine:   "🎉 优惠：*%g%%*，减免 *%.2f 元*\n",
		LabelPayCNY:         "应付人民币：*%.2f 元*\n",
		LabelCreditAmount:   "实际到账：*%.2f 元*\n",
		BtnShop:             "🛒 充值",
		BtnOrders:           "📋 我的订单",
		BtnBalance:          "💰 余额",
		BtnAccount:          "👤 我的账号",
		BtnHelp:             "❓ 帮助",
		BtnLanguage:         "🌐 语言",

		BtnBack:         "⬅️ 返回",
		BtnEasyPay:      "💳 易支付（支付宝）",
		BtnUSDT:         "₮ USDT 支付",
		BtnTRC20:        "🟢 TRC20（波场 TRON）",
		BtnBEP20:        "🟡 BEP20（币安智能链 BSC）",
		BtnPay:          "🔗 去支付",
		BtnRefresh:      "🔄 我已支付/刷新状态",
		BtnLangZH:       "🇨🇳 中文",
		BtnLangEN:       "🇬🇧 English",
		BtnCustomAmount: "✏️ 自定义金额",

		LabelAmountUnit: "💰 %g 元",
		LabelAmountGift: "💰 %g 元 +%g",

		MsgUseMenu:       "请使用下方菜单操作，或发送 /help 查看帮助。",
		MsgUnknownCmd:    "未知命令，发送 /help 查看可用命令。",
		MsgNeedBindFirst: "⚠️ 请先绑定账号邮箱：/bind 您的邮箱",

		MsgBindPrompt:  "请输入您要绑定的账号邮箱：",
		MsgBindInvalid: "❌ 邮箱格式不正确，请重新发送 /bind 您的邮箱",
		MsgBindFailed:  "❌ 绑定失败，请稍后重试",
		MsgBindSuccess: "✅ 绑定成功！\n当前账号：`%s`\n\n点击 *🛒 充值* 开始购买。",

		MsgWelcomeTitle:   "👋 *欢迎使用充值机器人！*\n\n",
		MsgWelcomeBound:   "当前绑定账号：`%s`\n\n点击下方 *🛒 充值* 开始购买。",
		MsgWelcomeUnbound: "首次使用，请先绑定您的账号邮箱：\n发送 `/bind 您的邮箱` 完成绑定。\n\n例如：`/bind user@example.com`",

		MsgHelp: "*使用帮助*\n\n" +
			"/start - 开始使用\n" +
			"/bind 邮箱 - 绑定充值账号\n" +
			"/shop - 选择充值额度\n" +
			"/orders - 查看我的订单\n" +
			"/balance - 查看账户余额\n" +
			"/account - 查看绑定账号\n" +
			"/lang - 切换语言\n\n" +
			"流程：绑定邮箱 → 选择额度 → 选择支付方式 → 完成支付 → 自动到账",

		MsgChooseAmount:       "请选择充值额度：",
		MsgGiftBanner:         "🎁 *充值有礼*：本期充值额外赠送 *%g%%*！\n例：充 100 元到账 %.2f 元。\n",
		MsgChooseMethod:       "请选择支付方式：",
		MsgChooseNetwork:      "请选择 USDT 网络：",
		MsgCustomAmountPrompt: "请输入充值金额（%g ~ %g 元，支持两位小数）：",
		MsgAmountInvalid:      "❌ 金额无效。请输入 %g ~ %g 元之间的数字（最多两位小数）。",

		MsgQueryOrdersFailed: "查询订单失败，请稍后重试",
		MsgNoOrders:          "您还没有订单。点击 🛒 充值 开始购买。",
		MsgRecentOrders:      "📋 *最近订单*\n\n",
		MsgOrderLine:         "`%s`\n额度：%.2f 元 | %s | %s\n\n",

		MsgAccountUnbound: "您还未绑定账号。发送 /bind 您的邮箱 进行绑定。",
		MsgAccount:        "👤 *我的账号*\n\n绑定邮箱：`%s`\n\n如需更换，发送 /bind 新邮箱",

		MsgBalance:            "💰 *余额*\n\n邮箱：`%s`\n可用余额：%.2f\n冻结余额：%.2f",
		MsgBalanceQueryFailed: "查询余额失败，请稍后重试。",

		MsgOrderNotFound:  "未找到该订单",
		MsgOrderCompleted: "✅ 订单 `%s` 已完成，*%.2f 元* 已到账。",
		MsgOrderPaid:      "⏳ 已收到支付，正在为您充值，请稍候…",
		MsgOrderPending:   "⏳ 订单待支付。完成支付后通常会自动到账；若已支付请稍候。",
		MsgOrderExpired:   "⌛ 订单已过期，请重新下单。",
		MsgOrderStatus:    "订单状态：%s",

		MsgCreateOrderFailed:  "❌ 创建订单失败，请稍后重试",
		MsgActivePendingOrder: "⚠️ 您有一笔待支付订单 `%s` 尚未完成。\n请先完成支付，或等其过期后再下单。\n\n点击下方按钮可刷新该订单状态。",
		MsgTooFast:            "⏳ 操作过于频繁，请稍候片刻再试。",

		MsgOrderCreated:    "🧾 *订单已创建*\n\n",
		LabelOrderNo:       "订单号：`%s`\n",
		LabelRechargeAmt:   "充值额度：*%.2f 元*\n",
		LabelGiftLine:      "🎁 活动赠送：*%.2f 元*\n",
		LabelChannel:       "支付方式：*%s*\n",
		MsgScanToPay:       "\n请用 *%s* 扫描下方二维码完成支付。",
		MsgOrderValidNote:  "\n\n订单 %d 分钟内有效，到账后自动通知。",
		MsgClickToPay:      "\n请点击下方按钮完成支付 👇",
		MsgOrderValidShort: "\n\n订单 %d 分钟内有效。",
		LabelPayNetwork:    "支付网络：*%s*\n",
		LabelPayAmount:     "应付金额：*%s USDT*\n",
		LabelWallet:        "收款地址：\n`%s`\n",
		MsgUSDTExactWarn:   "\n⚠️ 请按*精确金额*向上述地址转账。",

		MsgPaySuccess: "✅ *支付成功！*\n\n订单号：`%s`\n实际到账：*%.2f 元*\n账号：`%s`\n\n余额已到账，感谢您的购买！",

		ChannelAlipay: "支付宝",
		ChannelWxpay:  "微信",

		StatusPending:   "⏳ 待支付",
		StatusPaid:      "💰 充值中",
		StatusCompleted: "✅ 已完成",
		StatusFailed:    "❌ 失败",
		StatusExpired:   "⌛ 已过期",

		MethodEasyPay:   "易支付",
		MethodUSDTTRC20: "USDT-TRC20",
		MethodUSDTBEP20: "USDT-BEP20",

		MsgChooseLanguage: "请选择语言：",
		MsgLanguageSet:    "✅ 已切换为中文。",
	},
	EN: {
		LabelAmountDiscount: "💰 ¥%g · pay ¥%.2f",
		MsgDiscountBanner:   "🎉 *Limited-time offer*: *%g%% off*!\nE.g. get ¥100 credit, pay ¥%.2f.\n",
		LabelPromoStart:     "Starts: %s (Asia/Shanghai)\n",
		LabelPromoEnd:       "Ends: %s (Asia/Shanghai)\n",
		MsgQuoteTiming:      "Offers are locked when the order is created and kept during its payment window.\n",
		MsgQuoteUnavailable: "This amount cannot be quoted (payment must be at least ¥0.01). Return to Recharge and choose another amount.",
		LabelDiscountLine:   "🎉 Discount: *%g%%*, saving *¥%.2f*\n",
		LabelPayCNY:         "Amount due in CNY: *¥%.2f*\n",
		LabelCreditAmount:   "Actual credit: *¥%.2f*\n",
		BtnBalance:          "💰 Balance",
		BtnShop:             "🛒 Recharge",
		BtnOrders:           "📋 My Orders",
		BtnAccount:          "👤 My Account",
		BtnHelp:             "❓ Help",
		BtnLanguage:         "🌐 Language",

		BtnBack:         "⬅️ Back",
		BtnEasyPay:      "💳 EasyPay (Alipay)",
		BtnUSDT:         "₮ Pay with USDT",
		BtnTRC20:        "🟢 TRC20 (TRON)",
		BtnBEP20:        "🟡 BEP20 (BSC)",
		BtnPay:          "🔗 Pay now",
		BtnRefresh:      "🔄 I've paid / Refresh status",
		BtnLangZH:       "🇨🇳 中文",
		BtnLangEN:       "🇬🇧 English",
		BtnCustomAmount: "✏️ Custom amount",

		LabelAmountUnit: "💰 ¥%g",
		LabelAmountGift: "💰 ¥%g +%g",

		MsgUseMenu:       "Please use the menu below, or send /help for assistance.",
		MsgUnknownCmd:    "Unknown command. Send /help to see available commands.",
		MsgNeedBindFirst: "⚠️ Please bind your account email first: /bind your@email.com",

		MsgBindPrompt:  "Please enter the account email you want to bind:",
		MsgBindInvalid: "❌ Invalid email format. Please resend /bind your@email.com",
		MsgBindFailed:  "❌ Binding failed, please try again later.",
		MsgBindSuccess: "✅ Bound successfully!\nCurrent account: `%s`\n\nTap *🛒 Recharge* to start.",

		MsgWelcomeTitle:   "👋 *Welcome to the recharge bot!*\n\n",
		MsgWelcomeBound:   "Current account: `%s`\n\nTap *🛒 Recharge* below to start.",
		MsgWelcomeUnbound: "First time here? Please bind your account email:\nSend `/bind your@email.com` to bind.\n\nExample: `/bind user@example.com`",

		MsgHelp: "*Help*\n\n" +
			"/start - Get started\n" +
			"/bind <email> - Bind your recharge account\n" +
			"/shop - Choose a recharge amount\n" +
			"/orders - View my orders\n" +
			"/balance - View account balance\n" +
			"/account - View bound account\n" +
			"/lang - Switch language\n\n" +
			"Flow: bind email → choose amount → choose payment → pay → auto credit",

		MsgChooseAmount:       "Please choose a recharge amount:",
		MsgGiftBanner:         "🎁 *Bonus*: extra *%g%%* on this recharge!\nE.g. pay ¥100, get ¥%.2f.\n",
		MsgChooseMethod:       "Please choose a payment method:",
		MsgChooseNetwork:      "Please choose a USDT network:",
		MsgCustomAmountPrompt: "Please enter the amount (¥%g ~ ¥%g, up to 2 decimals):",
		MsgAmountInvalid:      "❌ Invalid amount. Please enter a number between ¥%g and ¥%g (max 2 decimals).",

		MsgQueryOrdersFailed: "Failed to fetch orders, please try again later.",
		MsgNoOrders:          "You have no orders yet. Tap 🛒 Recharge to start.",
		MsgRecentOrders:      "📋 *Recent Orders*\n\n",
		MsgOrderLine:         "`%s`\nAmount: ¥%.2f | %s | %s\n\n",

		MsgAccountUnbound: "No account bound yet. Send /bind your@email.com to bind.",
		MsgAccount:        "👤 *My Account*\n\nBound email: `%s`\n\nTo change, send /bind <new email>",

		MsgBalance:            "💰 *Balance*\n\nEmail: `%s`\nAvailable: %.2f\nFrozen: %.2f",
		MsgBalanceQueryFailed: "Failed to query balance, please try again later.",

		MsgOrderNotFound:  "Order not found.",
		MsgOrderCompleted: "✅ Order `%s` completed, *¥%.2f* credited.",
		MsgOrderPaid:      "⏳ Payment received, crediting your account, please wait…",
		MsgOrderPending:   "⏳ Order pending payment. It usually credits automatically once paid; if already paid, please wait.",
		MsgOrderExpired:   "⌛ Order expired, please place a new order.",
		MsgOrderStatus:    "Order status: %s",

		MsgCreateOrderFailed:  "❌ Failed to create order, please try again later.",
		MsgActivePendingOrder: "⚠️ You already have an unpaid order `%s`.\nPlease complete it first, or wait for it to expire before placing a new one.\n\nTap the button below to refresh its status.",
		MsgTooFast:            "⏳ Too many requests, please wait a moment and try again.",

		MsgOrderCreated:    "🧾 *Order Created*\n\n",
		LabelOrderNo:       "Order No: `%s`\n",
		LabelRechargeAmt:   "Amount: *¥%.2f*\n",
		LabelGiftLine:      "🎁 Bonus: *¥%.2f*\n",
		LabelChannel:       "Payment: *%s*\n",
		MsgScanToPay:       "\nPlease scan the QR code below with *%s* to pay.",
		MsgOrderValidNote:  "\n\nValid for %d minutes; you'll be notified once credited.",
		MsgClickToPay:      "\nPlease tap the button below to pay 👇",
		MsgOrderValidShort: "\n\nValid for %d minutes.",
		LabelPayNetwork:    "Network: *%s*\n",
		LabelPayAmount:     "Amount due: *%s USDT*\n",
		LabelWallet:        "Address:\n`%s`\n",
		MsgUSDTExactWarn:   "\n⚠️ Please transfer the *exact amount* to the address above.",

		MsgPaySuccess: "✅ *Payment successful!*\n\nOrder No: `%s`\nCredited: *¥%.2f*\nAccount: `%s`\n\nBalance credited. Thank you for your purchase!",

		ChannelAlipay: "Alipay",
		ChannelWxpay:  "WeChat Pay",

		StatusPending:   "⏳ Pending",
		StatusPaid:      "💰 Crediting",
		StatusCompleted: "✅ Completed",
		StatusFailed:    "❌ Failed",
		StatusExpired:   "⌛ Expired",

		MethodEasyPay:   "EasyPay",
		MethodUSDTTRC20: "USDT-TRC20",
		MethodUSDTBEP20: "USDT-BEP20",

		MsgChooseLanguage: "Please choose a language:",
		MsgLanguageSet:    "✅ Switched to English.",
	},
}
