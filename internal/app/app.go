package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stripe/stripe-go/v79"
	checkoutsession "github.com/stripe/stripe-go/v79/checkout/session"
	"github.com/stripe/stripe-go/v79/webhook"
)

type Config struct {
	HTTPAddr               string
	PostgresDSN            string
	StripeSecretKey        string
	StripeWebhookSecret    string
	CheckoutPublicBaseURL  string
	USDCReceiveAddress     string
	USDCReceiveChain       string
	USDCWebhookBearerToken string
	USDCWebhookHMACSecret  string
	USDCMinConfirmations   int
	AlertWebhookURL        string
	AlertWebhookBearer     string
	CORSAllowOrigins       string
	OneAPITopupURL         string
	OneAPITopupBearer      string
	OneAPITimeoutSeconds   int
	OneAPIRetrySeconds     int
}

func LoadConfigFromEnv() Config {
	return Config{
		HTTPAddr:               getEnv("HTTP_ADDR", ":8080"),
		PostgresDSN:            getEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/epay?sslmode=disable"),
		StripeSecretKey:        os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:    os.Getenv("STRIPE_WEBHOOK_SECRET"),
		CheckoutPublicBaseURL:  strings.TrimSpace(os.Getenv("PUBLIC_CHECKOUT_BASE_URL")),
		USDCReceiveAddress:     strings.TrimSpace(os.Getenv("USDC_RECEIVE_ADDRESS")),
		USDCReceiveChain:       getEnv("USDC_RECEIVE_CHAIN", "ethereum"),
		USDCWebhookBearerToken: os.Getenv("USDC_WEBHOOK_BEARER"),
		USDCWebhookHMACSecret:  os.Getenv("USDC_WEBHOOK_HMAC_SECRET"),
		USDCMinConfirmations:   getEnvInt("USDC_MIN_CONFIRMATIONS", 1),
		AlertWebhookURL:        os.Getenv("ALERT_WEBHOOK_URL"),
		AlertWebhookBearer:     os.Getenv("ALERT_WEBHOOK_BEARER"),
		CORSAllowOrigins:       os.Getenv("CORS_ALLOW_ORIGINS"),
		OneAPITopupURL:         os.Getenv("ONEAPI_TOPUP_URL"),
		OneAPITopupBearer:      os.Getenv("ONEAPI_TOPUP_BEARER"),
		OneAPITimeoutSeconds:   getEnvInt("ONEAPI_TIMEOUT_SECONDS", 10),
		OneAPIRetrySeconds:     getEnvInt("ONEAPI_RETRY_SECONDS", 30),
	}
}

type App struct {
	db         *sql.DB
	mux        *http.ServeMux
	cfg        Config
	alertMu    sync.Mutex
	alertCache map[string]time.Time
}

type ReconcileSummary struct {
	Since          time.Time `json:"since"`
	TotalOrders    int       `json:"total_orders"`
	PaidOrCredited int       `json:"paid_or_credited"`
	CreditedOrders int       `json:"credited_orders"`
	LedgerTopups   int       `json:"ledger_topups"`
	UncreditedPaid int       `json:"uncredited_paid"`
	OrdersNoEvent  int       `json:"orders_no_event"`
	CreatedAt      time.Time `json:"created_at"`
}

