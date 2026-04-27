CREATE TABLE IF NOT EXISTS oneapi_customer_binding (
    customer_id VARCHAR(64) PRIMARY KEY,
    oneapi_user_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS oneapi_credit_job (
    id UUID PRIMARY KEY,
    order_no VARCHAR(64) UNIQUE NOT NULL REFERENCES payment_order(order_no),
    customer_id VARCHAR(64) NOT NULL,
    oneapi_user_id VARCHAR(64),
    amount NUMERIC(20, 6) NOT NULL CHECK (amount > 0),
    currency VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'failed', 'synced')),
    retry_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_oneapi_credit_job_retry ON oneapi_credit_job (status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_oneapi_credit_job_customer ON oneapi_credit_job (customer_id, updated_at DESC);
