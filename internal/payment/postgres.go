package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/outbox"
)

type PostgresRepository struct {
	db       *pgxpool.Pool
	ledger   *ledger.Service
	orderSvc *order.Service
	outbox   *outbox.Writer
}

func initialMethodState(method string) string {
	switch method {
	case "upi":
		return MethodStateUPIIntentCreated
	case "netbanking":
		return MethodStateNetbankingRedirectCreated
	case "wallet":
		return MethodStateWalletRedirectCreated
	default:
		return MethodStateCardAuthorizationStarted
	}
}

func methodStateForAuthorized(method string) string {
	switch method {
	case "upi":
		return MethodStateUPICollected
	case "netbanking":
		return MethodStateNetbankingSucceeded
	case "wallet":
		return MethodStateWalletSucceeded
	default:
		return MethodStateCardAuthorized
	}
}

func methodStateForFailure(method string) string {
	switch method {
	case "upi":
		return MethodStateUPIFailed
	case "netbanking":
		return MethodStateNetbankingFailed
	case "wallet":
		return MethodStateWalletFailed
	default:
		return MethodStateCardDeclined
	}
}

func methodStateForChallengeFailure(status string) string {
	switch status {
	case "abandoned":
		return MethodStateCard3DSAbandoned
	case "expired":
		return MethodStateCard3DSExpired
	default:
		return MethodStateCard3DSFailed
	}
}

func methodStateForCapture(method string) string {
	switch method {
	case "upi":
		return MethodStateUPICollected
	case "netbanking":
		return MethodStateNetbankingSucceeded
	case "wallet":
		return MethodStateWalletSucceeded
	default:
		return MethodStateCardCaptured
	}
}

func NewPostgresRepository(db *pgxpool.Pool, ledgerSvc *ledger.Service, orderSvc *order.Service) *PostgresRepository {
	return &PostgresRepository{db: db, ledger: ledgerSvc, orderSvc: orderSvc, outbox: outbox.NewWriter()}
}

func (r *PostgresRepository) CreateFailedAttempt(ctx context.Context, in CreateAuthorizedInput, errorCode, errorDescription string) error {
	attemptID := idgen.New("attempt")
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payments.payment_attempts
(id, order_id, merchant_id, payment_id, amount, currency, method, provider, routing_reason, attempted_providers, status, error_code, error_description, idempotency_key)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'failed',$11,$12,$13)
`, attemptID, in.OrderID, in.MerchantID, nullableText(in.PaymentID), in.Amount, in.Currency, in.Method, nonEmptyText(in.Provider), nonEmptyText(in.RoutingReason), normalizeAttemptedProviders(in.AttemptedProviders), errorCode, errorDescription, in.IdempotencyKey)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_orders.orders
SET status = CASE WHEN status = 'created' THEN 'attempted' ELSE status END,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, in.OrderID, in.MerchantID); err != nil {
		return fmt.Errorf("update order status on failed attempt: %w", err)
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment_attempt", AggregateID: attemptID, EventType: "payment.failed", MerchantID: in.MerchantID, Payload: map[string]any{"order_id": in.OrderID, "error_code": errorCode}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) StartAuthorization(ctx context.Context, in CreateAuthorizedInput) (CaptureResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CaptureResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if in.Amount <= 0 {
		return CaptureResult{}, ErrAmountMismatch
	}

	var orderStatus string
	var orderAmount int64
	var orderAmountDue int64
	var orderCurrency string
	var partialPayment bool
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `
SELECT status, amount, amount_due, currency, partial_payment, expires_at
FROM paygate_orders.orders
WHERE id = $1 AND merchant_id = $2
FOR UPDATE
`, in.OrderID, in.MerchantID).Scan(&orderStatus, &orderAmount, &orderAmountDue, &orderCurrency, &partialPayment, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrOrderNotFound
		}
		return CaptureResult{}, fmt.Errorf("lock order: %w", err)
	}
	if orderStatus == "expired" || time.Now().UTC().After(expiresAt) {
		return CaptureResult{}, ErrOrderExpired
	}
	if orderStatus == "paid" {
		return CaptureResult{}, ErrInvalidTransition
	}
	if orderStatus == "failed" {
		return CaptureResult{}, ErrInvalidTransition
	}
	if in.Currency != orderCurrency {
		return CaptureResult{}, ErrCurrencyMismatch
	}
	if partialPayment {
		if in.Amount <= 0 || in.Amount > orderAmountDue {
			return CaptureResult{}, ErrAmountMismatch
		}
	} else if in.Amount != orderAmount {
		return CaptureResult{}, ErrAmountMismatch
	}

	attemptID := idgen.New("attempt")
	paymentID := in.PaymentID
	if paymentID == "" {
		paymentID = idgen.New("pay")
	}
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payments.payment_attempts
(id, order_id, merchant_id, payment_id, amount, currency, method, provider, routing_reason, attempted_providers, status, idempotency_key)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'processing',$11)
`, attemptID, in.OrderID, in.MerchantID, paymentID, in.Amount, in.Currency, in.Method, nonEmptyText(in.Provider), nonEmptyText(in.RoutingReason), normalizeAttemptedProviders(in.AttemptedProviders), in.IdempotencyKey)
	if err != nil {
		var pgErr *pgconn.PgError
		if in.IdempotencyKey != "" && errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return r.getPaymentByIdempotencyKey(ctx, in.MerchantID, in.OrderID, in.IdempotencyKey)
		}
		return CaptureResult{}, fmt.Errorf("insert payment attempt: %w", err)
	}

	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payments.payments
