package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/outbox"
)

func (r *PostgresRepository) CreateRedirectSession(ctx context.Context, in CreateRedirectSessionRecordInput) (RedirectSessionResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return RedirectSessionResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if in.Amount <= 0 {
		return RedirectSessionResult{}, ErrAmountMismatch
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
			return RedirectSessionResult{}, ErrOrderNotFound
		}
		return RedirectSessionResult{}, fmt.Errorf("lock order: %w", err)
	}
	if orderStatus == "expired" || time.Now().UTC().After(expiresAt) {
		return RedirectSessionResult{}, ErrOrderExpired
	}
	if in.Currency != orderCurrency {
		return RedirectSessionResult{}, ErrCurrencyMismatch
	}
	if partialPayment {
		if in.Amount <= 0 || in.Amount > orderAmountDue {
			return RedirectSessionResult{}, ErrAmountMismatch
		}
	} else if in.Amount != orderAmount {
		return RedirectSessionResult{}, ErrAmountMismatch
	}
	attemptID := idgen.New("attempt")
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payments.payment_attempts
(id, order_id, merchant_id, payment_id, amount, currency, method, provider, routing_reason, attempted_providers, status, idempotency_key)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'processing',$11)
`, attemptID, in.OrderID, in.MerchantID, in.PaymentID, in.Amount, in.Currency, in.Method, nonEmptyText(in.Provider), nonEmptyText(in.RoutingReason), normalizeAttemptedProviders(in.Attempted), in.IdempotencyKey)
	if err != nil {
		var pgErr *pgconn.PgError
		if in.IdempotencyKey != "" && errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return r.getRedirectSessionByIdempotencyKey(ctx, in.MerchantID, in.OrderID, in.IdempotencyKey)
		}
		return RedirectSessionResult{}, fmt.Errorf("insert redirect payment attempt: %w", err)
	}
	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payments.payments
(id, attempt_id, order_id, merchant_id, amount, currency, method, provider, routing_reason, attempted_providers, method_state, status, captured)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'pending_customer_action',false)
`, in.PaymentID, attemptID, in.OrderID, in.MerchantID, in.Amount, in.Currency, in.Method, nonEmptyText(in.Provider), nonEmptyText(in.RoutingReason), normalizeAttemptedProviders(in.Attempted), initialMethodState(in.Method))
	if err != nil {
		return RedirectSessionResult{}, fmt.Errorf("insert redirect payment: %w", err)
	}
	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payments.redirect_payment_details
(payment_id, merchant_id, order_id, method, bank_code, bank_name, wallet_code, wallet_name, provider_status, callback_token, expires_at)
VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),'pending',$9,$10)
`, in.PaymentID, in.MerchantID, in.OrderID, in.Method, in.BankCode, in.BankName, in.WalletCode, in.WalletName, in.CallbackToken, in.ExpiresAt)
	if err != nil {
		return RedirectSessionResult{}, fmt.Errorf("insert redirect detail: %w", err)
	}
	_, _ = tx.Exec(ctx, `
UPDATE paygate_orders.orders
SET status = CASE WHEN status = 'created' THEN 'attempted' ELSE status END,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, in.OrderID, in.MerchantID)
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payment",
		AggregateID:   in.PaymentID,
		EventType:     "payment.pending_customer_action",
		MerchantID:    in.MerchantID,
		Payload: map[string]any{
			"payment_id": in.PaymentID,
			"order_id":   in.OrderID,
			"method":     in.Method,
		},
	}); err != nil {
		return RedirectSessionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RedirectSessionResult{}, err
	}
	return RedirectSessionResult{
		CaptureResult: CaptureResult{
			PaymentID:          in.PaymentID,
			MerchantID:         in.MerchantID,
			OrderID:            in.OrderID,
			Amount:             in.Amount,
			Currency:           in.Currency,
			Method:             in.Method,
			Provider:           in.Provider,
			RoutingReason:      in.RoutingReason,
			AttemptedProviders: in.Attempted,
			MethodState:        initialMethodState(in.Method),
			Status:             StatePendingCustomerAction,
			Captured:           false,
			CreatedAt:          now,
		},
		FlowType:       in.FlowType,
		BankCode:       in.BankCode,
		BankName:       in.BankName,
		WalletCode:     in.WalletCode,
		WalletName:     in.WalletName,
		ProviderStatus: RedirectProviderStatusPending,
		CallbackToken:  in.CallbackToken,
		ExpiresAt:      in.ExpiresAt,
	}, nil
}