type ReconcileItem struct {
	OrderNo    string    `json:"order_no"`
	CustomerID string    `json:"customer_id"`
	Status     string    `json:"status"`
	Credited   bool      `json:"credited"`
	Provider   string    `json:"provider"`
	Amount     float64   `json:"amount"`
	Currency   string    `json:"currency"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func New(cfg Config) (*App, error) {
	db, err := sql.Open("pgx", cfg.PostgresDSN)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}

	app := &App{
		db:         db,
		mux:        http.NewServeMux(),
		cfg:        cfg,
		alertCache: map[string]time.Time{},
	}
	app.routes()
	return app, nil
}

func (a *App) Router() http.Handler { return a.mux }

// Close releases database resources.
func (a *App) Close() error { return a.db.Close() }

type createOrderReq struct {
	CustomerID    string                 `json:"customer_id"`
	PaymentMethod string                 `json:"payment_method"`
	Provider      string                 `json:"provider"`
	Currency      string                 `json:"currency"`
	Amount        float64                `json:"amount"`
	Metadata      map[string]interface{} `json:"metadata"`
}

type usdcEventReq struct {
	OrderNo               string  `json:"order_no"`
	Chain                 string  `json:"chain"`
	TxHash                string  `json:"tx_hash"`
	ToAddress             string  `json:"to_address"`
	Token                 string  `json:"token"`
	Amount                float64 `json:"amount"`
	Currency              string  `json:"currency"`
	Confirmations         int     `json:"confirmations"`
	RequiredConfirmations int     `json:"required_confirmations"`
}

type bindOneAPIReq struct {
	CustomerID  string `json:"customer_id"`
	OneAPIUserID string `json:"oneapi_user_id"`
}

func (a *App) routes() {
	a.mux.HandleFunc("POST /payments/orders", a.handleCreateOrder)
	a.mux.HandleFunc("POST /payments/checkout/stripe", a.handleCheckoutStripe)
	a.mux.HandleFunc("POST /payments/checkout/crypto", a.handleCheckoutCrypto)
	a.mux.HandleFunc("GET /payments/orders", a.handleListOrders)
	a.mux.HandleFunc("GET /payments/orders/{orderNo}", a.handleGetOrder)
	a.mux.HandleFunc("POST /payments/orders/{orderNo}/retry-credit", a.handleRetryCredit)
	a.mux.HandleFunc("POST /payments/webhook/stripe", a.handleStripeWebhook)
	a.mux.HandleFunc("POST /payments/webhook/usdc", a.handleUSDCWebhook)
	a.mux.HandleFunc("GET /ops/wallets", a.handleListWallets)
	a.mux.HandleFunc("POST /ops/retry-failed", a.handleRetryFailedCredits)
	a.mux.HandleFunc("GET /ops/reconcile/summary", a.handleReconcileSummary)
	a.mux.HandleFunc("GET /ops/reconcile/details", a.handleReconcileDetails)
	a.mux.HandleFunc("GET /metrics/payment", a.handlePaymentMetrics)
	a.mux.HandleFunc("POST /ops/oneapi/bind", a.handleBindOneAPIUser)
	a.mux.HandleFunc("GET /ops/oneapi/bind/{customerID}", a.handleGetOneAPIBinding)
	a.mux.HandleFunc("POST /ops/oneapi/retry", a.handleRetryOneAPICredits)
	a.mux.HandleFunc("GET /ops/oneapi/jobs", a.handleListOneAPIJobs)
}

func (a *App) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	var req createOrderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.CustomerID == "" || req.Amount <= 0 || req.Provider == "" || req.Currency == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing required fields"})
		return
	}
	if req.PaymentMethod != "fiat" && req.PaymentMethod != "crypto" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payment_method must be fiat or crypto"})
		return
	}

	orderNo := "ord_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	metadataJSON, _ := json.Marshal(req.Metadata)
	if metadataJSON == nil {
		metadataJSON = []byte("{}")
	}

	_, err := a.db.ExecContext(r.Context(), `
		INSERT INTO payment_order (id, order_no, customer_id, payment_method, provider, currency, amount, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
	`, uuid.New(), orderNo, req.CustomerID, req.PaymentMethod, req.Provider, strings.ToUpper(req.Currency), req.Amount, string(metadataJSON))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create order failed"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"order_no": orderNo, "status": "pending"})
}

type checkoutStripeReq struct {
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
}

func (a *App) handleCheckoutStripe(w http.ResponseWriter, r *http.Request) {
	var req checkoutStripeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.CustomerID = strings.TrimSpace(req.CustomerID)
	if req.CustomerID == "" || req.Amount <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "customer_id and positive amount required"})
		return
	}
	currency := strings.TrimSpace(strings.ToUpper(req.Currency))
	if currency == "" {
		currency = "USD"
	}
	if strings.TrimSpace(a.cfg.StripeSecretKey) == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "STRIPE_SECRET_KEY not configured"})
		return
	}
	base := strings.TrimRight(strings.TrimSpace(a.cfg.CheckoutPublicBaseURL), "/")
	if base == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PUBLIC_CHECKOUT_BASE_URL not configured"})
		return
	}
	amountCents := int64(math.Round(req.Amount * 100))
	if amountCents < 50 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "amount too small for card checkout (min 0.50 in major units)"})
		return
	}

	orderNo := "ord_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	meta := map[string]interface{}{"checkout": "stripe"}
	metaJSON, _ := json.Marshal(meta)
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}
	_, err := a.db.ExecContext(r.Context(), `
		INSERT INTO payment_order (id, order_no, customer_id, payment_method, provider, currency, amount, metadata)
		VALUES ($1, $2, $3, 'fiat', 'stripe', $4, $5, $6::jsonb)
	`, uuid.New(), orderNo, req.CustomerID, currency, req.Amount, string(metaJSON))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create order failed"})
		return
	}

	stripe.Key = a.cfg.StripeSecretKey
	successURL := fmt.Sprintf("%s/#/checkout/success?order_no=%s", base, orderNo)
	cancelURL := fmt.Sprintf("%s/#/checkout", base)
	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
		Metadata: map[string]string{
			"order_no": orderNo,
		},
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Quantity: stripe.Int64(1),
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   stripe.String(strings.ToLower(currency)),
					UnitAmount: stripe.Int64(amountCents),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String("Account top-up"),
					},
				},
			},
		},
	}
	sess, err := checkoutsession.New(params)
	if err != nil {
		log.Printf("stripe checkout session failed: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "stripe checkout failed"})
		return
	}
	if sess.URL == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "stripe returned empty checkout url"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"order_no": orderNo, "checkout_url": sess.URL, "status": "pending"})
}

type checkoutCryptoReq struct {
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
}

func (a *App) handleCheckoutCrypto(w http.ResponseWriter, r *http.Request) {
	var req checkoutCryptoReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.CustomerID = strings.TrimSpace(req.CustomerID)
	if req.CustomerID == "" || req.Amount <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "customer_id and positive amount required"})
		return
	}
	addr := strings.TrimSpace(a.cfg.USDCReceiveAddress)
	if addr == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "USDC_RECEIVE_ADDRESS not configured"})
		return
	}
	currency := strings.TrimSpace(strings.ToUpper(req.Currency))
	if currency == "" {
		currency = "USD"
	}
	chain := strings.TrimSpace(a.cfg.USDCReceiveChain)
	if chain == "" {
		chain = "ethereum"
	}

	orderNo := "ord_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	meta := map[string]interface{}{
		"checkout":        "crypto",
		"deposit_token":   "USDC",
		"deposit_chain":   chain,
		"deposit_address": addr,
	}
	metaJSON, _ := json.Marshal(meta)
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}
	_, err := a.db.ExecContext(r.Context(), `
		INSERT INTO payment_order (id, order_no, customer_id, payment_method, provider, currency, amount, metadata)
		VALUES ($1, $2, $3, 'crypto', 'usdc', $4, $5, $6::jsonb)
	`, uuid.New(), orderNo, req.CustomerID, currency, req.Amount, string(metaJSON))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create order failed"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"order_no":     orderNo,
		"status":       "pending",
		"amount":       req.Amount,
		"currency":     currency,
		"token":        "USDC",
		"chain":        chain,
		"to_address":   addr,
		"instructions": "Send the exact USDC amount on the configured network to the address above. Your payment will be confirmed after your indexer calls the USDC webhook with tx details.",
	})
}

func (a *App) handleListOrders(w http.ResponseWriter, r *http.Request) {
	page := 1
	limit := 20
	if raw := r.URL.Query().Get("page"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			page = v
		}
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}
	offset := (page - 1) * limit

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	var (
		rows *sql.Rows
		err  error
	)
	if statusFilter != "" {
		rows, err = a.db.QueryContext(r.Context(), `
			SELECT order_no, customer_id, payment_method, provider, currency, amount, status, credited, created_at, updated_at
			FROM payment_order WHERE status = $1
			ORDER BY created_at DESC LIMIT $2 OFFSET $3
		`, statusFilter, limit, offset)
	} else {
		rows, err = a.db.QueryContext(r.Context(), `
			SELECT order_no, customer_id, payment_method, provider, currency, amount, status, credited, created_at, updated_at
			FROM payment_order
			ORDER BY created_at DESC LIMIT $1 OFFSET $2
		`, limit, offset)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type row struct {
		OrderNo       string    `json:"order_no"`
		CustomerID    string    `json:"customer_id"`
		PaymentMethod string    `json:"payment_method"`
		Provider      string    `json:"provider"`
		Currency      string    `json:"currency"`
		Amount        float64   `json:"amount"`
		Status        string    `json:"status"`
		Credited      bool      `json:"credited"`
		CreatedAt     time.Time `json:"created_at"`
		UpdatedAt     time.Time `json:"updated_at"`
	}
	var list []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.OrderNo, &item.CustomerID, &item.PaymentMethod, &item.Provider, &item.Currency, &item.Amount, &item.Status, &item.Credited, &item.CreatedAt, &item.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		list = append(list, item)
	}

	var total int
	if statusFilter != "" {
		err = a.db.QueryRowContext(r.Context(), `SELECT COUNT(1) FROM payment_order WHERE status = $1`, statusFilter).Scan(&total)
	} else {
		err = a.db.QueryRowContext(r.Context(), `SELECT COUNT(1) FROM payment_order`).Scan(&total)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "count failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": list,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (a *App) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	rawBody, err := readBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body failed"})
		return
	}

	event, err := a.parseStripeEvent(rawBody, r.Header.Get("Stripe-Signature"))
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	orderNo, providerTxnID, amount, currency, shouldProcess, err := extractStripePaymentResult(event)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := a.handleStripeNonSuccessEvent(r.Context(), event, rawBody); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !shouldProcess {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	eventKey := "stripe:" + providerTxnID
	if err := a.markPaidAndCredit(r.Context(), orderNo, eventKey, "stripe", rawBody, amount, currency); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleStripeNonSuccessEvent(ctx context.Context, event stripe.Event, rawBody []byte) error {
	switch event.Type {
	case "invoice.payment_failed":
		orderNo, reason, err := extractStripeFailure(event)
		if err != nil {
			return err
		}
		if orderNo == "" {
			return nil
		}
		if err := a.markOrderFailed(ctx, orderNo, "stripe_failed:"+event.ID, "stripe", rawBody, reason); err != nil {
			return err
		}
		a.sendAlert(ctx, "stripe_payment_failed", map[string]interface{}{
			"order_no": orderNo,
			"reason":   reason,
			"event_id": event.ID,
		})
	case "charge.refunded":
		orderNo, amount, currency, err := extractStripeRefund(event)
		if err != nil {
			return err
		}
		if orderNo == "" {
			return nil
		}
		_, _ = a.db.ExecContext(ctx, `
			INSERT INTO payment_event (id, order_no, event_key, source, payload)
			VALUES ($1, $2, $3, $4, $5::jsonb)
			ON CONFLICT (event_key) DO NOTHING
		`, uuid.New(), orderNo, "stripe_refund:"+event.ID, "stripe_refund", string(rawBody))
		a.sendAlert(ctx, "stripe_refund", map[string]interface{}{
			"order_no": orderNo,
			"amount":   amount,
			"currency": currency,
			"event_id": event.ID,
		})
	}
	return nil
}

func (a *App) handleUSDCWebhook(w http.ResponseWriter, r *http.Request) {
	if a.cfg.USDCWebhookBearerToken != "" && r.Header.Get("Authorization") != "Bearer "+a.cfg.USDCWebhookBearerToken {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	rawBody, err := readBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body failed"})
		return
	}

	var req usdcEventReq
	if err := json.Unmarshal(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json payload"})
		return
	}
	if a.cfg.USDCWebhookHMACSecret != "" {
		if !verifyUSDCHMAC(rawBody, r.Header.Get("X-USDC-Signature"), a.cfg.USDCWebhookHMACSecret) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid usdc signature"})
			return
		}
	}

	token := strings.ToUpper(req.Token)
	if token != "USDC" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only USDC accepted"})
		return
	}
	requiredConfs := req.RequiredConfirmations
	if requiredConfs <= 0 {
		requiredConfs = a.cfg.USDCMinConfirmations
	}
	if req.Confirmations < requiredConfs {
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":                 "waiting_confirmations",
			"confirmations":          req.Confirmations,
			"required_confirmations": requiredConfs,
		})
		return
	}
	if req.TxHash == "" || req.OrderNo == "" || req.Chain == "" || req.ToAddress == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing chain tx/order fields"})
		return
	}

	eventKey := fmt.Sprintf("usdc:%s:%s:%s:%s", strings.ToLower(req.Chain), strings.ToLower(req.TxHash), token, strings.ToLower(req.ToAddress))
	if err := a.markPaidAndCredit(r.Context(), req.OrderNo, eventKey, "usdc", rawBody, req.Amount, req.Currency); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	orderNo := r.PathValue("orderNo")
	if orderNo == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid order"})
		return
	}

	row := a.db.QueryRowContext(r.Context(), `
		SELECT order_no, customer_id, payment_method, provider, currency, amount, status, credited, created_at, updated_at
		FROM payment_order WHERE order_no = $1
	`, orderNo)

	var out struct {
		OrderNo       string    `json:"order_no"`
		CustomerID    string    `json:"customer_id"`
		PaymentMethod string    `json:"payment_method"`
		Provider      string    `json:"provider"`
		Currency      string    `json:"currency"`
		Amount        float64   `json:"amount"`
		Status        string    `json:"status"`
		Credited      bool      `json:"credited"`
		CreatedAt     time.Time `json:"created_at"`
		UpdatedAt     time.Time `json:"updated_at"`
	}
	if err := row.Scan(&out.OrderNo, &out.CustomerID, &out.PaymentMethod, &out.Provider, &out.Currency, &out.Amount, &out.Status, &out.Credited, &out.CreatedAt, &out.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "order not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) handleRetryCredit(w http.ResponseWriter, r *http.Request) {
	orderNo := r.PathValue("orderNo")
	if orderNo == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid order"})
		return
	}
	payload := []byte(`{"manual_retry":true}`)
	err := a.markPaidAndCredit(r.Context(), orderNo, "manual_retry:"+orderNo+":"+uuid.NewString(), "manual_retry", payload, 0, "")
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "retried"})
}

func (a *App) handleListWallets(w http.ResponseWriter, r *http.Request) {
	page := 1
	limit := 20
	if raw := r.URL.Query().Get("page"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			page = v
		}
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}
	offset := (page - 1) * limit

	rows, err := a.db.QueryContext(r.Context(), `
		SELECT customer_id, currency, balance, updated_at
		FROM wallet_account
		ORDER BY updated_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type walletRow struct {
		CustomerID string    `json:"customer_id"`
		Currency   string    `json:"currency"`
		Balance    float64   `json:"balance"`
		UpdatedAt  time.Time `json:"updated_at"`
	}
	var list []walletRow
	for rows.Next() {
		var item walletRow
		if err := rows.Scan(&item.CustomerID, &item.Currency, &item.Balance, &item.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		list = append(list, item)
	}

	var total int
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(1) FROM wallet_account`).Scan(&total); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "count failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": list,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (a *App) handleRetryFailedCredits(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}
	retried, failed, err := a.RetryUncreditedOrders(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"retried": retried, "failed": failed})
}

func (a *App) handleReconcileSummary(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-24 * time.Hour)
	if raw := r.URL.Query().Get("since_hours"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			since = time.Now().Add(-time.Duration(v) * time.Hour)
		}
	}
	report, err := a.BuildReconcileSummary(r.Context(), since)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (a *App) handleReconcileDetails(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 2000 {
			limit = v
		}
	}
	mode := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = "uncredited_paid"
	}
	items, err := a.BuildReconcileDetails(r.Context(), mode, limit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"mode":  mode,
		"count": len(items),
		"items": items,
	})
}

func (a *App) handlePaymentMetrics(w http.ResponseWriter, r *http.Request) {
	report, err := a.BuildReconcileSummary(r.Context(), time.Now().Add(-24*time.Hour))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"window_hours":            24,
		"paid_or_credited_orders": report.PaidOrCredited,
		"credited_orders":         report.CreditedOrders,
		"uncredited_paid_orders":  report.UncreditedPaid,
	})
}

func (a *App) parseStripeEvent(rawBody []byte, sig string) (stripe.Event, error) {
	if a.cfg.StripeWebhookSecret != "" {
		event, err := webhook.ConstructEvent(rawBody, sig, a.cfg.StripeWebhookSecret)
		if err != nil {
			return stripe.Event{}, errors.New("invalid stripe signature")
		}
		return event, nil
	}
	var event stripe.Event
	if err := json.Unmarshal(rawBody, &event); err != nil {
		return stripe.Event{}, errors.New("invalid stripe event payload")
	}
	return event, nil
}

func extractStripePaymentResult(event stripe.Event) (orderNo, providerTxnID string, amount float64, currency string, shouldProcess bool, err error) {
	var obj map[string]interface{}
	if err = json.Unmarshal(event.Data.Raw, &obj); err != nil {
		return "", "", 0, "", false, errors.New("invalid stripe object")
	}
	metadata := nestedMap(obj, "metadata")
	orderNo = strVal(metadata["order_no"])
	if orderNo == "" {
		return "", "", 0, "", false, errors.New("stripe metadata.order_no required")
	}

	switch event.Type {
	case "payment_intent.succeeded":
		providerTxnID = strVal(obj["id"])
		amount = toMajorUnit(numVal(obj["amount_received"]))
		if amount <= 0 {
			amount = toMajorUnit(numVal(obj["amount"]))
		}
		currency = strings.ToUpper(strVal(obj["currency"]))
		return orderNo, providerTxnID, amount, currency, true, nil
	case "checkout.session.completed":
		providerTxnID = strVal(obj["payment_intent"])
		if providerTxnID == "" {
			providerTxnID = strVal(obj["id"])
		}
		amount = toMajorUnit(numVal(obj["amount_total"]))
		currency = strings.ToUpper(strVal(obj["currency"]))
		return orderNo, providerTxnID, amount, currency, true, nil
	case "invoice.payment_succeeded":
		providerTxnID = strVal(obj["payment_intent"])
		if providerTxnID == "" {
			providerTxnID = strVal(obj["id"])
		}
		amount = toMajorUnit(numVal(obj["amount_paid"]))
		currency = strings.ToUpper(strVal(obj["currency"]))
		return orderNo, providerTxnID, amount, currency, true, nil
	default:
		return "", "", 0, "", false, nil
	}
}

func (a *App) markPaidAndCredit(ctx context.Context, orderNo, eventKey, source string, payload []byte, eventAmount float64, eventCurrency string) error {
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var customerID, currency string
	var amount float64
	var credited bool
	err = tx.QueryRowContext(ctx, `
		SELECT customer_id, amount, currency, credited
		FROM payment_order WHERE order_no = $1 FOR UPDATE
	`, orderNo).Scan(&customerID, &amount, &currency, &credited)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO payment_event (id, order_no, event_key, source, payload)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		ON CONFLICT (event_key) DO NOTHING
	`, uuid.New(), orderNo, eventKey, source, string(payload))
	if err != nil {
		return fmt.Errorf("insert payment_event: %w", err)
	}
	if eventAmount > 0 && abs(amount-eventAmount) > 0.000001 {
		return errors.New("event amount mismatch")
	}
	if eventCurrency != "" && strings.ToUpper(eventCurrency) != strings.ToUpper(currency) {
		return errors.New("event currency mismatch")
	}
	if credited {
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `UPDATE payment_order SET status = 'paid', updated_at = NOW() WHERE order_no = $1`, orderNo); err != nil {
		return fmt.Errorf("mark paid: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO wallet_account (customer_id, currency, balance)
		VALUES ($1, $2, 0)
		ON CONFLICT (customer_id) DO NOTHING
	`, customerID, currency); err != nil {
		return fmt.Errorf("ensure wallet_account: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO wallet_ledger (id, customer_id, order_no, entry_type, amount, currency)
		VALUES ($1, $2, $3, 'topup', $4, $5)
		ON CONFLICT (order_no, entry_type) DO NOTHING
	`, uuid.New(), customerID, orderNo, amount, currency)
	if err != nil {
		return fmt.Errorf("insert wallet_ledger: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE payment_order SET status='credited', credited=TRUE, updated_at=NOW() WHERE order_no=$1`, orderNo); err != nil {
			return fmt.Errorf("sync credited state: %w", err)
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `UPDATE wallet_account SET balance = balance + $1, updated_at = NOW() WHERE customer_id = $2`, amount, customerID); err != nil {
		return fmt.Errorf("update wallet_account: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE payment_order SET status='credited', credited=TRUE, updated_at=NOW() WHERE order_no=$1`, orderNo); err != nil {
		return fmt.Errorf("update payment_order: %w", err)
	}
	if err := a.upsertOneAPICreditJobTx(ctx, tx, orderNo, customerID, amount, currency); err != nil {
		return fmt.Errorf("prepare oneapi credit job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := a.trySyncOneAPICreditJob(ctx, orderNo); err != nil {
		log.Printf("oneapi credit sync deferred for %s: %v", orderNo, err)
	}
	return nil
}

func (a *App) markOrderFailed(ctx context.Context, orderNo, eventKey, source string, payload []byte, reason string) error {
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var credited bool
	err = tx.QueryRowContext(ctx, `SELECT credited FROM payment_order WHERE order_no = $1 FOR UPDATE`, orderNo).Scan(&credited)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO payment_event (id, order_no, event_key, source, payload)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		ON CONFLICT (event_key) DO NOTHING
	`, uuid.New(), orderNo, eventKey, source, string(payload)); err != nil {
		return err
	}

	if !credited {
		if _, err := tx.ExecContext(ctx, `UPDATE payment_order SET status='failed', updated_at=NOW() WHERE order_no=$1`, orderNo); err != nil {
			return err
		}
	}
	if reason != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE payment_order
			SET metadata = metadata || jsonb_build_object('last_failure_reason', $2, 'last_failure_at', NOW()::text), updated_at = NOW()
			WHERE order_no = $1
		`, orderNo, reason); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *App) RetryUncreditedOrders(ctx context.Context, limit int) (retried int, failed int, err error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT po.order_no
		FROM payment_order po
		WHERE po.credited = FALSE
		  AND po.status IN ('pending', 'paid')
		  AND EXISTS (SELECT 1 FROM payment_event pe WHERE pe.order_no = po.order_no)
		ORDER BY po.updated_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	var orderNos []string
	for rows.Next() {
		var orderNo string
		if err := rows.Scan(&orderNo); err != nil {
			return retried, failed, err
		}
		orderNos = append(orderNos, orderNo)
	}
	for _, orderNo := range orderNos {
		payload := []byte(`{"auto_retry":true}`)
		err := a.markPaidAndCredit(ctx, orderNo, "auto_retry:"+orderNo+":"+uuid.NewString(), "auto_retry", payload, 0, "")
		if err != nil {
			failed++
			a.sendAlert(ctx, "retry_credit_failed", map[string]interface{}{
				"order_no": orderNo,
				"error":    err.Error(),
			})
			continue
		}
		retried++
	}
	return retried, failed, nil
}

func (a *App) RetryOneAPICredits(ctx context.Context, limit int) (synced int, failed int, err error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT order_no
		FROM oneapi_credit_job
		WHERE status <> 'synced'
		  AND next_retry_at <= NOW()
		ORDER BY next_retry_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	var orderNos []string
	for rows.Next() {
		var orderNo string
		if err := rows.Scan(&orderNo); err != nil {
			return synced, failed, err
		}
		orderNos = append(orderNos, orderNo)
	}

	for _, orderNo := range orderNos {
		if err := a.trySyncOneAPICreditJob(ctx, orderNo); err != nil {
			failed++
			a.sendAlert(ctx, "oneapi_credit_failed", map[string]interface{}{
				"order_no": orderNo,
				"error":    err.Error(),
			})
			continue
		}
		synced++
	}
	return synced, failed, nil
}

func (a *App) handleBindOneAPIUser(w http.ResponseWriter, r *http.Request) {
	var req bindOneAPIReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.CustomerID = strings.TrimSpace(req.CustomerID)
	req.OneAPIUserID = strings.TrimSpace(req.OneAPIUserID)
	if req.CustomerID == "" || req.OneAPIUserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "customer_id and oneapi_user_id required"})
		return
	}
	_, err := a.db.ExecContext(r.Context(), `
		INSERT INTO oneapi_customer_binding (customer_id, oneapi_user_id, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (customer_id) DO UPDATE SET oneapi_user_id=EXCLUDED.oneapi_user_id, updated_at=NOW()
	`, req.CustomerID, req.OneAPIUserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "bind failed"})
		return
	}
	if _, _, err := a.retryOneAPICreditsByCustomer(r.Context(), req.CustomerID, 50); err != nil {
		log.Printf("oneapi immediate retry after bind failed for %s: %v", req.CustomerID, err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "bound"})
}

