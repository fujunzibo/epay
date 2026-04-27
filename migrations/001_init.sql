CREATE TABLE IF NOT EXISTS payment_order (
    id UUID PRIMARY KEY,
    order_no VARCHAR(64) UNIQUE NOT NULL,
    customer_id VARCHAR(64) NOT NULL,
    payment_method VARCHAR(16) NOT NULL CHECK (payment_method IN ('fiat', 'crypto')),
    provider VARCHAR(32) NOT NULL,
    currency VARCHAR(16) NOT NULL,
    amount NUMERIC(20, 6) NOT NULL CHECK (amount > 0),
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'credited', 'failed')),
    credited BOOLEAN NOT NULL DEFAULT FALSE,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS payment_event (
    id UUID PRIMARY KEY,
    order_no VARCHAR(64) NOT NULL REFERENCES payment_order(order_no),
    event_key VARCHAR(128) UNIQUE NOT NULL,
    source VARCHAR(32) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS wallet_account (
    customer_id VARCHAR(64) PRIMARY KEY,
    currency VARCHAR(16) NOT NULL DEFAULT 'USD',
    balance NUMERIC(20, 6) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS wallet_ledger (
    id UUID PRIMARY KEY,
    customer_id VARCHAR(64) NOT NULL,
    order_no VARCHAR(64) NOT NULL,
    entry_type VARCHAR(16) NOT NULL CHECK (entry_type IN ('topup')),
    amount NUMERIC(20, 6) NOT NULL CHECK (amount > 0),
    currency VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (order_no, entry_type)
);
