# ePay MVP (Go + PostgreSQL + Stripe + USDC)

This is a 2-3 day MVP scaffold for:
- **React admin UI** (`admin/`, Ant Design + Vite + HashRouter)
- Unified payment orders (`fiat` / `crypto`)
- Stripe standard webhook ingestion (`payment_intent.succeeded` / `checkout.session.completed` / `invoice.payment_succeeded`)
- USDC webhook ingestion with confirmation check
- Automatic API balance top-up after payment success
- Idempotent event processing and wallet ledger
- Auto retry for uncredited paid orders
- Basic reconcile and payment metrics endpoints
- Stripe failed/refund event handling and alert webhook

## 1) Prepare PostgreSQL

Create database and run migration:

```sql
\i migrations/001_init.sql
\i migrations/002_oneapi_sync.sql
```

## 2) Environment

```bash
set HTTP_ADDR=:8080
set POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/epay?sslmode=disable
set STRIPE_WEBHOOK_SECRET=whsec_xxx
set USDC_WEBHOOK_BEARER=your_usdc_webhook_token
set USDC_WEBHOOK_HMAC_SECRET=your_usdc_hmac_secret
set USDC_MIN_CONFIRMATIONS=1
set ALERT_WEBHOOK_URL=https://example.com/ops-alert
set ALERT_WEBHOOK_BEARER=ops_token
set RETRY_INTERVAL_SECONDS=30
set CORS_ALLOW_ORIGINS=http://localhost:5173,http://127.0.0.1:5173
set ONEAPI_TOPUP_URL=http://127.0.0.1:3000/api/internal/topup
set ONEAPI_TOPUP_BEARER=oneapi_internal_token
set ONEAPI_TIMEOUT_SECONDS=10
set ONEAPI_RETRY_SECONDS=30
```

If you want local testing without signature enforcement, leave `STRIPE_WEBHOOK_SECRET` and `USDC_WEBHOOK_HMAC_SECRET` empty.

`CORS_ALLOW_ORIGINS` is a comma-separated list of browser origins allowed to call the API (used when the admin runs on Vite dev port `5173`).

## 3) Run

```bash
go mod tidy
go run ./cmd/server
```

Run retry worker (optional but recommended):

```bash
go run ./cmd/worker
```

Run one-off reconcile report:

```bash
go run ./cmd/reconcile
```

## 3b) Admin UI (React)

**Development (Vite proxy → API on :18080 by default):**

```bash
cd admin
npm install
npm run dev
```

Open **http://localhost:5173/admin/#/dashboard** (HashRouter; API requests are proxied to `http://127.0.0.1:18080`).

If your backend runs on a different port, set:

```bash
set VITE_PROXY_TARGET=http://127.0.0.1:8080
npm run dev
```

**Production (static files served by Go):**

```bash
cd admin
npm run build
cd ..
go run ./cmd/server
```

Open **http://localhost:18080/admin/** (redirects to `/admin/`) then use the sidebar, or go directly to **http://localhost:18080/admin/#/dashboard**.

Override static directory with `ADMIN_STATIC_DIR` if you build the UI elsewhere.

## 4) API quickstart

Create an order:

```bash
curl -X POST http://localhost:8080/payments/orders ^
  -H "Content-Type: application/json" ^
  -d "{\"customer_id\":\"cust_001\",\"payment_method\":\"fiat\",\"provider\":\"stripe\",\"currency\":\"USD\",\"amount\":20}"
```

Stripe webhook (standard Stripe event payload):

```bash
curl -X POST http://localhost:8080/payments/webhook/stripe ^
  -H "Content-Type: application/json" ^
  -d "{\"id\":\"evt_123\",\"type\":\"payment_intent.succeeded\",\"data\":{\"object\":{\"id\":\"pi_123\",\"amount_received\":2000,\"currency\":\"usd\",\"metadata\":{\"order_no\":\"ord_xxx\"}}}}"
```

Stripe failed event example:

```bash
curl -X POST http://localhost:8080/payments/webhook/stripe ^
  -H "Content-Type: application/json" ^
  -d "{\"id\":\"evt_124\",\"type\":\"invoice.payment_failed\",\"data\":{\"object\":{\"id\":\"in_123\",\"billing_reason\":\"subscription_cycle\",\"metadata\":{\"order_no\":\"ord_xxx\"}}}}"
```

USDC success webhook (MVP payload):

```bash
curl -X POST http://localhost:8080/payments/webhook/usdc ^
  -H "Authorization: Bearer your_usdc_webhook_token" ^
  -H "X-USDC-Signature: <sha256-hmac-of-body>" ^
  -H "Content-Type: application/json" ^
  -d "{\"order_no\":\"ord_xxx\",\"chain\":\"ethereum\",\"tx_hash\":\"0xabc\",\"to_address\":\"0xwallet\",\"token\":\"USDC\",\"amount\":20,\"currency\":\"USD\",\"confirmations\":3,\"required_confirmations\":1}"
```

List orders:

```bash
curl "http://localhost:8080/payments/orders?page=1&limit=20"
```

Query one order:

```bash
curl http://localhost:8080/payments/orders/ord_xxx
```

Manual retry credit:

```bash
curl -X POST http://localhost:8080/payments/orders/ord_xxx/retry-credit
```

Wallet balances:

```bash
curl "http://localhost:8080/ops/wallets?page=1&limit=20"
```

Auto retry failed credits (batch):

```bash
curl -X POST "http://localhost:8080/ops/retry-failed?limit=100"
```

Bind customer to OneAPI user:

```bash
curl -X POST http://localhost:8080/ops/oneapi/bind ^
  -H "Content-Type: application/json" ^
  -d "{\"customer_id\":\"cust_001\",\"oneapi_user_id\":\"1001\"}"
```

Query OneAPI binding:

```bash
curl "http://localhost:8080/ops/oneapi/bind/cust_001"
```

Retry OneAPI top-up jobs:

```bash
curl -X POST "http://localhost:8080/ops/oneapi/retry?limit=100"
```

List OneAPI top-up jobs:

```bash
curl "http://localhost:8080/ops/oneapi/jobs?status=failed&limit=50"
```

Reconcile summary:

```bash
curl "http://localhost:8080/ops/reconcile/summary?since_hours=24"
```

Reconcile details:

```bash
curl "http://localhost:8080/ops/reconcile/details?mode=uncredited_paid&limit=100"
curl "http://localhost:8080/ops/reconcile/details?mode=orders_no_event&limit=100"
```

Payment metrics:

```bash
curl "http://localhost:8080/metrics/payment"
```