func (r *PostgresRepository) AttachRedirectGatewayData(ctx context.Context, merchantID, paymentID string, gatewayResult GatewayRedirectResult) (RedirectSessionResult, error) {
	_, err := r.db.Exec(ctx, `
UPDATE paygate_payments.redirect_payment_details
SET redirect_url = $3,
    gateway_reference = $4,
    bank_code = COALESCE(NULLIF($5, ''), bank_code),
    bank_name = COALESCE(NULLIF($6, ''), bank_name),
    wallet_code = COALESCE(NULLIF($7, ''), wallet_code),
    wallet_name = COALESCE(NULLIF($8, ''), wallet_name),
    expires_at = $9,
    updated_at = NOW()
WHERE payment_id = $1 AND merchant_id = $2
`, paymentID, merchantID, gatewayResult.RedirectURL, gatewayResult.GatewayReference, gatewayResult.BankCode, gatewayResult.BankName, gatewayResult.WalletCode, gatewayResult.WalletName, gatewayResult.ExpiresAt)
	if err != nil {
		return RedirectSessionResult{}, err
	}
	_, err = r.db.Exec(ctx, `
UPDATE paygate_payments.payments
SET gateway_reference = $3,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, paymentID, merchantID, gatewayResult.GatewayReference)
	if err != nil {
		return RedirectSessionResult{}, err
	}
	return r.GetRedirectSession(ctx, merchantID, paymentID)
}

func (r *PostgresRepository) GetRedirectSession(ctx context.Context, merchantID, paymentID string) (RedirectSessionResult, error) {
	var out RedirectSessionResult
	var status string
	var providerStatus string
	err := r.db.QueryRow(ctx, `
SELECT p.id, p.merchant_id, p.order_id, p.amount, p.currency, p.method, COALESCE(p.provider, ''), COALESCE(p.routing_reason, ''), COALESCE(p.attempted_providers, '{}'), COALESCE(p.method_state, ''), COALESCE(p.method_state_reason, ''), p.status, p.captured,
       p.captured_at, p.created_at, p.authorized_at,
       COALESCE(d.bank_code, ''), COALESCE(d.bank_name, ''), COALESCE(d.wallet_code, ''), COALESCE(d.wallet_name, ''), COALESCE(d.redirect_url, ''), COALESCE(d.gateway_reference, ''),
       d.provider_status, d.callback_token, d.expires_at, d.completed_at, d.last_polled_at, COALESCE(d.failure_code, ''), COALESCE(d.failure_description, '')
FROM paygate_payments.payments p
JOIN paygate_payments.redirect_payment_details d ON d.payment_id = p.id
WHERE p.id = $1 AND p.merchant_id = $2
`, paymentID, merchantID).Scan(
		&out.PaymentID, &out.MerchantID, &out.OrderID, &out.Amount, &out.Currency, &out.Method, &out.Provider, &out.RoutingReason, &out.AttemptedProviders, &out.MethodState, &out.MethodStateReason, &status, &out.Captured,
		&out.CapturedAt, &out.CreatedAt, &out.AuthorizedAt,
		&out.BankCode, &out.BankName, &out.WalletCode, &out.WalletName, &out.RedirectURL, &out.GatewayReference,
		&providerStatus, &out.CallbackToken, &out.ExpiresAt, &out.CompletedAt, &out.LastPolledAt, &out.FailureCode, &out.FailureDescription,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RedirectSessionResult{}, ErrPaymentNotFound
		}
		return RedirectSessionResult{}, err
	}
	out.Status = PaymentState(status)
	out.ProviderStatus = RedirectProviderStatus(providerStatus)
	out.FlowType = out.Method
	return out, nil
}

func (r *PostgresRepository) PollRedirectSession(ctx context.Context, merchantID, paymentID string, polledAt time.Time) error {
	cmd, err := r.db.Exec(ctx, `
UPDATE paygate_payments.redirect_payment_details
SET last_polled_at = $3,
    updated_at = NOW()
WHERE payment_id = $1 AND merchant_id = $2
`, paymentID, merchantID, polledAt)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrPaymentNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkRedirectProcessing(ctx context.Context, paymentID string, eventID string) (RedirectSessionResult, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return RedirectSessionResult{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, terminal, processed, err := r.lockRedirectSessionTx(ctx, tx, paymentID, eventID)
	if err != nil {
		return RedirectSessionResult{}, false, err
	}
	if !processed || terminal {
		if err := tx.Commit(ctx); err != nil {
			return RedirectSessionResult{}, false, err
		}
		return current, false, nil
	}
	state := MethodStateNetbankingProcessing
	if current.Method == "wallet" {
		state = MethodStateWalletProcessing
	}
	if current.Status == StatePendingCustomerAction {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'processing', method_state = $2, updated_at = NOW()
WHERE id = $1
`, paymentID, state); err != nil {
			return RedirectSessionResult{}, false, err
		}
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.redirect_payment_details
SET provider_status = 'pending', updated_at = NOW()
WHERE payment_id = $1
`, paymentID); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RedirectSessionResult{}, false, err
	}
	out, err := r.GetRedirectSession(ctx, current.MerchantID, paymentID)
	return out, true, err
}

func (r *PostgresRepository) CompleteRedirectSession(ctx context.Context, paymentID string, eventID string, gatewayReference string, processedAt time.Time) (RedirectSessionResult, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return RedirectSessionResult{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, terminal, processed, err := r.lockRedirectSessionTx(ctx, tx, paymentID, eventID)
	if err != nil {
		return RedirectSessionResult{}, false, err
	}
	if !processed || terminal {
		if err := tx.Commit(ctx); err != nil {
			return RedirectSessionResult{}, false, err
		}
		return current, false, nil
	}
	fee := current.Amount * 2 / 100
	entries := []ledger.Entry{
		{AccountCode: "CUSTOMER_RECEIVABLE", DebitAmount: current.Amount, Description: current.Method + " capture receivable"},
		{AccountCode: "MERCHANT_PAYABLE", CreditAmount: current.Amount - fee, Description: "merchant payable on " + current.Method + " capture"},
		{AccountCode: "PLATFORM_FEE_REVENUE", CreditAmount: fee, Description: "platform fee revenue"},
	}
	if _, err := r.ledger.CreateEntriesTx(ctx, tx, current.MerchantID, "payment", current.PaymentID, current.Method+" payment capture", entries); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'captured', method_state = $5, captured = true, captured_at = $2, gateway_reference = COALESCE(NULLIF($3, ''), gateway_reference), fee = $4, updated_at = NOW()
WHERE id = $1
`, paymentID, processedAt, gatewayReference, fee, methodStateForCapture(current.Method)); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.redirect_payment_details
SET provider_status = 'succeeded', completed_at = $2, gateway_reference = COALESCE(NULLIF($3, ''), gateway_reference), updated_at = NOW()
WHERE payment_id = $1
`, paymentID, processedAt, gatewayReference); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_orders.orders
SET status = 'paid', amount_paid = amount, amount_due = 0, updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, current.OrderID, current.MerchantID); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment", AggregateID: paymentID, EventType: "payment.captured", MerchantID: current.MerchantID, Payload: map[string]any{"payment_id": paymentID, "order_id": current.OrderID, "method": current.Method}}); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "order", AggregateID: current.OrderID, EventType: "order.paid", MerchantID: current.MerchantID, Payload: map[string]any{"order_id": current.OrderID, "payment_id": paymentID}}); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RedirectSessionResult{}, false, err
	}
	out, err := r.GetRedirectSession(ctx, current.MerchantID, paymentID)
	return out, true, err
}

func (r *PostgresRepository) FailRedirectSession(ctx context.Context, paymentID string, eventID string, providerStatus RedirectProviderStatus, errorCode, errorDescription string, processedAt time.Time) (RedirectSessionResult, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return RedirectSessionResult{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, terminal, processed, err := r.lockRedirectSessionTx(ctx, tx, paymentID, eventID)
	if err != nil {
		return RedirectSessionResult{}, false, err
	}
	if !processed || terminal {
		if err := tx.Commit(ctx); err != nil {
			return RedirectSessionResult{}, false, err
		}
		return current, false, nil
	}
	if providerStatus == "" {
		providerStatus = RedirectProviderStatusFailed
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'failed', method_state = $4, method_state_reason = NULLIF($5, ''), error_code = $2, error_description = $3, updated_at = NOW()
WHERE id = $1
`, paymentID, errorCode, errorDescription, methodStateForFailure(current.Method), errorDescription); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payment_attempts
SET status = 'failed', error_code = $2, error_description = $3, updated_at = NOW()
WHERE payment_id = $1
`, paymentID, errorCode, errorDescription); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.redirect_payment_details
SET provider_status = $2, completed_at = $3, failure_code = $4, failure_description = $5, updated_at = NOW()
WHERE payment_id = $1
`, paymentID, providerStatus, processedAt, errorCode, errorDescription); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_orders.orders
SET status = CASE WHEN status = 'attempted' THEN 'failed' ELSE status END,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, current.OrderID, current.MerchantID); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment", AggregateID: paymentID, EventType: "payment.failed", MerchantID: current.MerchantID, Payload: map[string]any{"payment_id": paymentID, "order_id": current.OrderID, "method": current.Method, "error_code": errorCode}}); err != nil {
		return RedirectSessionResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RedirectSessionResult{}, false, err
	}
	out, err := r.GetRedirectSession(ctx, current.MerchantID, paymentID)
	return out, true, err
}

