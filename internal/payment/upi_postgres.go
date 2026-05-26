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

func (r *PostgresRepository) CreateUPIIntent(ctx context.Context, in CreateUPIIntentRecordInput) (UPIIntentResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return UPIIntentResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if in.Amount <= 0 {
		return UPIIntentResult{}, ErrAmountMismatch
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
			return UPIIntentResult{}, ErrOrderNotFound
		}
		return UPIIntentResult{}, fmt.Errorf("lock order: %w", err)
	}
	if orderStatus == "expired" || time.Now().UTC().After(expiresAt) {
		return UPIIntentResult{}, ErrOrderExpired
	}
	if in.Currency != orderCurrency {
		return UPIIntentResult{}, ErrCurrencyMismatch
	}
	if partialPayment {
		if in.Amount <= 0 || in.Amount > orderAmountDue {
			return UPIIntentResult{}, ErrAmountMismatch
		}
	} else if in.Amount != orderAmount {
		return UPIIntentResult{}, ErrAmountMismatch
	}

	attemptID := idgen.New("attempt")
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payments.payment_attempts
(id, order_id, merchant_id, payment_id, amount, currency, method, status, idempotency_key)
VALUES ($1,$2,$3,$4,$5,$6,'upi','processing',$7)
`, attemptID, in.OrderID, in.MerchantID, in.PaymentID, in.Amount, in.Currency, in.IdempotencyKey)
	if err != nil {
		var pgErr *pgconn.PgError
		if in.IdempotencyKey != "" && errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return r.getUPIIntentByIdempotencyKey(ctx, in.MerchantID, in.OrderID, in.IdempotencyKey)
		}
		return UPIIntentResult{}, fmt.Errorf("insert upi payment attempt: %w", err)
	}

	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payments.payments
(id, attempt_id, order_id, merchant_id, amount, currency, method, method_state, status, captured)
VALUES ($1,$2,$3,$4,$5,$6,'upi',$7,'pending_customer_action',false)
`, in.PaymentID, attemptID, in.OrderID, in.MerchantID, in.Amount, in.Currency, MethodStateUPIPendingCustomerAction)
	if err != nil {
		return UPIIntentResult{}, fmt.Errorf("insert upi payment: %w", err)
	}

	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payments.upi_payment_details
(payment_id, merchant_id, order_id, flow_type, mandate_id, qr_mode, qr_payload, qr_image_url, display_name, is_reusable, vpa, provider_status, callback_token, expires_at)
VALUES ($1,$2,$3,$4,NULLIF($5, ''),NULLIF($6, ''),NULLIF($7, ''),NULLIF($8, ''),NULLIF($9, ''),$10,$11,'pending',$12,$13)
`, in.PaymentID, in.MerchantID, in.OrderID, in.FlowType, in.MandateID, in.QRMode, in.QRPayload, in.QRImageURL, in.DisplayName, in.IsReusable, in.VPA, in.CallbackToken, in.ExpiresAt)
	if err != nil {
		return UPIIntentResult{}, fmt.Errorf("insert upi detail: %w", err)
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
			"method":     "upi",
		},
	}); err != nil {
		return UPIIntentResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return UPIIntentResult{}, err
	}

	return UPIIntentResult{
		CaptureResult: CaptureResult{
			PaymentID:   in.PaymentID,
			MerchantID:  in.MerchantID,
			OrderID:     in.OrderID,
			Amount:      in.Amount,
			Currency:    in.Currency,
			Method:      "upi",
			MethodState: MethodStateUPIPendingCustomerAction,
			Status:      StatePendingCustomerAction,
			Captured:    false,
			CreatedAt:   now,
		},
		MandateID:      in.MandateID,
		FlowType:       in.FlowType,
		QRMode:         in.QRMode,
		QRPayload:      in.QRPayload,
		QRImageURL:     in.QRImageURL,
		DisplayName:    in.DisplayName,
		IsReusable:     in.IsReusable,
		VPA:            in.VPA,
		ProviderStatus: UPIProviderStatusPending,
		ExpiresAt:      in.ExpiresAt,
	}, nil
}

func (r *PostgresRepository) AttachUPIIntentGatewayData(ctx context.Context, merchantID, paymentID string, gatewayResult GatewayUPIIntentResult) (UPIIntentResult, error) {
	_, err := r.db.Exec(ctx, `