(id, attempt_id, order_id, merchant_id, amount, currency, method, provider, routing_reason, attempted_providers, method_state, status, captured)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'created',false)
`, paymentID, attemptID, in.OrderID, in.MerchantID, in.Amount, in.Currency, in.Method, nonEmptyText(in.Provider), nonEmptyText(in.RoutingReason), normalizeAttemptedProviders(in.AttemptedProviders), initialMethodState(in.Method))
	if err != nil {
		return CaptureResult{}, fmt.Errorf("insert payment: %w", err)
	}
	if err := r.insertPaymentSplitsTx(ctx, tx, in.MerchantID, paymentID, in.Splits); err != nil {
		return CaptureResult{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_orders.orders
SET status = CASE WHEN status = 'created' THEN 'attempted' ELSE status END,
    updated_at = NOW()
WHERE id=$1 AND merchant_id=$2
`, in.OrderID, in.MerchantID); err != nil {
		return CaptureResult{}, fmt.Errorf("update order status on authorization start: %w", err)
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment", AggregateID: paymentID, EventType: "payment.authorized", MerchantID: in.MerchantID, Payload: map[string]any{"payment_id": paymentID, "order_id": in.OrderID}}); err != nil {
		return CaptureResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CaptureResult{}, err
	}

	return CaptureResult{PaymentID: paymentID, MerchantID: in.MerchantID, OrderID: in.OrderID, Amount: in.Amount, Currency: in.Currency, Method: in.Method, Provider: in.Provider, RoutingReason: in.RoutingReason, AttemptedProviders: in.AttemptedProviders, MethodState: initialMethodState(in.Method), Status: StateCreated, Captured: false, CreatedAt: now}, nil
}

