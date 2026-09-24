-- 兼容已有订单；可手动执行，应用启动时的 AutoMigrate 也会补齐字段。
-- amount 仍是用户选择的充值面额，到账 = amount + gift_amount，
-- 实付人民币/返利基数 = amount - discount_amount。
ALTER TABLE tg_orders ADD COLUMN IF NOT EXISTS gift_amount DECIMAL(20, 2) DEFAULT 0;
ALTER TABLE tg_orders ADD COLUMN IF NOT EXISTS discount_amount DECIMAL(20, 2) NOT NULL DEFAULT 0;
ALTER TABLE tg_orders ADD COLUMN IF NOT EXISTS promotion_mode VARCHAR(20) NOT NULL DEFAULT '';
ALTER TABLE tg_orders ADD COLUMN IF NOT EXISTS promotion_percent DOUBLE PRECISION NOT NULL DEFAULT 0;