func (r *PostgresRepository) AbandonRedirectSession(ctx context.Context, merchantID, paymentID, reason string, abandonedAt time.Time) (RedirectSessionResult, error) {
	_, _, err := r.FailRedirectSession(ctx, paymentID, "", RedirectProviderStatusAbandoned, "CUSTOMER_ABANDONED", reason, abandonedAt)
	if err != nil {
		return RedirectSessionResult{}, err
	}
	return r.GetRedirectSession(ctx, merchantID, paymentID)
}

func (r *PostgresRepository) ExpireRedirectSession(ctx context.Context, merchantID, paymentID string, expiredAt time.Time) (RedirectSessionResult, error) {
	_, _, err := r.FailRedirectSession(ctx, paymentID, "", RedirectProviderStatusExpired, "REDIRECT_EXPIRED", "redirect session expired before completion", expiredAt)
	if err != nil {
		return RedirectSessionResult{}, err
	}
	return r.GetRedirectSession(ctx, merchantID, paymentID)
}

func (r *PostgresRepository) lockRedirectSessionTx(ctx context.Context, tx pgx.Tx, paymentID, eventID string) (RedirectSessionResult, bool, bool, error) {
	var current RedirectSessionResult
	var status string
	var providerStatus string
	err := tx.QueryRow(ctx, `
SELECT p.id, p.merchant_id, p.order_id, p.amount, p.currency, p.method, COALESCE(p.provider, ''), COALESCE(p.routing_reason, ''), COALESCE(p.attempted_providers, '{}'), COALESCE(p.method_state, ''), COALESCE(p.method_state_reason, ''), p.status, p.captured,
       p.captured_at, p.created_at, p.authorized_at,
       COALESCE(d.bank_code, ''), COALESCE(d.bank_name, ''), COALESCE(d.wallet_code, ''), COALESCE(d.wallet_name, ''), COALESCE(d.redirect_url, ''), COALESCE(d.gateway_reference, ''),
       d.provider_status, d.callback_token, d.expires_at, d.completed_at, d.last_polled_at, COALESCE(d.failure_code, ''), COALESCE(d.failure_description, '')
FROM paygate_payments.payments p
JOIN paygate_payments.redirect_payment_details d ON d.payment_id = p.id
WHERE p.id = $1
FOR UPDATE
`, paymentID).Scan(
		&current.PaymentID, &current.MerchantID, &current.OrderID, &current.Amount, &current.Currency, &current.Method, &current.Provider, &current.RoutingReason, &current.AttemptedProviders, &current.MethodState, &current.MethodStateReason, &status, &current.Captured,
		&current.CapturedAt, &current.CreatedAt, &current.AuthorizedAt,
		&current.BankCode, &current.BankName, &current.WalletCode, &current.WalletName, &current.RedirectURL, &current.GatewayReference,
		&providerStatus, &current.CallbackToken, &current.ExpiresAt, &current.CompletedAt, &current.LastPolledAt, &current.FailureCode, &current.FailureDescription,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RedirectSessionResult{}, false, false, ErrPaymentNotFound
		}
		return RedirectSessionResult{}, false, false, err
	}
	current.Status = PaymentState(status)
	current.ProviderStatus = RedirectProviderStatus(providerStatus)
	current.FlowType = current.Method
	if eventID != "" {
		inserted, err := r.insertRedirectCallbackEventTx(ctx, tx, paymentID, eventID)
		if err != nil {
			return RedirectSessionResult{}, false, false, err
		}
		if !inserted {
			return current, false, false, nil
		}
	}
	if current.Status == StateCaptured || current.Status == StateFailed {
		return current, true, true, nil
	}
	return current, false, true, nil
}

func (r *PostgresRepository) insertRedirectCallbackEventTx(ctx context.Context, tx pgx.Tx, paymentID, eventID string) (bool, error) {
	if eventID == "" {
		return true, nil
	}
	tag, err := tx.Exec(ctx, `
INSERT INTO paygate_payments.redirect_callback_events (event_id, payment_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`, eventID, paymentID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresRepository) getRedirectSessionByIdempotencyKey(ctx context.Context, merchantID, orderID, idempotencyKey string) (RedirectSessionResult, error) {
	var paymentID string
	if err := r.db.QueryRow(ctx, `
SELECT payment_id
FROM paygate_payments.payment_attempts
WHERE merchant_id = $1 AND order_id = $2 AND idempotency_key = $3
  AND payment_id IS NOT NULL
ORDER BY created_at DESC
LIMIT 1
`, merchantID, orderID, idempotencyKey).Scan(&paymentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RedirectSessionResult{}, ErrPaymentNotFound
		}
		return RedirectSessionResult{}, err
	}
	return r.GetRedirectSession(ctx, merchantID, paymentID)
}