func (r *PostgresRepository) UpdateGatewayRouting(ctx context.Context, merchantID, paymentID string, decision GatewayRouteDecision) error {
	_, err := r.db.Exec(ctx, `
UPDATE paygate_payments.payments
SET provider = NULLIF($3, ''),
    routing_reason = NULLIF($4, ''),
    attempted_providers = $5,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, paymentID, merchantID, decision.Provider, decision.RoutingReason, normalizeAttemptedProviders(decision.AttemptedProviders))
	if err != nil {
		return fmt.Errorf("update payment routing: %w", err)
	}
	_, err = r.db.Exec(ctx, `
UPDATE paygate_payments.payment_attempts
SET provider = NULLIF($3, ''),
    routing_reason = NULLIF($4, ''),
    attempted_providers = $5,
    updated_at = NOW()
WHERE payment_id = $1 AND merchant_id = $2
`, paymentID, merchantID, decision.Provider, decision.RoutingReason, normalizeAttemptedProviders(decision.AttemptedProviders))
	if err != nil {
		return fmt.Errorf("update attempt routing: %w", err)
	}
	return nil
}

func (r *PostgresRepository) AttachCardPaymentDetails(ctx context.Context, merchantID, paymentID string, in CardPaymentDetailsInput) error {
	_, err := r.db.Exec(ctx, `
INSERT INTO paygate_payments.card_payment_details
(payment_id, card_token_id, brand, last4, exp_month, exp_year, token_class, issuer_name, issuer_country, card_country, funding_type, network_token_type)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (payment_id) DO UPDATE
SET card_token_id = EXCLUDED.card_token_id,
    brand = EXCLUDED.brand,
    last4 = EXCLUDED.last4,
    exp_month = EXCLUDED.exp_month,
    exp_year = EXCLUDED.exp_year,
    token_class = EXCLUDED.token_class,
    issuer_name = EXCLUDED.issuer_name,
    issuer_country = EXCLUDED.issuer_country,
    card_country = EXCLUDED.card_country,
    funding_type = EXCLUDED.funding_type,
    network_token_type = EXCLUDED.network_token_type
`, paymentID, in.PaymentMethodTokenID, in.CardBrand, in.CardLast4, in.CardExpMonth, in.CardExpYear, in.CardTokenClass, in.CardIssuerName, in.CardIssuerCountry, in.CardCountry, in.FundingType, in.NetworkTokenType)
	if err != nil {
		return fmt.Errorf("attach card payment details: %w", err)
	}
	return nil
}

func (r *PostgresRepository) MarkAuthorizationRequiresAction(ctx context.Context, merchantID, paymentID string, in CardChallengeSessionInput) (CaptureResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CaptureResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var attemptID, orderID, currentStatus string
	err = tx.QueryRow(ctx, `
SELECT attempt_id, order_id, status
FROM paygate_payments.payments
WHERE id = $1 AND merchant_id = $2
FOR UPDATE
`, paymentID, merchantID).Scan(&attemptID, &orderID, &currentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrPaymentNotFound
		}
		return CaptureResult{}, err
	}
	switch PaymentState(currentStatus) {
	case StateRequiresAction:
		return r.GetPayment(ctx, merchantID, paymentID)
	case StateCreated:
	default:
		return CaptureResult{}, ErrInvalidTransition
	}

	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'requires_action',
    method_state = $3,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, paymentID, merchantID, MethodStateCard3DSRequired); err != nil {
		return CaptureResult{}, err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_payments.card_challenge_sessions
    (payment_id, merchant_id, card_token_id, session_id, callback_token, status, redirect_url, challenge_reference, auto_capture_requested, expires_at)
VALUES ($1,$2,$3,$4,$5,'pending',$6,$7,$8,$9)
ON CONFLICT (payment_id) DO UPDATE
SET status = 'pending',
    session_id = EXCLUDED.session_id,
    callback_token = EXCLUDED.callback_token,
    redirect_url = EXCLUDED.redirect_url,
    challenge_reference = EXCLUDED.challenge_reference,
    auto_capture_requested = EXCLUDED.auto_capture_requested,
    expires_at = EXCLUDED.expires_at,
    completed_at = NULL,
    failure_code = '',
    failure_description = '',
    updated_at = NOW()
`, paymentID, merchantID, in.CardTokenID, in.SessionID, in.CallbackToken, in.RedirectURL, in.ChallengeReference, in.AutoCaptureRequested, in.ExpiresAt); err != nil {
		return CaptureResult{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payment_attempts
SET status = 'requires_action',
    gateway_reference = NULLIF($2, ''),
    updated_at = NOW()
WHERE id = $1
`, attemptID, in.ChallengeReference); err != nil {
		return CaptureResult{}, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payment",
		AggregateID:   paymentID,
		EventType:     "payment.pending_customer_action",
		MerchantID:    merchantID,
		Payload: map[string]any{
			"payment_id": paymentID,
			"order_id":   orderID,
			"method":     "card",
			"flow_type":  "3ds",
		},
	}); err != nil {
		return CaptureResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CaptureResult{}, err
	}
	return r.GetPayment(ctx, merchantID, paymentID)
}

func (r *PostgresRepository) MarkAuthorizationAuthorized(ctx context.Context, merchantID, paymentID, gatewayReference, authCode string, autoCaptureAt *time.Time) (CaptureResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CaptureResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current CaptureResult
	var attemptID string
	var status string
	err = tx.QueryRow(ctx, `
SELECT id, attempt_id, merchant_id, order_id, amount, currency, method, method_state, COALESCE(method_state_reason, ''), status, captured, created_at, authorized_at
FROM paygate_payments.payments
WHERE id = $1 AND merchant_id = $2
FOR UPDATE
`, paymentID, merchantID).Scan(&current.PaymentID, &attemptID, &current.MerchantID, &current.OrderID, &current.Amount, &current.Currency, &current.Method, &current.MethodState, &current.MethodStateReason, &status, &current.Captured, &current.CreatedAt, &current.AuthorizedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrPaymentNotFound
		}
		return CaptureResult{}, err
	}
	switch PaymentState(status) {
	case StateAuthorized:
		return current, nil
	case StateCreated:
	default:
		return CaptureResult{}, ErrInvalidTransition
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'authorized',
    method_state = $6,
    gateway_reference = $2,
    auth_code = $3,
    authorized_at = $4,
    auto_capture_at = $5,
    updated_at = NOW()
WHERE id = $1
`, paymentID, gatewayReference, authCode, now, autoCaptureAt, methodStateForAuthorized(current.Method)); err != nil {
		return CaptureResult{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payment_attempts
SET status = 'authorized',
    gateway_reference = $2,
    updated_at = NOW()
WHERE id = $1
`, attemptID, gatewayReference); err != nil {
		return CaptureResult{}, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment", AggregateID: paymentID, EventType: "payment.authorized", MerchantID: merchantID, Payload: map[string]any{"payment_id": paymentID, "order_id": current.OrderID}}); err != nil {
		return CaptureResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CaptureResult{}, err
	}
	return r.GetPayment(ctx, merchantID, paymentID)
}

func (r *PostgresRepository) MarkAuthorizationFailed(ctx context.Context, merchantID, paymentID, errorCode, errorDescription string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var attemptID string
	var orderID string
	var currentMethod string
	var status string
	err = tx.QueryRow(ctx, `
SELECT attempt_id, order_id, method, status
FROM paygate_payments.payments
WHERE id = $1 AND merchant_id = $2
FOR UPDATE
`, paymentID, merchantID).Scan(&attemptID, &orderID, &currentMethod, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPaymentNotFound
		}
		return err
	}
	switch PaymentState(status) {
	case StateFailed:
		return nil
	case StateCreated, StateRequiresAction:
	default:
		return ErrInvalidTransition
	}

	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'failed',
    method_state = $4,
    method_state_reason = NULLIF($5, ''),
    error_code = $2,
    error_description = $3,
    updated_at = NOW()
WHERE id = $1
`, paymentID, errorCode, errorDescription, methodStateForFailure(currentMethod), errorDescription); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payment_attempts
SET status = 'failed',
    error_code = $2,
    error_description = $3,
    updated_at = NOW()
WHERE id = $1
`, attemptID, errorCode, errorDescription); err != nil {
		return err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment_attempt", AggregateID: attemptID, EventType: "payment.failed", MerchantID: merchantID, Payload: map[string]any{"payment_id": paymentID, "order_id": orderID, "error_code": errorCode}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) CompleteCardChallenge(ctx context.Context, merchantID, paymentID, callbackToken, gatewayReference, authCode string, completedAt time.Time) (CaptureResult, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CaptureResult{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current CaptureResult
	var attemptID, status string
	var challengeStatus, challengeToken string
	var autoCaptureRequested bool
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `
SELECT p.id, p.merchant_id, p.order_id, p.amount, p.currency, p.method, p.method_state, COALESCE(p.method_state_reason, ''), p.status, p.captured, p.created_at,
       COALESCE(cpd.card_token_id, ''), COALESCE(cpd.brand, ''), COALESCE(cpd.last4, ''), COALESCE(cpd.exp_month, 0), COALESCE(cpd.exp_year, 0), COALESCE(cpd.issuer_name, ''), COALESCE(cpd.issuer_country, ''), COALESCE(cpd.card_country, ''), COALESCE(cpd.funding_type, ''), COALESCE(cpd.network_token_type, ''),
       p.attempt_id, COALESCE(ccs.status, ''), COALESCE(ccs.callback_token, ''), COALESCE(ccs.auto_capture_requested, false), COALESCE(ccs.expires_at, NOW())
FROM paygate_payments.payments p
LEFT JOIN paygate_payments.card_payment_details cpd ON cpd.payment_id = p.id
LEFT JOIN paygate_payments.card_challenge_sessions ccs ON ccs.payment_id = p.id
WHERE p.id = $1 AND p.merchant_id = $2
FOR UPDATE
`, paymentID, merchantID).Scan(
		&current.PaymentID, &current.MerchantID, &current.OrderID, &current.Amount, &current.Currency, &current.Method, &current.MethodState, &current.MethodStateReason, &status, &current.Captured, &current.CreatedAt,
		&current.PaymentMethodTokenID, &current.CardBrand, &current.CardLast4, &current.CardExpMonth, &current.CardExpYear, &current.CardIssuerName, &current.CardIssuerCountry, &current.CardCountry, &current.FundingType, &current.NetworkTokenType,
		&attemptID, &challengeStatus, &challengeToken, &autoCaptureRequested, &expiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, false, ErrPaymentNotFound
		}
		return CaptureResult{}, false, err
	}
	if challengeToken == "" || callbackToken == "" || challengeToken != callbackToken {
		return CaptureResult{}, false, ErrCardChallengeRejected
	}
	if challengeStatus != "pending" {
		out, err := r.GetPayment(ctx, merchantID, paymentID)
		return out, false, err
	}
	if PaymentState(status) != StateRequiresAction {
		out, err := r.GetPayment(ctx, merchantID, paymentID)
		return out, false, err
	}
	if completedAt.After(expiresAt) {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.card_challenge_sessions
SET status = 'expired', completed_at = $2, failure_code = 'CARD_3DS_EXPIRED', failure_description = 'challenge session expired', updated_at = NOW()
WHERE payment_id = $1
`, paymentID, completedAt); err != nil {
			return CaptureResult{}, false, err
		}
		if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'failed', method_state = $3, method_state_reason = 'challenge session expired', error_code = 'CARD_3DS_EXPIRED', error_description = 'challenge session expired', updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, paymentID, merchantID, MethodStateCard3DSExpired); err != nil {
			return CaptureResult{}, false, err
		}
		if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payment_attempts
SET status = 'failed', error_code = 'CARD_3DS_EXPIRED', error_description = 'challenge session expired', updated_at = NOW()
WHERE id = $1
`, attemptID); err != nil {
			return CaptureResult{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CaptureResult{}, false, err
		}
		out, err := r.GetPayment(ctx, merchantID, paymentID)
		return out, false, err
	}
	var autoCaptureAt *time.Time
	if autoCaptureRequested {
		t := completedAt.Add(30 * time.Second)
		autoCaptureAt = &t
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'authorized',
    method_state = $4,
    gateway_reference = NULLIF($2, ''),
    auth_code = NULLIF($3, ''),
    authorized_at = $5,
    auto_capture_at = $6,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $7
`, paymentID, gatewayReference, authCode, MethodStateCard3DSAuthenticated, completedAt, autoCaptureAt, merchantID); err != nil {
		return CaptureResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payment_attempts
SET status = 'authorized', gateway_reference = NULLIF($2, ''), updated_at = NOW()
WHERE id = $1
`, attemptID, gatewayReference); err != nil {
		return CaptureResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.card_challenge_sessions
SET status = 'completed', completed_at = $2, failure_code = '', failure_description = '', updated_at = NOW()
WHERE payment_id = $1
`, paymentID, completedAt); err != nil {
		return CaptureResult{}, false, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment", AggregateID: paymentID, EventType: "payment.authorized", MerchantID: merchantID, Payload: map[string]any{"payment_id": paymentID, "order_id": current.OrderID, "flow_type": "3ds"}}); err != nil {
		return CaptureResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CaptureResult{}, false, err
	}
	out, err := r.GetPayment(ctx, merchantID, paymentID)
	return out, true, err
}