UPDATE paygate_payments.upi_payment_details
SET deep_link = $3,
    intent_uri = $4,
    gateway_reference = $5,
    qr_payload = COALESCE(NULLIF($6, ''), qr_payload),
    qr_image_url = COALESCE(NULLIF($7, ''), qr_image_url),
    expires_at = $8,
    updated_at = NOW()
WHERE payment_id = $1 AND merchant_id = $2
`, paymentID, merchantID, gatewayResult.DeepLink, gatewayResult.IntentURI, gatewayResult.GatewayReference, gatewayResult.QRPayload, gatewayResult.QRImageURL, gatewayResult.ExpiresAt)
	if err != nil {
		return UPIIntentResult{}, err
	}
	_, err = r.db.Exec(ctx, `
UPDATE paygate_payments.payments
SET gateway_reference = $3,
    method_state = $4,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, paymentID, merchantID, gatewayResult.GatewayReference, MethodStateUPIPendingCustomerAction)
	if err != nil {
		return UPIIntentResult{}, err
	}
	return r.GetUPIIntent(ctx, merchantID, paymentID)
}

func (r *PostgresRepository) GetUPIIntent(ctx context.Context, merchantID, paymentID string) (UPIIntentResult, error) {
	var out UPIIntentResult
	var status string
	var providerStatus string
	err := r.db.QueryRow(ctx, `
SELECT p.id, p.merchant_id, p.order_id, p.amount, p.currency, p.method, COALESCE(p.method_state, ''), COALESCE(p.method_state_reason, ''), p.status, p.captured,
       p.captured_at, p.created_at, p.authorized_at,
       d.flow_type, COALESCE(d.mandate_id, ''), COALESCE(d.qr_mode, ''), COALESCE(d.qr_payload, ''), COALESCE(d.qr_image_url, ''), COALESCE(d.display_name, ''), d.is_reusable,
       d.vpa, COALESCE(d.deep_link, ''), COALESCE(d.intent_uri, ''), COALESCE(d.gateway_reference, ''),
       d.provider_status, d.callback_token, d.expires_at, d.completed_at, d.last_polled_at,
       COALESCE(d.failure_code, ''), COALESCE(d.failure_description, '')
FROM paygate_payments.payments p
JOIN paygate_payments.upi_payment_details d ON d.payment_id = p.id
WHERE p.id = $1 AND p.merchant_id = $2
`, paymentID, merchantID).Scan(
		&out.PaymentID, &out.MerchantID, &out.OrderID, &out.Amount, &out.Currency, &out.Method, &out.MethodState, &out.MethodStateReason, &status, &out.Captured,
		&out.CapturedAt, &out.CreatedAt, &out.AuthorizedAt,
		&out.FlowType, &out.MandateID, &out.QRMode, &out.QRPayload, &out.QRImageURL, &out.DisplayName, &out.IsReusable,
		&out.VPA, &out.DeepLink, &out.IntentURI, &out.GatewayReference,
		&providerStatus, &out.CallbackToken, &out.ExpiresAt, &out.CompletedAt, &out.LastPolledAt,
		&out.FailureCode, &out.FailureDescription,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UPIIntentResult{}, ErrUPIIntentNotFound
		}
		return UPIIntentResult{}, err
	}
	out.Status = PaymentState(status)
	out.ProviderStatus = UPIProviderStatus(providerStatus)
	return out, nil
}

