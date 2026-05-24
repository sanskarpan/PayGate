package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateCustomer(ctx context.Context, customer Customer) (Customer, error) {
	meta, _ := json.Marshal(customer.Metadata)
	customer.ID = idgen.New("cust")
	err := r.db.QueryRow(ctx, `
INSERT INTO paygate_billing.customers
    (id, merchant_id, name, email, phone, external_reference, default_payment_token_id, metadata_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING created_at, updated_at
`, customer.ID, customer.MerchantID, customer.Name, customer.Email, customer.Phone, customer.ExternalReference, customer.DefaultPaymentTokenID, meta).
		Scan(&customer.CreatedAt, &customer.UpdatedAt)
	return customer, err
}

func (r *PostgresRepository) GetCustomer(ctx context.Context, merchantID, customerID string) (Customer, error) {
	var customer Customer
	var meta []byte
	err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, name, email, phone, external_reference, default_payment_token_id, metadata_json, created_at, updated_at
FROM paygate_billing.customers
WHERE merchant_id = $1 AND id = $2
`, merchantID, customerID).Scan(&customer.ID, &customer.MerchantID, &customer.Name, &customer.Email, &customer.Phone, &customer.ExternalReference, &customer.DefaultPaymentTokenID, &meta, &customer.CreatedAt, &customer.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Customer{}, ErrCustomerNotFound
		}
		return Customer{}, err
	}
	_ = json.Unmarshal(meta, &customer.Metadata)
	return customer, nil
}

func (r *PostgresRepository) ListCustomers(ctx context.Context, merchantID string, limit int) ([]Customer, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, name, email, phone, external_reference, default_payment_token_id, metadata_json, created_at, updated_at
FROM paygate_billing.customers
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Customer
	for rows.Next() {
		var customer Customer
		var meta []byte
		if err := rows.Scan(&customer.ID, &customer.MerchantID, &customer.Name, &customer.Email, &customer.Phone, &customer.ExternalReference, &customer.DefaultPaymentTokenID, &meta, &customer.CreatedAt, &customer.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(meta, &customer.Metadata)
		out = append(out, customer)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdateCustomer(ctx context.Context, customer Customer) (Customer, error) {
	meta, _ := json.Marshal(customer.Metadata)
	err := r.db.QueryRow(ctx, `
UPDATE paygate_billing.customers
SET name = $3,
    email = $4,
    phone = $5,
    external_reference = $6,
    default_payment_token_id = $7,
    metadata_json = $8,
    updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
RETURNING created_at, updated_at
`, customer.MerchantID, customer.ID, customer.Name, customer.Email, customer.Phone, customer.ExternalReference, customer.DefaultPaymentTokenID, meta).
		Scan(&customer.CreatedAt, &customer.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Customer{}, ErrCustomerNotFound
		}
		return Customer{}, err
	}
	return customer, nil
}

func scanVirtualAccount(row pgx.Row) (VirtualAccount, error) {
	var account VirtualAccount
	var meta []byte
	err := row.Scan(&account.ID, &account.MerchantID, &account.CustomerID, &account.OrderID, &account.Reference, &account.Provider,
		&account.BankName, &account.AccountNumber, &account.IFSC, &account.UPIVPA, &account.Status, &meta, &account.CreatedAt, &account.UpdatedAt)
	if err != nil {
		return VirtualAccount{}, err
	}
	_ = json.Unmarshal(meta, &account.Metadata)
	return account, nil
}

func (r *PostgresRepository) CreateVirtualAccount(ctx context.Context, account VirtualAccount) (VirtualAccount, error) {
	meta, _ := json.Marshal(account.Metadata)
	account.ID = idgen.New("va")
	account.AccountNumber = generateVirtualAccountNumber(account.ID)
	account.UPIVPA = account.Reference + "@paygate"
	if account.Provider == "" {
		account.Provider = "simulated"
	}
	if account.BankName == "" {
		account.BankName = "PayGate Bank"
	}
	if account.IFSC == "" {
		account.IFSC = "PGAT0001234"
	}
	if account.Status == "" {
		account.Status = VirtualAccountActive
	}
	err := r.db.QueryRow(ctx, `
INSERT INTO paygate_billing.virtual_accounts
    (id, merchant_id, customer_id, order_id, reference, provider, bank_name, account_number, ifsc, upi_vpa, status, metadata_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
RETURNING created_at, updated_at
`, account.ID, account.MerchantID, account.CustomerID, account.OrderID, account.Reference, account.Provider, account.BankName,
		account.AccountNumber, account.IFSC, account.UPIVPA, account.Status, meta).Scan(&account.CreatedAt, &account.UpdatedAt)
	return account, err
}

func (r *PostgresRepository) GetVirtualAccount(ctx context.Context, merchantID, virtualAccountID string) (VirtualAccount, error) {
	account, err := scanVirtualAccount(r.db.QueryRow(ctx, `
SELECT id, merchant_id, customer_id, order_id, reference, provider, bank_name, account_number, ifsc, upi_vpa, status, metadata_json, created_at, updated_at
FROM paygate_billing.virtual_accounts
WHERE merchant_id = $1 AND id = $2
`, merchantID, virtualAccountID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VirtualAccount{}, ErrVirtualAccountNotFound
		}
		return VirtualAccount{}, err
	}
	return account, nil
}

func (r *PostgresRepository) ListVirtualAccounts(ctx context.Context, merchantID string, limit int) ([]VirtualAccount, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, customer_id, order_id, reference, provider, bank_name, account_number, ifsc, upi_vpa, status, metadata_json, created_at, updated_at
FROM paygate_billing.virtual_accounts
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VirtualAccount
	for rows.Next() {
		item, err := scanVirtualAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanInboundCollection(row pgx.Row) (InboundCollection, error) {
	var item InboundCollection
	err := row.Scan(&item.ID, &item.MerchantID, &item.VirtualAccountID, &item.CustomerID, &item.OrderID, &item.Amount, &item.Currency,
		&item.RemitterName, &item.RemitterAccount, &item.RemitterIFSC, &item.RemitterVPA, &item.UTR, &item.Status,
		&item.ReviewNotes, &item.MatchedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return InboundCollection{}, err
	}
	return item, nil
}

func (r *PostgresRepository) CreateInboundCollection(ctx context.Context, collection InboundCollection) (InboundCollection, error) {
	collection.ID = idgen.New("coll")
	if collection.Status == "" {
		collection.Status = CollectionReviewRequired
	}
	err := r.db.QueryRow(ctx, `
INSERT INTO paygate_billing.inbound_collections
    (id, merchant_id, virtual_account_id, customer_id, order_id, amount, currency, remitter_name, remitter_account, remitter_ifsc, remitter_vpa, utr, status, review_notes, matched_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14, CASE WHEN $13 = 'matched' THEN NOW() ELSE NULL END)
RETURNING created_at, updated_at, matched_at
`, collection.ID, collection.MerchantID, collection.VirtualAccountID, collection.CustomerID, collection.OrderID, collection.Amount, collection.Currency,
		collection.RemitterName, collection.RemitterAccount, collection.RemitterIFSC, collection.RemitterVPA, collection.UTR, collection.Status, collection.ReviewNotes).
		Scan(&collection.CreatedAt, &collection.UpdatedAt, &collection.MatchedAt)
	return collection, err
}

func (r *PostgresRepository) GetInboundCollection(ctx context.Context, merchantID, collectionID string) (InboundCollection, error) {
	item, err := scanInboundCollection(r.db.QueryRow(ctx, `
SELECT id, merchant_id, virtual_account_id, customer_id, order_id, amount, currency, remitter_name, remitter_account, remitter_ifsc, remitter_vpa, utr, status, review_notes, matched_at, created_at, updated_at
FROM paygate_billing.inbound_collections
WHERE merchant_id = $1 AND id = $2
`, merchantID, collectionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InboundCollection{}, ErrCollectionNotFound
		}
		return InboundCollection{}, err
	}
	return item, nil
}

func (r *PostgresRepository) ListInboundCollections(ctx context.Context, merchantID string, limit int, reviewOnly bool) ([]InboundCollection, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, virtual_account_id, customer_id, order_id, amount, currency, remitter_name, remitter_account, remitter_ifsc, remitter_vpa, utr, status, review_notes, matched_at, created_at, updated_at
FROM paygate_billing.inbound_collections
WHERE merchant_id = $1
  AND ($2 = FALSE OR status = 'review_required')
ORDER BY created_at DESC
LIMIT $3
`, merchantID, reviewOnly, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InboundCollection
	for rows.Next() {
		item, err := scanInboundCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ReviewInboundCollection(ctx context.Context, merchantID, collectionID, orderID, customerID, notes string) (InboundCollection, error) {
	item, err := scanInboundCollection(r.db.QueryRow(ctx, `
UPDATE paygate_billing.inbound_collections
SET order_id = CASE WHEN $3 <> '' THEN $3 ELSE order_id END,
    customer_id = CASE WHEN $4 <> '' THEN $4 ELSE customer_id END,
    review_notes = $5,
    status = CASE WHEN COALESCE(NULLIF($3, ''), order_id) <> '' THEN 'matched' ELSE status END,
    matched_at = CASE WHEN COALESCE(NULLIF($3, ''), order_id) <> '' THEN COALESCE(matched_at, NOW()) ELSE matched_at END,
    updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
RETURNING id, merchant_id, virtual_account_id, customer_id, order_id, amount, currency, remitter_name, remitter_account, remitter_ifsc, remitter_vpa, utr, status, review_notes, matched_at, created_at, updated_at
`, merchantID, collectionID, orderID, customerID, notes))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InboundCollection{}, ErrCollectionNotFound
		}
		return InboundCollection{}, err
	}
	return item, nil
}

func scanConnectedAccount(row pgx.Row) (ConnectedAccount, error) {
	var item ConnectedAccount
	var meta []byte
	err := row.Scan(&item.ID, &item.MerchantID, &item.LinkedMerchantID, &item.BeneficiaryID, &item.DisplayName, &item.ExternalReference, &item.Status, &meta, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return ConnectedAccount{}, err
	}
	_ = json.Unmarshal(meta, &item.Metadata)
	return item, nil
}

func (r *PostgresRepository) CreateConnectedAccount(ctx context.Context, account ConnectedAccount) (ConnectedAccount, error) {
	meta, _ := json.Marshal(account.Metadata)
	account.ID = idgen.New("ca")
	if account.Status == "" {
		account.Status = ConnectedAccountActive
	}
	err := r.db.QueryRow(ctx, `
INSERT INTO paygate_billing.connected_accounts
    (id, merchant_id, linked_merchant_id, beneficiary_id, display_name, external_reference, status, metadata_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING created_at, updated_at
`, account.ID, account.MerchantID, account.LinkedMerchantID, account.BeneficiaryID, account.DisplayName, account.ExternalReference, account.Status, meta).
		Scan(&account.CreatedAt, &account.UpdatedAt)
	return account, err
}

func (r *PostgresRepository) GetConnectedAccount(ctx context.Context, merchantID, accountID string) (ConnectedAccount, error) {
	item, err := scanConnectedAccount(r.db.QueryRow(ctx, `
SELECT id, merchant_id, linked_merchant_id, beneficiary_id, display_name, external_reference, status, metadata_json, created_at, updated_at
FROM paygate_billing.connected_accounts
WHERE merchant_id = $1 AND id = $2
`, merchantID, accountID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ConnectedAccount{}, ErrConnectedAccountNotFound
		}
		return ConnectedAccount{}, err
	}
	return item, nil
}

func (r *PostgresRepository) ListConnectedAccounts(ctx context.Context, merchantID string, limit int) ([]ConnectedAccount, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, linked_merchant_id, beneficiary_id, display_name, external_reference, status, metadata_json, created_at, updated_at
FROM paygate_billing.connected_accounts
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConnectedAccount
	for rows.Next() {
		item, err := scanConnectedAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func generateVirtualAccountNumber(id string) string {
	suffix := strings.NewReplacer("va_", "", "-", "", "_", "").Replace(id)
	if len(suffix) > 12 {
		suffix = suffix[len(suffix)-12:]
	}
	for len(suffix) < 12 {
		suffix = "0" + suffix
	}
	return "100700" + suffix
}

func (r *PostgresRepository) CreateSubscription(ctx context.Context, subscription Subscription) (Subscription, error) {
	meta, _ := json.Marshal(subscription.Metadata)
	subscription.ID = idgen.New("sub")
	err := r.db.QueryRow(ctx, `
INSERT INTO paygate_billing.subscriptions
    (id, merchant_id, customer_id, plan_name, payment_method_token_id, amount, currency, interval_unit, interval_count,
     status, next_billing_at, max_retry_count, retry_interval_hours, cancel_at_period_end, pause_reason, metadata_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
RETURNING retry_count, created_at, updated_at
`, subscription.ID, subscription.MerchantID, subscription.CustomerID, subscription.PlanName, subscription.PaymentMethodTokenID, subscription.Amount, subscription.Currency, subscription.IntervalUnit, subscription.IntervalCount,
		subscription.Status, subscription.NextBillingAt, subscription.MaxRetryCount, subscription.RetryIntervalHours, subscription.CancelAtPeriodEnd, subscription.PauseReason, meta).
		Scan(&subscription.RetryCount, &subscription.CreatedAt, &subscription.UpdatedAt)
	return subscription, err
}

func scanSubscription(row pgx.Row) (Subscription, error) {
	var subscription Subscription
	var canceledAt *time.Time
	var meta []byte
	err := row.Scan(&subscription.ID, &subscription.MerchantID, &subscription.CustomerID, &subscription.PlanName, &subscription.PaymentMethodTokenID,
		&subscription.Amount, &subscription.Currency, &subscription.IntervalUnit, &subscription.IntervalCount, &subscription.Status,
		&subscription.NextBillingAt, &subscription.RetryCount, &subscription.MaxRetryCount, &subscription.RetryIntervalHours,
		&subscription.CancelAtPeriodEnd, &subscription.PauseReason, &canceledAt, &meta, &subscription.CreatedAt, &subscription.UpdatedAt)
	if err != nil {
		return Subscription{}, err
	}
	subscription.CanceledAt = canceledAt
	_ = json.Unmarshal(meta, &subscription.Metadata)
	return subscription, nil
}

func (r *PostgresRepository) GetSubscription(ctx context.Context, merchantID, subscriptionID string) (Subscription, error) {
	subscription, err := scanSubscription(r.db.QueryRow(ctx, `
SELECT id, merchant_id, customer_id, plan_name, payment_method_token_id, amount, currency, interval_unit, interval_count,
       status, next_billing_at, retry_count, max_retry_count, retry_interval_hours, cancel_at_period_end, pause_reason,
       canceled_at, metadata_json, created_at, updated_at
FROM paygate_billing.subscriptions
WHERE merchant_id = $1 AND id = $2
`, merchantID, subscriptionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Subscription{}, ErrSubscriptionNotFound
		}
		return Subscription{}, err
	}
	return subscription, nil
}

func (r *PostgresRepository) ListSubscriptions(ctx context.Context, merchantID string, limit int) ([]Subscription, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, customer_id, plan_name, payment_method_token_id, amount, currency, interval_unit, interval_count,
       status, next_billing_at, retry_count, max_retry_count, retry_interval_hours, cancel_at_period_end, pause_reason,
       canceled_at, metadata_json, created_at, updated_at
FROM paygate_billing.subscriptions
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, subscription)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdateSubscriptionStatus(ctx context.Context, merchantID, subscriptionID string, status SubscriptionStatus, pauseReason string, cancelAtPeriodEnd bool, nextBillingAt *time.Time) (Subscription, error) {
	query := `
UPDATE paygate_billing.subscriptions
SET status = $3,
    pause_reason = $4,
    cancel_at_period_end = $5,
    next_billing_at = COALESCE($6, next_billing_at),
    canceled_at = CASE WHEN $3 = 'canceled' THEN NOW() ELSE canceled_at END,
    updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
RETURNING id, merchant_id, customer_id, plan_name, payment_method_token_id, amount, currency, interval_unit, interval_count,
       status, next_billing_at, retry_count, max_retry_count, retry_interval_hours, cancel_at_period_end, pause_reason,
       canceled_at, metadata_json, created_at, updated_at`
	subscription, err := scanSubscription(r.db.QueryRow(ctx, query, merchantID, subscriptionID, status, pauseReason, cancelAtPeriodEnd, nextBillingAt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Subscription{}, ErrSubscriptionNotFound
		}
		return Subscription{}, err
	}
	return subscription, nil
}

func (r *PostgresRepository) LeaseDueSubscriptions(ctx context.Context, before time.Time, limit int) ([]Subscription, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, customer_id, plan_name, payment_method_token_id, amount, currency, interval_unit, interval_count,
       status, next_billing_at, retry_count, max_retry_count, retry_interval_hours, cancel_at_period_end, pause_reason,
       canceled_at, metadata_json, created_at, updated_at
FROM paygate_billing.subscriptions
WHERE status = 'active' AND next_billing_at <= $1
ORDER BY next_billing_at ASC
LIMIT $2
`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, subscription)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CreateInvoice(ctx context.Context, invoice Invoice) (Invoice, error) {
	invoice.ID = idgen.New("inv")
	err := r.db.QueryRow(ctx, `
INSERT INTO paygate_billing.invoices
    (id, merchant_id, customer_id, subscription_id, amount, currency, status, billing_reason, period_start, period_end, due_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING created_at, updated_at
`, invoice.ID, invoice.MerchantID, invoice.CustomerID, invoice.SubscriptionID, invoice.Amount, invoice.Currency, invoice.Status, invoice.BillingReason, invoice.PeriodStart, invoice.PeriodEnd, invoice.DueAt).
		Scan(&invoice.CreatedAt, &invoice.UpdatedAt)
	return invoice, err
}

func scanInvoice(row pgx.Row) (Invoice, error) {
	var invoice Invoice
	err := row.Scan(&invoice.ID, &invoice.MerchantID, &invoice.CustomerID, &invoice.SubscriptionID, &invoice.Amount, &invoice.Currency, &invoice.Status,
		&invoice.BillingReason, &invoice.PeriodStart, &invoice.PeriodEnd, &invoice.DueAt, &invoice.OrderID, &invoice.PaymentID,
		&invoice.FailureCode, &invoice.FailureMessage, &invoice.CreatedAt, &invoice.UpdatedAt)
	return invoice, err
}

func (r *PostgresRepository) GetInvoice(ctx context.Context, merchantID, invoiceID string) (Invoice, error) {
	invoice, err := scanInvoice(r.db.QueryRow(ctx, `
SELECT id, merchant_id, customer_id, subscription_id, amount, currency, status, billing_reason, period_start, period_end, due_at,
       order_id, payment_id, failure_code, failure_message, created_at, updated_at
FROM paygate_billing.invoices
WHERE merchant_id = $1 AND id = $2
`, merchantID, invoiceID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invoice{}, ErrInvoiceNotFound
		}
		return Invoice{}, err
	}
	return invoice, nil
}

func (r *PostgresRepository) ListInvoices(ctx context.Context, merchantID, subscriptionID string, limit int) ([]Invoice, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, customer_id, subscription_id, amount, currency, status, billing_reason, period_start, period_end, due_at,
       order_id, payment_id, failure_code, failure_message, created_at, updated_at
FROM paygate_billing.invoices
WHERE merchant_id = $1 AND ($2 = '' OR subscription_id = $2)
ORDER BY created_at DESC
LIMIT $3
`, merchantID, subscriptionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invoice
	for rows.Next() {
		invoice, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, invoice)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CreateInvoiceAttempt(ctx context.Context, attempt InvoiceAttempt) (InvoiceAttempt, error) {
	attempt.ID = idgen.New("invatt")
	err := r.db.QueryRow(ctx, `
INSERT INTO paygate_billing.invoice_attempts
    (id, invoice_id, merchant_id, subscription_id, attempt_number, status, order_id, payment_id, failure_code, failure_message)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING created_at
`, attempt.ID, attempt.InvoiceID, attempt.MerchantID, attempt.SubscriptionID, attempt.AttemptNumber, attempt.Status, attempt.OrderID, attempt.PaymentID, attempt.FailureCode, attempt.FailureMessage).
		Scan(&attempt.CreatedAt)
	return attempt, err
}

func (r *PostgresRepository) MarkInvoiceAttempt(ctx context.Context, merchantID, attemptID string, status InvoiceAttemptStatus, orderID, paymentID, failureCode, failureMessage string) error {
	cmd, err := r.db.Exec(ctx, `
UPDATE paygate_billing.invoice_attempts
SET status = $3,
    order_id = CASE WHEN $4 <> '' THEN $4 ELSE order_id END,
    payment_id = CASE WHEN $5 <> '' THEN $5 ELSE payment_id END,
    failure_code = $6,
    failure_message = $7
WHERE merchant_id = $1 AND id = $2
`, merchantID, attemptID, status, orderID, paymentID, failureCode, failureMessage)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvoiceNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkInvoicePaid(ctx context.Context, merchantID, invoiceID, orderID, paymentID string, nextBillingAt time.Time) (Invoice, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Invoice{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var subscriptionID string
	var invoice Invoice
	err = tx.QueryRow(ctx, `
UPDATE paygate_billing.invoices
SET status = 'paid',
    order_id = $3,
    payment_id = $4,
    failure_code = '',
    failure_message = '',
    updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
RETURNING id, merchant_id, customer_id, subscription_id, amount, currency, status, billing_reason, period_start, period_end, due_at,
       order_id, payment_id, failure_code, failure_message, created_at, updated_at
`, merchantID, invoiceID, orderID, paymentID).Scan(&invoice.ID, &invoice.MerchantID, &invoice.CustomerID, &subscriptionID, &invoice.Amount, &invoice.Currency, &invoice.Status, &invoice.BillingReason, &invoice.PeriodStart, &invoice.PeriodEnd, &invoice.DueAt, &invoice.OrderID, &invoice.PaymentID, &invoice.FailureCode, &invoice.FailureMessage, &invoice.CreatedAt, &invoice.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invoice{}, ErrInvoiceNotFound
		}
		return Invoice{}, err
	}
	invoice.SubscriptionID = subscriptionID
	if _, err := tx.Exec(ctx, `
UPDATE paygate_billing.subscriptions
SET retry_count = 0,
    next_billing_at = $3,
    status = CASE WHEN cancel_at_period_end THEN 'canceled' ELSE status END,
    canceled_at = CASE WHEN cancel_at_period_end THEN NOW() ELSE canceled_at END,
    updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
`, merchantID, subscriptionID, nextBillingAt); err != nil {
		return Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, err
	}
	return invoice, nil
}

func (r *PostgresRepository) MarkInvoiceFailed(ctx context.Context, merchantID, invoiceID, failureCode, failureMessage string, nextBillingAt time.Time, retryCount int) (Invoice, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Invoice{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var subscriptionID string
	var invoice Invoice
	err = tx.QueryRow(ctx, `
UPDATE paygate_billing.invoices
SET status = 'failed',
    failure_code = $3,
    failure_message = $4,
    updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
RETURNING id, merchant_id, customer_id, subscription_id, amount, currency, status, billing_reason, period_start, period_end, due_at,
       order_id, payment_id, failure_code, failure_message, created_at, updated_at
`, merchantID, invoiceID, failureCode, failureMessage).Scan(&invoice.ID, &invoice.MerchantID, &invoice.CustomerID, &subscriptionID, &invoice.Amount, &invoice.Currency, &invoice.Status, &invoice.BillingReason, &invoice.PeriodStart, &invoice.PeriodEnd, &invoice.DueAt, &invoice.OrderID, &invoice.PaymentID, &invoice.FailureCode, &invoice.FailureMessage, &invoice.CreatedAt, &invoice.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invoice{}, ErrInvoiceNotFound
		}
		return Invoice{}, err
	}
	invoice.SubscriptionID = subscriptionID
	if _, err := tx.Exec(ctx, `
UPDATE paygate_billing.subscriptions
SET retry_count = $3,
    next_billing_at = $4,
    updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
`, merchantID, subscriptionID, retryCount, nextBillingAt); err != nil {
		return Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, err
	}
	return invoice, nil
}

func nextAttemptNumber(existing []InvoiceAttempt) int {
	return len(existing) + 1
}

func (r *PostgresRepository) debugString() string {
	return fmt.Sprintf("billing-postgres:%p", r)
}