func (r *PostgresRepository) FailCardChallenge(ctx context.Context, merchantID, paymentID, callbackToken, errorCode, errorDescription string, challengeStatus string, failedAt time.Time) (CaptureResult, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CaptureResult{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var attemptID, orderID, currentStatus, storedToken string
	err = tx.QueryRow(ctx, `
SELECT p.attempt_id, p.order_id, p.status, COALESCE(ccs.callback_token, '')
FROM paygate_payments.payments p
LEFT JOIN paygate_payments.card_challenge_sessions ccs ON ccs.payment_id = p.id
WHERE p.id = $1 AND p.merchant_id = $2
FOR UPDATE
`, paymentID, merchantID).Scan(&attemptID, &orderID, &currentStatus, &storedToken)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, false, ErrPaymentNotFound
		}
		return CaptureResult{}, false, err
	}
	if storedToken == "" || callbackToken == "" || storedToken != callbackToken {
		return CaptureResult{}, false, ErrCardChallengeRejected
	}
	if PaymentState(currentStatus) != StateRequiresAction {
		out, err := r.GetPayment(ctx, merchantID, paymentID)
		return out, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.card_challenge_sessions
SET status = $2, completed_at = $3, failure_code = $4, failure_description = $5, updated_at = NOW()
WHERE payment_id = $1
`, paymentID, challengeStatus, failedAt, errorCode, errorDescription); err != nil {
		return CaptureResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'failed', method_state = $4, method_state_reason = NULLIF($5, ''), error_code = $2, error_description = $3, updated_at = NOW()
WHERE id = $1 AND merchant_id = $6
`, paymentID, errorCode, errorDescription, methodStateForChallengeFailure(challengeStatus), errorDescription, merchantID); err != nil {
		return CaptureResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payment_attempts
SET status = 'failed', error_code = $2, error_description = $3, updated_at = NOW()
WHERE id = $1
`, attemptID, errorCode, errorDescription); err != nil {
		return CaptureResult{}, false, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment_attempt", AggregateID: attemptID, EventType: "payment.failed", MerchantID: merchantID, Payload: map[string]any{"payment_id": paymentID, "order_id": orderID, "error_code": errorCode}}); err != nil {
		return CaptureResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CaptureResult{}, false, err
	}
	out, err := r.GetPayment(ctx, merchantID, paymentID)
	return out, true, err
}

func (r *PostgresRepository) ReverseAuthorization(ctx context.Context, merchantID, paymentID, reason string) (CaptureResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CaptureResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current CaptureResult
	var attemptID string
	var status string
	err = tx.QueryRow(ctx, `
SELECT id, attempt_id, merchant_id, order_id, amount, currency, method, method_state, COALESCE(method_state_reason, ''), status, captured, created_at, authorized_at
FROM paygate_payments.payments
WHERE id = $1 AND merchant_id = $2
FOR UPDATE
`, paymentID, merchantID).Scan(&current.PaymentID, &attemptID, &current.MerchantID, &current.OrderID, &current.Amount, &current.Currency, &current.Method, &current.MethodState, &current.MethodStateReason, &status, &current.Captured, &current.CreatedAt, &current.AuthorizedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrPaymentNotFound
		}
		return CaptureResult{}, err
	}
	switch PaymentState(status) {
	case StateAuthorizationReversed:
		current.Status = StateAuthorizationReversed
		return current, nil
	case StateAuthorized:
	default:
		return CaptureResult{}, ErrInvalidTransition
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'authorization_reversed',
    method_state = $4,
    method_state_reason = NULLIF($5, ''),
    auto_capture_at = NULL,
    error_code = 'AUTHORIZATION_REVERSED',
    error_description = NULLIF($2, ''),
    updated_at = $3
WHERE id = $1
`, paymentID, reason, now, MethodStateCardAuthorizationReversed, reason); err != nil {
		return CaptureResult{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payment_attempts
SET status = 'authorization_reversed',
    error_code = 'AUTHORIZATION_REVERSED',
    error_description = NULLIF($2, ''),
    updated_at = $3
WHERE id = $1
`, attemptID, reason, now); err != nil {
		return CaptureResult{}, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payment",
		AggregateID:   paymentID,
		EventType:     "payment.authorization_reversed",
		MerchantID:    merchantID,
		Payload: map[string]any{
			"payment_id": paymentID,
			"order_id":   current.OrderID,
			"reason":     reason,
		},
	}); err != nil {
		return CaptureResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CaptureResult{}, err
	}
	return r.GetPayment(ctx, merchantID, paymentID)
}

func (r *PostgresRepository) CaptureAuthorizedPayment(ctx context.Context, merchantID, paymentID string, amount int64) (CaptureResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CaptureResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current CaptureResult
	var status string
	err = tx.QueryRow(ctx, `
SELECT id, merchant_id, order_id, amount, currency, method, method_state, COALESCE(method_state_reason, ''), status, captured, created_at, authorized_at
FROM paygate_payments.payments
WHERE id=$1 AND merchant_id=$2 FOR UPDATE
`, paymentID, merchantID).Scan(&current.PaymentID, &current.MerchantID, &current.OrderID, &current.Amount, &current.Currency, &current.Method, &current.MethodState, &current.MethodStateReason, &status, &current.Captured, &current.CreatedAt, &current.AuthorizedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrPaymentNotFound
		}
		return CaptureResult{}, err
	}

	if PaymentState(status) != StateAuthorized {
		if PaymentState(status) == StateCaptured {
			return current, nil
		}
		return CaptureResult{}, ErrInvalidTransition
	}
	if amount != current.Amount {
		return CaptureResult{}, ErrAmountMismatch
	}

	fee := amount * 2 / 100
	splits, err := r.listPaymentSplitsTx(ctx, tx, paymentID)
	if err != nil {
		return CaptureResult{}, err
	}
	entries := []ledger.Entry{
		{AccountCode: "CUSTOMER_RECEIVABLE", DebitAmount: amount, Description: "payment capture receivable"},
		{AccountCode: "PLATFORM_FEE_REVENUE", CreditAmount: fee, Description: "platform fee revenue"},
	}
	var splitTotal int64
	for _, split := range splits {
		if split.Amount <= 0 {
			continue
		}
		splitTotal += split.Amount
		description := "connected account payable"
		if split.BeneficiaryLabel != "" {
			description = "connected account payable: " + split.BeneficiaryLabel
		}
		entries = append(entries, ledger.Entry{AccountCode: "MERCHANT_PAYABLE", CreditAmount: split.Amount, Description: description})
	}
	residual := amount - fee - splitTotal
	if residual < 0 {
		return CaptureResult{}, ErrAmountMismatch
	}
	if residual > 0 {
		entries = append(entries, ledger.Entry{AccountCode: "MERCHANT_PAYABLE", CreditAmount: residual, Description: "merchant payable on capture"})
	}
	if _, err := r.ledger.CreateEntriesTx(ctx, tx, current.MerchantID, "payment", current.PaymentID, "payment capture", entries); err != nil {
		return CaptureResult{}, err
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `UPDATE paygate_payments.payments SET status='captured', method_state=$4, captured=true, captured_at=$2, updated_at=NOW(), fee=$3 WHERE id=$1`, paymentID, now, fee, methodStateForCapture(current.Method))
	if err != nil {
		return CaptureResult{}, err
	}

	_, err = tx.Exec(ctx, `UPDATE paygate_orders.orders SET status='paid', amount_paid=amount, amount_due=0, updated_at=NOW() WHERE id=$1 AND merchant_id=$2`, current.OrderID, current.MerchantID)
	if err != nil {
		return CaptureResult{}, err
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment", AggregateID: paymentID, EventType: "payment.captured", MerchantID: current.MerchantID, Payload: map[string]any{"payment_id": paymentID, "order_id": current.OrderID}}); err != nil {
		return CaptureResult{}, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "order", AggregateID: current.OrderID, EventType: "order.paid", MerchantID: current.MerchantID, Payload: map[string]any{"order_id": current.OrderID, "payment_id": paymentID}}); err != nil {
		return CaptureResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CaptureResult{}, err
	}
	return r.GetPayment(ctx, merchantID, paymentID)
}

func (r *PostgresRepository) GetPayment(ctx context.Context, merchantID, paymentID string) (CaptureResult, error) {
	var out CaptureResult
	var status string
	err := r.db.QueryRow(ctx, `
SELECT p.id, p.merchant_id, p.order_id, p.amount, p.currency, p.method, COALESCE(p.provider, ''), COALESCE(p.routing_reason, ''), COALESCE(p.attempted_providers, '{}'), p.method_state, COALESCE(p.method_state_reason, ''), p.status, p.captured, p.captured_at, p.created_at, p.authorized_at,
       COALESCE(cpd.card_token_id, ''), COALESCE(cpd.brand, ''), COALESCE(cpd.last4, ''), COALESCE(cpd.exp_month, 0), COALESCE(cpd.exp_year, 0), COALESCE(cpd.issuer_name, ''), COALESCE(cpd.issuer_country, ''), COALESCE(cpd.card_country, ''), COALESCE(cpd.funding_type, ''), COALESCE(cpd.network_token_type, ''),
       COALESCE(ccs.session_id, ''), COALESCE(ccs.status, ''), COALESCE(ccs.redirect_url, ''), COALESCE(ccs.callback_token, ''), ccs.expires_at
FROM paygate_payments.payments p
LEFT JOIN paygate_payments.card_payment_details cpd ON cpd.payment_id = p.id
LEFT JOIN paygate_payments.card_challenge_sessions ccs ON ccs.payment_id = p.id
WHERE p.id=$1 AND p.merchant_id=$2
`, paymentID, merchantID).Scan(&out.PaymentID, &out.MerchantID, &out.OrderID, &out.Amount, &out.Currency, &out.Method, &out.Provider, &out.RoutingReason, &out.AttemptedProviders, &out.MethodState, &out.MethodStateReason, &status, &out.Captured, &out.CapturedAt, &out.CreatedAt, &out.AuthorizedAt, &out.PaymentMethodTokenID, &out.CardBrand, &out.CardLast4, &out.CardExpMonth, &out.CardExpYear, &out.CardIssuerName, &out.CardIssuerCountry, &out.CardCountry, &out.FundingType, &out.NetworkTokenType, &out.ChallengeSessionID, &out.ChallengeStatus, &out.ChallengeRedirectURL, &out.ChallengeCallbackToken, &out.ChallengeExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrPaymentNotFound
		}
		return CaptureResult{}, err
	}
	out.Status = PaymentState(status)
	out.Splits, _ = r.listPaymentSplits(ctx, paymentID)
	return out, nil
}

func (r *PostgresRepository) ListPayments(ctx context.Context, f ListFilter) (ListResult, error) {
	if f.Count <= 0 || f.Count > 100 {
		f.Count = 20
	}
	args := []any{f.MerchantID}
	query := `
SELECT p.id, p.merchant_id, p.order_id, p.amount, p.currency, p.method, COALESCE(p.provider, ''), COALESCE(p.routing_reason, ''), COALESCE(p.attempted_providers, '{}'), p.method_state, COALESCE(p.method_state_reason, ''), p.status, p.captured, p.captured_at, p.created_at, p.authorized_at,
       COALESCE(cpd.card_token_id, ''), COALESCE(cpd.brand, ''), COALESCE(cpd.last4, ''), COALESCE(cpd.exp_month, 0), COALESCE(cpd.exp_year, 0), COALESCE(cpd.issuer_name, ''), COALESCE(cpd.issuer_country, ''), COALESCE(cpd.card_country, ''), COALESCE(cpd.funding_type, ''), COALESCE(cpd.network_token_type, ''),
       COALESCE(ccs.session_id, ''), COALESCE(ccs.status, ''), COALESCE(ccs.redirect_url, ''), COALESCE(ccs.callback_token, ''), ccs.expires_at
FROM paygate_payments.payments p
LEFT JOIN paygate_payments.card_payment_details cpd ON cpd.payment_id = p.id
LEFT JOIN paygate_payments.card_challenge_sessions ccs ON ccs.payment_id = p.id
WHERE p.merchant_id = $1`
	if f.OrderID != "" {
		query += ` AND p.order_id = $2`
		args = append(args, f.OrderID)
	}
	query += ` ORDER BY p.created_at DESC LIMIT `
	if len(args) == 1 {
		query += `$2`
	} else {
		query += `$3`
	}
	args = append(args, f.Count)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()

	var items []CaptureResult
	for rows.Next() {
		var item CaptureResult
		var status string
		if err := rows.Scan(
			&item.PaymentID,
			&item.MerchantID,
			&item.OrderID,
			&item.Amount,
			&item.Currency,
			&item.Method,
			&item.Provider,
			&item.RoutingReason,
			&item.AttemptedProviders,
			&item.MethodState,
			&item.MethodStateReason,
			&status,
			&item.Captured,
			&item.CapturedAt,
			&item.CreatedAt,
			&item.AuthorizedAt,
			&item.PaymentMethodTokenID,
			&item.CardBrand,
			&item.CardLast4,
			&item.CardExpMonth,
			&item.CardExpYear,
			&item.CardIssuerName,
			&item.CardIssuerCountry,
			&item.CardCountry,
			&item.FundingType,
			&item.NetworkTokenType,
			&item.ChallengeSessionID,
			&item.ChallengeStatus,
			&item.ChallengeRedirectURL,
			&item.ChallengeCallbackToken,
			&item.ChallengeExpiresAt,
		); err != nil {
			return ListResult{}, err
		}
		item.Status = PaymentState(status)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, err
	}
	// Batch-fetch splits to avoid N+1 queries.
	paymentIDs := make([]string, len(items))
	for i := range items {
		paymentIDs[i] = items[i].PaymentID
	}
	splitsMap, err := r.listPaymentSplitsBatch(ctx, paymentIDs)
	if err != nil {
		return ListResult{}, err
	}
	for i := range items {
		items[i].Splits = splitsMap[items[i].PaymentID]
	}
	return ListResult{Items: items}, nil
}

func (r *PostgresRepository) AutoCaptureDue(ctx context.Context) (int64, error) {
	// Process payments one at a time. Each iteration locks a single row with
	// FOR UPDATE SKIP LOCKED so concurrent sweeper instances never double-capture
	// the same payment.
	var count int64
	for range 50 {
		tx, err := r.db.Begin(ctx)
		if err != nil {
			return count, err
		}

		var id, merchantID string
		var amount int64
		err = tx.QueryRow(ctx, `
SELECT id, merchant_id, amount
FROM paygate_payments.payments
WHERE status = 'authorized'
  AND auto_capture_at IS NOT NULL
  AND auto_capture_at <= NOW()
ORDER BY auto_capture_at
LIMIT 1
FOR UPDATE SKIP LOCKED
`).Scan(&id, &merchantID, &amount)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			break
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return count, err
		}
		// Release the lock; CaptureAuthorizedPayment opens its own TX with
		// FOR UPDATE on the same row. The brief window between rollback and
		// re-acquire is safe because CaptureAuthorizedPayment checks status
		// after locking: if another goroutine captured first, it returns the
		// existing result (idempotent). FOR UPDATE SKIP LOCKED above prevents
		// double-pickup within a single sweeper cycle.
		_ = tx.Rollback(ctx)

		if _, err := r.CaptureAuthorizedPayment(ctx, merchantID, id, amount); err == nil {
			count++
		}
	}
	return count, nil
}

func (r *PostgresRepository) ExpireAuthorizationWindow(ctx context.Context, window time.Duration) (int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
SELECT id, merchant_id, order_id
FROM paygate_payments.payments
WHERE status = 'authorized' AND authorized_at <= NOW() - ($1::interval)
ORDER BY authorized_at
LIMIT 50
FOR UPDATE SKIP LOCKED
`, fmt.Sprintf("%f seconds", window.Seconds()))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type expiringPayment struct {
		id         string
		merchantID string
		orderID    string
	}
	var payments []expiringPayment
	for rows.Next() {
		var item expiringPayment
		if err := rows.Scan(&item.id, &item.merchantID, &item.orderID); err != nil {
			return 0, err
		}
		payments = append(payments, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, item := range payments {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status='auto_refunded', method_state='card_authorization_reversed', method_state_reason='capture window expired', error_code='AUTH_WINDOW_EXPIRED', error_description='capture window expired', updated_at=NOW()
WHERE id = $1
`, item.id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `
UPDATE paygate_orders.orders
SET status = CASE WHEN status = 'attempted' THEN 'failed' ELSE status END,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, item.orderID, item.merchantID); err != nil {
			return 0, err
		}
		if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
			AggregateType: "payment",
			AggregateID:   item.id,
			EventType:     "payment.auto_refunded",
			MerchantID:    item.merchantID,
			Payload: map[string]any{
				"payment_id": item.id,
				"order_id":   item.orderID,
			},
		}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int64(len(payments)), nil
}

func (r *PostgresRepository) getPaymentByIdempotencyKey(ctx context.Context, merchantID, orderID, idempotencyKey string) (CaptureResult, error) {
	var paymentID *string
	if err := r.db.QueryRow(ctx, `
SELECT payment_id
FROM paygate_payments.payment_attempts
WHERE merchant_id = $1 AND order_id = $2 AND idempotency_key = $3
  AND payment_id IS NOT NULL
ORDER BY created_at DESC
LIMIT 1
`, merchantID, orderID, idempotencyKey).Scan(&paymentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrPaymentNotFound
		}
		return CaptureResult{}, err
	}
	if paymentID == nil || *paymentID == "" {
		return CaptureResult{}, ErrPaymentNotFound
	}
	return r.GetPayment(ctx, merchantID, *paymentID)
}