func (r *PostgresRepository) PollUPIIntent(ctx context.Context, merchantID, paymentID string, polledAt time.Time) error {
	cmd, err := r.db.Exec(ctx, `
UPDATE paygate_payments.upi_payment_details
SET last_polled_at = $3,
    updated_at = NOW()
WHERE payment_id = $1 AND merchant_id = $2
`, paymentID, merchantID, polledAt)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrUPIIntentNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkUPIIntentProcessing(ctx context.Context, paymentID string, eventID string) (UPIIntentResult, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return UPIIntentResult{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, terminal, processed, err := r.lockUPIIntentTx(ctx, tx, paymentID, eventID)
	if err != nil {
		return UPIIntentResult{}, false, err
	}
	if !processed || terminal {
		if err := tx.Commit(ctx); err != nil {
			return UPIIntentResult{}, false, err
		}
		return current, false, nil
	}
	if current.Status == StatePendingCustomerAction {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'processing', method_state = $2, updated_at = NOW()
WHERE id = $1
`, paymentID, MethodStateUPIProcessing); err != nil {
			return UPIIntentResult{}, false, err
		}
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.upi_payment_details
SET provider_status = 'pending', updated_at = NOW()
WHERE payment_id = $1
`, paymentID); err != nil {
		return UPIIntentResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UPIIntentResult{}, false, err
	}
	out, err := r.GetUPIIntent(ctx, current.MerchantID, paymentID)
	return out, true, err
}

func (r *PostgresRepository) CompleteUPIIntent(ctx context.Context, paymentID string, eventID string, gatewayReference string, processedAt time.Time) (UPIIntentResult, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return UPIIntentResult{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, terminal, processed, err := r.lockUPIIntentTx(ctx, tx, paymentID, eventID)
	if err != nil {
		return UPIIntentResult{}, false, err
	}
	if !processed || terminal {
		if err := tx.Commit(ctx); err != nil {
			return UPIIntentResult{}, false, err
		}
		return current, false, nil
	}

	fee := current.Amount * 2 / 100
	entries := []ledger.Entry{
		{AccountCode: "CUSTOMER_RECEIVABLE", DebitAmount: current.Amount, Description: "upi intent capture receivable"},
		{AccountCode: "MERCHANT_PAYABLE", CreditAmount: current.Amount - fee, Description: "merchant payable on upi capture"},
		{AccountCode: "PLATFORM_FEE_REVENUE", CreditAmount: fee, Description: "platform fee revenue"},
	}
	if _, err := r.ledger.CreateEntriesTx(ctx, tx, current.MerchantID, "payment", current.PaymentID, "upi payment capture", entries); err != nil {
		return UPIIntentResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET status = 'captured',
    method_state = $5,
    captured = true,
    captured_at = $2,
    gateway_reference = COALESCE(NULLIF($3, ''), gateway_reference),
    fee = $4,
    updated_at = NOW()
WHERE id = $1
`, paymentID, processedAt, gatewayReference, fee, MethodStateUPICollected); err != nil {
		return UPIIntentResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.upi_payment_details
SET provider_status = 'succeeded',
    completed_at = $2,
    gateway_reference = COALESCE(NULLIF($3, ''), gateway_reference),
    updated_at = NOW()
WHERE payment_id = $1
`, paymentID, processedAt, gatewayReference); err != nil {
		return UPIIntentResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_orders.orders
SET status = 'paid', amount_paid = amount, amount_due = 0, updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, current.OrderID, current.MerchantID); err != nil {
		return UPIIntentResult{}, false, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment", AggregateID: paymentID, EventType: "payment.captured", MerchantID: current.MerchantID, Payload: map[string]any{"payment_id": paymentID, "order_id": current.OrderID, "method": "upi"}}); err != nil {
		return UPIIntentResult{}, false, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "order", AggregateID: current.OrderID, EventType: "order.paid", MerchantID: current.MerchantID, Payload: map[string]any{"order_id": current.OrderID, "payment_id": paymentID}}); err != nil {
		return UPIIntentResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UPIIntentResult{}, false, err
	}
	out, err := r.GetUPIIntent(ctx, current.MerchantID, paymentID)
	return out, true, err
}

func (r *PostgresRepository) FailUPIIntent(ctx context.Context, paymentID string, eventID string, providerStatus UPIProviderStatus, errorCode, errorDescription string, processedAt time.Time) (UPIIntentResult, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return UPIIntentResult{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, terminal, processed, err := r.lockUPIIntentTx(ctx, tx, paymentID, eventID)
	if err != nil {
		return UPIIntentResult{}, false, err
	}
	if !processed || terminal {
		if err := tx.Commit(ctx); err != nil {
			return UPIIntentResult{}, false, err
		}
		return current, false, nil
	}
	if providerStatus == "" {
		providerStatus = UPIProviderStatusFailed
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
`, paymentID, errorCode, errorDescription, methodStateForFailure("upi"), errorDescription); err != nil {
		return UPIIntentResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payment_attempts
SET status = 'failed',
    error_code = $2,
    error_description = $3,
    updated_at = NOW()
WHERE payment_id = $1
`, paymentID, errorCode, errorDescription); err != nil {
		return UPIIntentResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.upi_payment_details
SET provider_status = $2,
    completed_at = $3,
    failure_code = $4,
    failure_description = $5,
    updated_at = NOW()
WHERE payment_id = $1
`, paymentID, providerStatus, processedAt, errorCode, errorDescription); err != nil {
		return UPIIntentResult{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_orders.orders
SET status = CASE WHEN status = 'attempted' THEN 'failed' ELSE status END,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, current.OrderID, current.MerchantID); err != nil {
		return UPIIntentResult{}, false, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment", AggregateID: paymentID, EventType: "payment.failed", MerchantID: current.MerchantID, Payload: map[string]any{"payment_id": paymentID, "order_id": current.OrderID, "method": "upi", "error_code": errorCode}}); err != nil {
		return UPIIntentResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UPIIntentResult{}, false, err
	}
	out, err := r.GetUPIIntent(ctx, current.MerchantID, paymentID)
	return out, true, err
}

func (r *PostgresRepository) AbandonUPIIntent(ctx context.Context, merchantID, paymentID, reason string, abandonedAt time.Time) (UPIIntentResult, error) {
	_, _, err := r.FailUPIIntent(ctx, paymentID, "", UPIProviderStatusAbandoned, "CUSTOMER_ABANDONED", reason, abandonedAt)
	if err != nil {
		return UPIIntentResult{}, err
	}
	return r.GetUPIIntent(ctx, merchantID, paymentID)
}

func (r *PostgresRepository) ExpireUPIIntent(ctx context.Context, merchantID, paymentID string, expiredAt time.Time) (UPIIntentResult, error) {
	_, _, err := r.FailUPIIntent(ctx, paymentID, "", UPIProviderStatusExpired, "UPI_INTENT_EXPIRED", "upi intent expired before completion", expiredAt)
	if err != nil {
		return UPIIntentResult{}, err
	}
	return r.GetUPIIntent(ctx, merchantID, paymentID)
}

func (r *PostgresRepository) lockUPIIntentTx(ctx context.Context, tx pgx.Tx, paymentID, eventID string) (UPIIntentResult, bool, bool, error) {
	var current UPIIntentResult
	var status string
	var providerStatus string
	err := tx.QueryRow(ctx, `
SELECT p.id, p.merchant_id, p.order_id, p.amount, p.currency, p.method, COALESCE(p.method_state, ''), COALESCE(p.method_state_reason, ''), p.status, p.captured,
       p.captured_at, p.created_at, p.authorized_at,
       d.flow_type, COALESCE(d.mandate_id, ''), COALESCE(d.qr_mode, ''), COALESCE(d.qr_payload, ''), COALESCE(d.qr_image_url, ''), COALESCE(d.display_name, ''), d.is_reusable,
       d.vpa, COALESCE(d.deep_link, ''), COALESCE(d.intent_uri, ''), COALESCE(d.gateway_reference, ''),
       d.provider_status, d.callback_token, d.expires_at, d.completed_at, d.last_polled_at,
       COALESCE(d.failure_code, ''), COALESCE(d.failure_description, '')
FROM paygate_payments.payments p
JOIN paygate_payments.upi_payment_details d ON d.payment_id = p.id
WHERE p.id = $1
FOR UPDATE
`, paymentID).Scan(
		&current.PaymentID, &current.MerchantID, &current.OrderID, &current.Amount, &current.Currency, &current.Method, &current.MethodState, &current.MethodStateReason, &status, &current.Captured,
		&current.CapturedAt, &current.CreatedAt, &current.AuthorizedAt,
		&current.FlowType, &current.MandateID, &current.QRMode, &current.QRPayload, &current.QRImageURL, &current.DisplayName, &current.IsReusable,
		&current.VPA, &current.DeepLink, &current.IntentURI, &current.GatewayReference,
		&providerStatus, &current.CallbackToken, &current.ExpiresAt, &current.CompletedAt, &current.LastPolledAt,
		&current.FailureCode, &current.FailureDescription,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UPIIntentResult{}, false, false, ErrUPIIntentNotFound
		}
		return UPIIntentResult{}, false, false, err
	}
	current.Status = PaymentState(status)
	current.ProviderStatus = UPIProviderStatus(providerStatus)

	if eventID != "" {
		inserted, err := r.insertUPICallbackEventTx(ctx, tx, paymentID, eventID)
		if err != nil {
			return UPIIntentResult{}, false, false, err
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

func (r *PostgresRepository) insertUPICallbackEventTx(ctx context.Context, tx pgx.Tx, paymentID, eventID string) (bool, error) {
	if eventID == "" {
		return true, nil
	}
	tag, err := tx.Exec(ctx, `
INSERT INTO paygate_payments.upi_callback_events (event_id, payment_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`, eventID, paymentID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresRepository) getUPIIntentByIdempotencyKey(ctx context.Context, merchantID, orderID, idempotencyKey string) (UPIIntentResult, error) {
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
			return UPIIntentResult{}, ErrUPIIntentNotFound
		}
		return UPIIntentResult{}, err
	}
	return r.GetUPIIntent(ctx, merchantID, paymentID)
}