func (a *App) handleGetOneAPIBinding(w http.ResponseWriter, r *http.Request) {
	customerID := strings.TrimSpace(r.PathValue("customerID"))
	if customerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid customerID"})
		return
	}
	var oneapiUserID string
	err := a.db.QueryRowContext(r.Context(), `SELECT oneapi_user_id FROM oneapi_customer_binding WHERE customer_id=$1`, customerID).Scan(&oneapiUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "binding not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"customer_id": customerID, "oneapi_user_id": oneapiUserID})
}

func (a *App) handleRetryOneAPICredits(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}
	synced, failed, err := a.RetryOneAPICredits(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"synced": synced, "failed": failed})
}

func (a *App) handleListOneAPIJobs(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}
	status := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("status")))
	query := `
		SELECT order_no, customer_id, COALESCE(oneapi_user_id, ''), amount, currency, status, retry_count, COALESCE(last_error, ''), next_retry_at, created_at, updated_at
		FROM oneapi_credit_job
	`
	args := []interface{}{}
	if status != "" {
		query += ` WHERE status = $1`
		args = append(args, status)
	}
	query += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT %d", limit)
	rows, err := a.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type item struct {
		OrderNo     string    `json:"order_no"`
		CustomerID  string    `json:"customer_id"`
		OneAPIUserID string   `json:"oneapi_user_id"`
		Amount      float64   `json:"amount"`
		Currency    string    `json:"currency"`
		Status      string    `json:"status"`
		RetryCount  int       `json:"retry_count"`
		LastError   string    `json:"last_error"`
		NextRetryAt time.Time `json:"next_retry_at"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
	}
	var list []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.OrderNo, &it.CustomerID, &it.OneAPIUserID, &it.Amount, &it.Currency, &it.Status, &it.RetryCount, &it.LastError, &it.NextRetryAt, &it.CreatedAt, &it.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": list, "count": len(list)})
}

func (a *App) upsertOneAPICreditJobTx(ctx context.Context, tx *sql.Tx, orderNo, customerID string, amount float64, currency string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO oneapi_credit_job (id, order_no, customer_id, amount, currency, status, next_retry_at)
		VALUES ($1, $2, $3, $4, $5, 'pending', NOW())
		ON CONFLICT (order_no) DO NOTHING
	`, uuid.New(), orderNo, customerID, amount, strings.ToUpper(currency)); err != nil {
		return err
	}
	return nil
}

