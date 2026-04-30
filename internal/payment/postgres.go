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
(id, order_id, merchant_id, amount, currency, method, status, error_code, error_description, idempotency_key)
VALUES ($1,$2,$3,$4,$5,$6,'failed',$7,$8,$9)
`, attemptID, in.OrderID, in.MerchantID, in.Amount, in.Currency, in.Method, errorCode, errorDescription, in.IdempotencyKey)
	if err != nil {
		return err
	}
	_, _ = tx.Exec(ctx, `
UPDATE paygate_orders.orders
SET status = CASE WHEN status = 'created' THEN 'attempted' ELSE status END,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, in.OrderID, in.MerchantID)
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment_attempt", AggregateID: attemptID, EventType: "payment.failed", MerchantID: in.MerchantID, Payload: map[string]any{"order_id": in.OrderID, "error_code": errorCode}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) CreateAuthorizedPayment(ctx context.Context, in CreateAuthorizedInput) (CaptureResult, error) {
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
	paymentID := idgen.New("pay")
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payments.payment_attempts
(id, order_id, merchant_id, payment_id, amount, currency, method, status, gateway_reference, idempotency_key)
VALUES ($1,$2,$3,$4,$5,$6,$7,'created',$8,$9)
`, attemptID, in.OrderID, in.MerchantID, paymentID, in.Amount, in.Currency, in.Method, in.GatewayReference, in.IdempotencyKey)
	if err != nil {
		var pgErr *pgconn.PgError
		if in.IdempotencyKey != "" && errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return r.getPaymentByIdempotencyKey(ctx, in.MerchantID, in.OrderID, in.IdempotencyKey)
		}
		return CaptureResult{}, fmt.Errorf("insert payment attempt: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE paygate_payments.payment_attempts SET status='processing', updated_at=NOW() WHERE id=$1`, attemptID)
	if err != nil {
		return CaptureResult{}, err
	}

	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payments.payments
(id, attempt_id, order_id, merchant_id, amount, currency, method, status, captured, gateway_reference, auth_code, authorized_at, auto_capture_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,'created',false,$8,$9,$10,$11)
`, paymentID, attemptID, in.OrderID, in.MerchantID, in.Amount, in.Currency, in.Method, in.GatewayReference, in.AuthCode, now, in.AutoCaptureAt)
	if err != nil {
		return CaptureResult{}, fmt.Errorf("insert payment: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE paygate_payments.payments SET status='authorized', updated_at=NOW() WHERE id=$1`, paymentID)
	if err != nil {
		return CaptureResult{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE paygate_payments.payment_attempts SET status='authorized', updated_at=NOW() WHERE id=$1`, attemptID)
	if err != nil {
		return CaptureResult{}, err
	}

	_, _ = tx.Exec(ctx, `
UPDATE paygate_orders.orders
SET status = CASE WHEN status = 'created' THEN 'attempted' ELSE status END,
    updated_at = NOW()
WHERE id=$1 AND merchant_id=$2
`, in.OrderID, in.MerchantID)

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{AggregateType: "payment", AggregateID: paymentID, EventType: "payment.authorized", MerchantID: in.MerchantID, Payload: map[string]any{"payment_id": paymentID, "order_id": in.OrderID}}); err != nil {
		return CaptureResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CaptureResult{}, err
	}

	return CaptureResult{PaymentID: paymentID, MerchantID: in.MerchantID, OrderID: in.OrderID, Amount: in.Amount, Currency: in.Currency, Method: in.Method, Status: StateAuthorized, Captured: false, AuthorizedAt: &now, CreatedAt: now}, nil
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
SELECT id, merchant_id, order_id, amount, currency, method, status, captured, created_at, authorized_at
FROM paygate_payments.payments
WHERE id=$1 AND merchant_id=$2 FOR UPDATE
`, paymentID, merchantID).Scan(&current.PaymentID, &current.MerchantID, &current.OrderID, &current.Amount, &current.Currency, &current.Method, &status, &current.Captured, &current.CreatedAt, &current.AuthorizedAt)
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
	entries := []ledger.Entry{
		{AccountCode: "CUSTOMER_RECEIVABLE", DebitAmount: amount, Description: "payment capture receivable"},
		{AccountCode: "MERCHANT_PAYABLE", CreditAmount: amount - fee, Description: "merchant payable on capture"},
		{AccountCode: "PLATFORM_FEE_REVENUE", CreditAmount: fee, Description: "platform fee revenue"},
	}
	if _, err := r.ledger.CreateEntriesTx(ctx, tx, current.MerchantID, "payment", current.PaymentID, "payment capture", entries); err != nil {
		return CaptureResult{}, err
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `UPDATE paygate_payments.payments SET status='captured', captured=true, captured_at=$2, updated_at=NOW(), fee=$3 WHERE id=$1`, paymentID, now, fee)
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
	current.Status = StateCaptured
	current.Captured = true
	current.CapturedAt = &now
	return current, nil
}

