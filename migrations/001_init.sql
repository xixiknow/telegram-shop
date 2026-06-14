-- Telegram Shop 初始化迁移
-- 可手动执行，或依赖应用启动时的 GORM AutoMigrate

CREATE TABLE IF NOT EXISTS tg_users (
    id               BIGSERIAL PRIMARY KEY,
    telegram_user_id BIGINT       NOT NULL UNIQUE,
    username         VARCHAR(255) NOT NULL DEFAULT '',
    first_name       VARCHAR(255) NOT NULL DEFAULT '',
    last_name        VARCHAR(255) NOT NULL DEFAULT '',
    language_code    VARCHAR(10)  NOT NULL DEFAULT '',
    sub2api_email    VARCHAR(255) NOT NULL DEFAULT '',
    is_blocked       BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tg_users_sub2api_email ON tg_users (sub2api_email);

CREATE TABLE IF NOT EXISTS tg_orders (
    id                       BIGSERIAL PRIMARY KEY,
    order_no                 VARCHAR(64)    NOT NULL UNIQUE,
    telegram_user_id         BIGINT         NOT NULL,
    telegram_username        VARCHAR(255)   NOT NULL DEFAULT '',
    amount                   DECIMAL(20, 2) NOT NULL,
    pay_amount               DECIMAL(20, 8) NOT NULL,
    pay_currency             VARCHAR(10)    NOT NULL DEFAULT 'CNY',
    payment_method           VARCHAR(30)    NOT NULL,
    status                   VARCHAR(30)    NOT NULL DEFAULT 'pending',
    payment_trade_no         VARCHAR(128)   NOT NULL DEFAULT '',
    pay_url                  TEXT           NOT NULL DEFAULT '',
    qr_code                  TEXT           NOT NULL DEFAULT '',
    sub2api_email            VARCHAR(255)   NOT NULL,
    sub2api_callback_status  VARCHAR(30)    NOT NULL DEFAULT 'pending',
    sub2api_callback_error   TEXT           NOT NULL DEFAULT '',
    expires_at               TIMESTAMPTZ,
    paid_at                  TIMESTAMPTZ,
    completed_at             TIMESTAMPTZ,
    created_at               TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tg_orders_telegram_user_id ON tg_orders (telegram_user_id);
CREATE INDEX IF NOT EXISTS idx_tg_orders_status ON tg_orders (status);