func (a *App) retryOneAPICreditsByCustomer(ctx context.Context, customerID string, limit int) (synced int, failed int, err error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT order_no
		FROM oneapi_credit_job
		WHERE customer_id=$1 AND status <> 'synced'
		ORDER BY updated_at DESC
		LIMIT $2
	`, customerID, limit)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	var orderNos []string
	for rows.Next() {
		var orderNo string
		if err := rows.Scan(&orderNo); err != nil {
			return synced, failed, err
		}
		orderNos = append(orderNos, orderNo)
	}
	for _, orderNo := range orderNos {
		if err := a.trySyncOneAPICreditJob(ctx, orderNo); err != nil {
			failed++
			continue
		}
		synced++
	}
	return synced, failed, nil
}

func (a *App) trySyncOneAPICreditJob(ctx context.Context, orderNo string) error {
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var job struct {
		OrderNo      string
		CustomerID   string
		OneAPIUserID string
		Amount       float64
		Currency     string
		Status       string
		RetryCount   int
	}
	err = tx.QueryRowContext(ctx, `
		SELECT order_no, customer_id, COALESCE(oneapi_user_id, ''), amount, currency, status, retry_count
		FROM oneapi_credit_job
		WHERE order_no = $1
		FOR UPDATE
	`, orderNo).Scan(&job.OrderNo, &job.CustomerID, &job.OneAPIUserID, &job.Amount, &job.Currency, &job.Status, &job.RetryCount)
	if err != nil {
		return err
	}
	if job.Status == "synced" {
		return tx.Commit()
	}
	if job.OneAPIUserID == "" {
		_ = tx.QueryRowContext(ctx, `
			SELECT oneapi_user_id FROM oneapi_customer_binding WHERE customer_id = $1
		`, job.CustomerID).Scan(&job.OneAPIUserID)
	}
	if strings.TrimSpace(job.OneAPIUserID) == "" {
		backoff := time.Duration(maxInt(1, a.cfg.OneAPIRetrySeconds)) * time.Second
		_, _ = tx.ExecContext(ctx, `
			UPDATE oneapi_credit_job
			SET status='pending', retry_count=retry_count+1, last_error=$2, next_retry_at=NOW()+$3::interval, updated_at=NOW()
			WHERE order_no=$1
		`, orderNo, "missing oneapi user binding", fmt.Sprintf("%d seconds", int(backoff.Seconds())))
		return tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `UPDATE oneapi_credit_job SET oneapi_user_id=$2, updated_at=NOW() WHERE order_no=$1`, orderNo, job.OneAPIUserID)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if err := a.callOneAPITopup(ctx, job.OrderNo, job.CustomerID, job.OneAPIUserID, job.Amount, job.Currency); err != nil {
		backoffSeconds := maxInt(a.cfg.OneAPIRetrySeconds, 30)
		_, _ = a.db.ExecContext(ctx, `
			UPDATE oneapi_credit_job
			SET status='failed', retry_count=retry_count+1, last_error=$2, next_retry_at=NOW()+$3::interval, updated_at=NOW()
			WHERE order_no=$1
		`, orderNo, err.Error(), fmt.Sprintf("%d seconds", backoffSeconds))
		return err
	}
	_, err = a.db.ExecContext(ctx, `
		UPDATE oneapi_credit_job
		SET status='synced', last_error=NULL, synced_at=NOW(), updated_at=NOW()
		WHERE order_no=$1
	`, orderNo)
	return err
}

func (a *App) callOneAPITopup(ctx context.Context, orderNo, customerID, oneapiUserID string, amount float64, currency string) error {
	if strings.TrimSpace(a.cfg.OneAPITopupURL) == "" {
		return errors.New("ONEAPI_TOPUP_URL is empty")
	}
	payload := map[string]interface{}{
		"order_no":       orderNo,
		"customer_id":    customerID,
		"oneapi_user_id": oneapiUserID,
		"amount":         amount,
		"currency":       strings.ToUpper(currency),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	timeout := time.Duration(maxInt(a.cfg.OneAPITimeoutSeconds, 1)) * time.Second
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(cctx, http.MethodPost, a.cfg.OneAPITopupURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(a.cfg.OneAPITopupBearer) != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.OneAPITopupBearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("oneapi topup non-2xx: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (a *App) BuildReconcileSummary(ctx context.Context, since time.Time) (*ReconcileSummary, error) {
	report := &ReconcileSummary{Since: since.UTC(), CreatedAt: time.Now().UTC()}
	queries := []struct {
		target *int
		sql    string
	}{
		{&report.TotalOrders, `SELECT COUNT(1) FROM payment_order WHERE created_at >= $1`},
		{&report.PaidOrCredited, `SELECT COUNT(1) FROM payment_order WHERE created_at >= $1 AND status IN ('paid','credited')`},
		{&report.CreditedOrders, `SELECT COUNT(1) FROM payment_order WHERE created_at >= $1 AND credited=TRUE`},
		{&report.LedgerTopups, `SELECT COUNT(1) FROM wallet_ledger WHERE created_at >= $1 AND entry_type='topup'`},
		{&report.UncreditedPaid, `SELECT COUNT(1) FROM payment_order WHERE created_at >= $1 AND status='paid' AND credited=FALSE`},
		{&report.OrdersNoEvent, `SELECT COUNT(1) FROM payment_order po WHERE po.created_at >= $1 AND po.status IN ('paid','credited') AND NOT EXISTS (SELECT 1 FROM payment_event pe WHERE pe.order_no = po.order_no)`},
	}
	for _, q := range queries {
		if err := a.db.QueryRowContext(ctx, q.sql, since).Scan(q.target); err != nil {
			return nil, err
		}
	}
	return report, nil
}

func (a *App) BuildReconcileDetails(ctx context.Context, mode string, limit int) ([]ReconcileItem, error) {
	var query string
	switch mode {
	case "uncredited_paid":
		query = `
			SELECT order_no, customer_id, status, credited, provider, amount, currency, updated_at
			FROM payment_order
			WHERE status='paid' AND credited=FALSE
			ORDER BY updated_at DESC
			LIMIT $1
		`
	case "orders_no_event":
		query = `
			SELECT po.order_no, po.customer_id, po.status, po.credited, po.provider, po.amount, po.currency, po.updated_at
			FROM payment_order po
			WHERE po.status IN ('paid','credited')
			  AND NOT EXISTS (SELECT 1 FROM payment_event pe WHERE pe.order_no = po.order_no)
			ORDER BY po.updated_at DESC
			LIMIT $1
		`
	default:
		return nil, errors.New("mode must be uncredited_paid or orders_no_event")
	}

	rows, err := a.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ReconcileItem
	for rows.Next() {
		var item ReconcileItem
		if err := rows.Scan(&item.OrderNo, &item.CustomerID, &item.Status, &item.Credited, &item.Provider, &item.Amount, &item.Currency, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, 1<<20))
}

func verifyUSDCHMAC(body []byte, gotSig string, secret string) bool {
	gotSig = strings.TrimSpace(strings.ToLower(gotSig))
	if gotSig == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(gotSig))
}

func (a *App) sendAlert(ctx context.Context, event string, payload map[string]interface{}) {
	if a.cfg.AlertWebhookURL == "" {
		return
	}
	alertKey := event + ":" + strVal(payload["order_no"])
	if !a.allowAlert(alertKey, 2*time.Minute) {
		return
	}
	body := map[string]interface{}{
		"event":      event,
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"payload":    payload,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.AlertWebhookURL, strings.NewReader(string(data)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if a.cfg.AlertWebhookBearer != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.AlertWebhookBearer)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("alert webhook failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("alert webhook non-2xx: %d", resp.StatusCode)
	}
}

func (a *App) allowAlert(key string, cooldown time.Duration) bool {
	a.alertMu.Lock()
	defer a.alertMu.Unlock()

	last, ok := a.alertCache[key]
	now := time.Now()
	if ok && now.Sub(last) < cooldown {
		return false
	}
	a.alertCache[key] = now
	return true
}

func extractStripeFailure(event stripe.Event) (orderNo string, reason string, err error) {
	var obj map[string]interface{}
	if err = json.Unmarshal(event.Data.Raw, &obj); err != nil {
		return "", "", errors.New("invalid stripe object")
	}
	metadata := nestedMap(obj, "metadata")
	orderNo = strVal(metadata["order_no"])
	reason = strVal(obj["billing_reason"])
	if reason == "" {
		reason = "payment_failed"
	}
	return orderNo, reason, nil
}

func extractStripeRefund(event stripe.Event) (orderNo string, amount float64, currency string, err error) {
	var obj map[string]interface{}
	if err = json.Unmarshal(event.Data.Raw, &obj); err != nil {
		return "", 0, "", errors.New("invalid stripe object")
	}
	metadata := nestedMap(obj, "metadata")
	orderNo = strVal(metadata["order_no"])
	amount = toMajorUnit(numVal(obj["amount_refunded"]))
	if amount <= 0 {
		amount = toMajorUnit(numVal(obj["amount"]))
	}
	currency = strings.ToUpper(strVal(obj["currency"]))
	return orderNo, amount, currency, nil
}

func toMajorUnit(minor float64) float64 {
	return minor / 100
}

func nestedMap(source map[string]interface{}, key string) map[string]interface{} {
	v, ok := source[key]
	if !ok {
		return map[string]interface{}{}
	}
	out, ok := v.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return out
}

func strVal(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

func numVal(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int64:
		return float64(t)
	case int:
		return float64(t)
	default:
		return 0
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	out, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return out
}