func (r *PostgresRepository) GetPayment(ctx context.Context, merchantID, paymentID string) (CaptureResult, error) {
	var out CaptureResult
	var status string
	err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, order_id, amount, currency, method, status, captured, captured_at, created_at, authorized_at
FROM paygate_payments.payments
WHERE id=$1 AND merchant_id=$2
`, paymentID, merchantID).Scan(&out.PaymentID, &out.MerchantID, &out.OrderID, &out.Amount, &out.Currency, &out.Method, &status, &out.Captured, &out.CapturedAt, &out.CreatedAt, &out.AuthorizedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrPaymentNotFound
		}
		return CaptureResult{}, err
	}
	out.Status = PaymentState(status)
	return out, nil
}

func (r *PostgresRepository) ListPayments(ctx context.Context, f ListFilter) (ListResult, error) {
	if f.Count <= 0 || f.Count > 100 {
		f.Count = 20
	}
	args := []any{f.MerchantID}
	query := `
SELECT id, merchant_id, order_id, amount, currency, method, status, captured, captured_at, created_at, authorized_at
FROM paygate_payments.payments
WHERE merchant_id = $1`
	if f.OrderID != "" {
		query += ` AND order_id = $2`
		args = append(args, f.OrderID)
	}
	query += ` ORDER BY created_at DESC LIMIT `
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
			&status,
			&item.Captured,
			&item.CapturedAt,
			&item.CreatedAt,
			&item.AuthorizedAt,
		); err != nil {
			return ListResult{}, err
		}
		item.Status = PaymentState(status)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items}, nil
}

func (r *PostgresRepository) AutoCaptureDue(ctx context.Context) (int64, error) {
	rows, err := r.db.Query(ctx, `SELECT id, amount FROM paygate_payments.payments WHERE status='authorized' AND auto_capture_at IS NOT NULL AND auto_capture_at <= NOW() LIMIT 50`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var count int64
	for rows.Next() {
		var id string
		var amount int64
		if err := rows.Scan(&id, &amount); err != nil {
			return count, err
		}
		var merchantID string
		err = r.db.QueryRow(ctx, `SELECT merchant_id FROM paygate_payments.payments WHERE id = $1`, id).Scan(&merchantID)
		if err != nil {
			return count, err
		}
		if _, err := r.CaptureAuthorizedPayment(ctx, merchantID, id, amount); err == nil {
			count++
		}
	}
	return count, rows.Err()
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
SET status='auto_refunded', error_code='AUTH_WINDOW_EXPIRED', error_description='capture window expired', updated_at=NOW()
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
	var paymentID string
	if err := r.db.QueryRow(ctx, `
SELECT payment_id
FROM paygate_payments.payment_attempts
WHERE merchant_id = $1 AND order_id = $2 AND idempotency_key = $3
ORDER BY created_at DESC
LIMIT 1
`, merchantID, orderID, idempotencyKey).Scan(&paymentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrPaymentNotFound
		}
		return CaptureResult{}, err
	}
	return r.GetPayment(ctx, merchantID, paymentID)
}