func (r *PostgresRepository) insertPaymentSplitsTx(ctx context.Context, tx pgx.Tx, merchantID, paymentID string, splits []CreatePaymentSplitInput) error {
	for _, split := range splits {
		if split.Amount < 0 {
			return ErrAmountMismatch
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_payments.payment_splits
    (id, merchant_id, payment_id, destination_type, destination_ref, beneficiary_label, amount, currency)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
`, idgen.New("psplit"), merchantID, paymentID, split.DestinationType, split.DestinationRef, split.BeneficiaryLabel, split.Amount, split.Currency); err != nil {
			return fmt.Errorf("insert payment split: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) listPaymentSplits(ctx context.Context, paymentID string) ([]PaymentSplit, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, payment_id, destination_type, destination_ref, beneficiary_label, amount, currency, created_at
FROM paygate_payments.payment_splits
WHERE payment_id = $1
ORDER BY created_at, id
`, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPaymentSplits(rows)
}

func (r *PostgresRepository) listPaymentSplitsBatch(ctx context.Context, paymentIDs []string) (map[string][]PaymentSplit, error) {
	if len(paymentIDs) == 0 {
		return map[string][]PaymentSplit{}, nil
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, payment_id, destination_type, destination_ref, beneficiary_label, amount, currency, created_at
FROM paygate_payments.payment_splits
WHERE payment_id = ANY($1)
ORDER BY payment_id, created_at, id
`, paymentIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]PaymentSplit)
	for rows.Next() {
		var item PaymentSplit
		if err := rows.Scan(&item.ID, &item.MerchantID, &item.PaymentID, &item.DestinationType, &item.DestinationRef, &item.BeneficiaryLabel, &item.Amount, &item.Currency, &item.CreatedAt); err != nil {
			return nil, err
		}
		out[item.PaymentID] = append(out[item.PaymentID], item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) listPaymentSplitsTx(ctx context.Context, tx pgx.Tx, paymentID string) ([]PaymentSplit, error) {
	rows, err := tx.Query(ctx, `
SELECT id, merchant_id, payment_id, destination_type, destination_ref, beneficiary_label, amount, currency, created_at
FROM paygate_payments.payment_splits
WHERE payment_id = $1
ORDER BY created_at, id
`, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPaymentSplits(rows)
}

func scanPaymentSplits(rows pgx.Rows) ([]PaymentSplit, error) {
	var out []PaymentSplit
	for rows.Next() {
		var item PaymentSplit
		if err := rows.Scan(&item.ID, &item.MerchantID, &item.PaymentID, &item.DestinationType, &item.DestinationRef, &item.BeneficiaryLabel, &item.Amount, &item.Currency, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func nullableText(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func nonEmptyText(v string) string {
	return v
}

func normalizeAttemptedProviders(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
