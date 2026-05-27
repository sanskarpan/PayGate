package billing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/common/protect"
	"github.com/sanskarpan/PayGate/internal/outbox"
)

type PostgresRepository struct {
	db        *pgxpool.Pool
	outbox    *outbox.Writer
	protector *protect.Protector
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db, outbox: outbox.NewWriter(), protector: protect.Default()}
}

func (r *PostgresRepository) CreateCustomer(ctx context.Context, customer Customer) (Customer, error) {
	meta, _ := json.Marshal(customer.Metadata)
	customer.ID = idgen.New("cust")
	email, err := r.protector.SealStringForDomain(protect.DomainBillingCustomerPII, customer.Email)
	if err != nil {
		return Customer{}, err
	}
	phone, err := r.protector.SealStringForDomain(protect.DomainBillingCustomerPII, customer.Phone)
	if err != nil {
		return Customer{}, err
	}
	externalReference, err := r.protector.SealStringForDomain(protect.DomainBillingCustomerPII, customer.ExternalReference)
	if err != nil {
		return Customer{}, err
	}
	err = r.db.QueryRow(ctx, `
INSERT INTO paygate_billing.customers
    (id, merchant_id, name, email, phone, external_reference, default_payment_token_id, metadata_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING created_at, updated_at
`, customer.ID, customer.MerchantID, customer.Name, email, phone, externalReference, customer.DefaultPaymentTokenID, meta).
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
	customer, err = r.decryptCustomer(customer)
	if err != nil {
		return Customer{}, err
	}
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
		customer, err = r.decryptCustomer(customer)
		if err != nil {
			return nil, err
		}
		out = append(out, customer)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdateCustomer(ctx context.Context, customer Customer) (Customer, error) {
	meta, _ := json.Marshal(customer.Metadata)
	email, err := r.protector.SealStringForDomain(protect.DomainBillingCustomerPII, customer.Email)
	if err != nil {
		return Customer{}, err
	}
	phone, err := r.protector.SealStringForDomain(protect.DomainBillingCustomerPII, customer.Phone)
	if err != nil {
		return Customer{}, err
	}
	externalReference, err := r.protector.SealStringForDomain(protect.DomainBillingCustomerPII, customer.ExternalReference)
	if err != nil {
		return Customer{}, err
	}
	err = r.db.QueryRow(ctx, `
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
`, customer.MerchantID, customer.ID, customer.Name, email, phone, externalReference, customer.DefaultPaymentTokenID, meta).
		Scan(&customer.CreatedAt, &customer.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Customer{}, ErrCustomerNotFound
		}
		return Customer{}, err
	}
	return customer, nil
}

func (r *PostgresRepository) decryptCustomer(customer Customer) (Customer, error) {
	var err error
	customer.Email, err = r.protector.OpenStringForDomain(protect.DomainBillingCustomerPII, customer.Email)
	if err != nil {
		return Customer{}, err
	}
	customer.Phone, err = r.protector.OpenStringForDomain(protect.DomainBillingCustomerPII, customer.Phone)
	if err != nil {
		return Customer{}, err
	}
	customer.ExternalReference, err = r.protector.OpenStringForDomain(protect.DomainBillingCustomerPII, customer.ExternalReference)
	if err != nil {
		return Customer{}, err
	}
	return customer, nil
}

func scanUPIMandate(row pgx.Row) (UPIMandate, error) {
	var mandate UPIMandate
	var meta []byte
	err := row.Scan(
		&mandate.ID, &mandate.MerchantID, &mandate.CustomerID, &mandate.Reference, &mandate.DisplayName, &mandate.VPA,
		&mandate.AmountLimit, &mandate.Currency, &mandate.IntervalUnit, &mandate.IntervalCount, &mandate.RetryWindowHours,
		&mandate.Status, &mandate.ApprovalToken, &mandate.ApprovedAt, &mandate.PausedAt, &mandate.RevokedAt, &mandate.ExpiresAt,
		&meta, &mandate.CreatedAt, &mandate.UpdatedAt,
	)
	if err != nil {
		return UPIMandate{}, err
	}
	_ = json.Unmarshal(meta, &mandate.Metadata)
	return mandate, nil
}

func scanUPIMandateEvent(row pgx.Row) (UPIMandateEvent, error) {
	var event UPIMandateEvent
	var meta []byte
	err := row.Scan(&event.ID, &event.MerchantID, &event.MandateID, &event.EventType, &event.ActorType, &event.ActorID, &event.Reason, &event.PaymentID, &meta, &event.CreatedAt)
	if err != nil {
		return UPIMandateEvent{}, err
	}
	_ = json.Unmarshal(meta, &event.Metadata)
	return event, nil
}

func (r *PostgresRepository) CreateUPIMandate(ctx context.Context, mandate UPIMandate, actorType, actorID string) (UPIMandate, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return UPIMandate{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	meta, _ := json.Marshal(mandate.Metadata)
	mandate.ID = idgen.New("mandate")
	if mandate.Status == "" {
		mandate.Status = UPIMandatePendingApproval
	}
	err = tx.QueryRow(ctx, `
INSERT INTO paygate_billing.upi_mandates
    (id, merchant_id, customer_id, reference, display_name, vpa, amount_limit, currency, interval_unit, interval_count, retry_window_hours, status, approval_token, expires_at, metadata_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
RETURNING approved_at, paused_at, revoked_at, created_at, updated_at
`, mandate.ID, mandate.MerchantID, mandate.CustomerID, mandate.Reference, mandate.DisplayName, mandate.VPA, mandate.AmountLimit, mandate.Currency, mandate.IntervalUnit, mandate.IntervalCount, mandate.RetryWindowHours, mandate.Status, mandate.ApprovalToken, mandate.ExpiresAt, meta).
		Scan(&mandate.ApprovedAt, &mandate.PausedAt, &mandate.RevokedAt, &mandate.CreatedAt, &mandate.UpdatedAt)
	if err != nil {
		return UPIMandate{}, err
	}
	if err := r.insertUPIMandateEventTx(ctx, tx, mandate.MerchantID, mandate.ID, MandateEventCreated, actorType, actorID, "", "", nil); err != nil {
		return UPIMandate{}, err
	}
	if err := r.writeUPIMandateOutboxTx(ctx, tx, mandate, "upi_mandate.created"); err != nil {
		return UPIMandate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UPIMandate{}, err
	}
	return mandate, nil
}

func (r *PostgresRepository) GetUPIMandate(ctx context.Context, merchantID, mandateID string) (UPIMandate, error) {
	mandate, err := scanUPIMandate(r.db.QueryRow(ctx, `
SELECT id, merchant_id, customer_id, reference, display_name, vpa, amount_limit, currency, interval_unit, interval_count,
       retry_window_hours, status, COALESCE(approval_token, ''), approved_at, paused_at, revoked_at, expires_at,
       metadata_json, created_at, updated_at
FROM paygate_billing.upi_mandates
WHERE merchant_id = $1 AND id = $2
`, merchantID, mandateID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UPIMandate{}, ErrUPIMandateNotFound
		}
		return UPIMandate{}, err
	}
	return mandate, nil
}

func (r *PostgresRepository) ListUPIMandates(ctx context.Context, merchantID string, limit int) ([]UPIMandate, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, customer_id, reference, display_name, vpa, amount_limit, currency, interval_unit, interval_count,
       retry_window_hours, status, COALESCE(approval_token, ''), approved_at, paused_at, revoked_at, expires_at,
       metadata_json, created_at, updated_at
FROM paygate_billing.upi_mandates
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UPIMandate
	for rows.Next() {
		item, err := scanUPIMandate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdateUPIMandateStatus(ctx context.Context, merchantID, mandateID string, status UPIMandateStatus, actorType, actorID, reason string) (UPIMandate, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return UPIMandate{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var eventType UPIMandateEventType
	switch status {
	case UPIMandateActive:
		eventType = MandateEventActivated
	case UPIMandatePaused:
		eventType = MandateEventPaused
	case UPIMandateRevoked:
		eventType = MandateEventRevoked
	case UPIMandateExpired:
		eventType = MandateEventExpired
	default:
		return UPIMandate{}, ErrInvalidUPIMandate
	}
	var mandate UPIMandate
	query := `
UPDATE paygate_billing.upi_mandates
SET status = $3,
    approved_at = CASE WHEN $3 = 'active' AND approved_at IS NULL THEN NOW() ELSE approved_at END,
    paused_at = CASE WHEN $3 = 'paused' THEN NOW() WHEN $3 = 'active' THEN NULL ELSE paused_at END,
    revoked_at = CASE WHEN $3 = 'revoked' THEN NOW() ELSE revoked_at END,
    approval_token = CASE WHEN $3 = 'active' THEN '' ELSE approval_token END,
    updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
RETURNING id, merchant_id, customer_id, reference, display_name, vpa, amount_limit, currency, interval_unit, interval_count,
       retry_window_hours, status, COALESCE(approval_token, ''), approved_at, paused_at, revoked_at, expires_at,
       metadata_json, created_at, updated_at`
	mandate, err = scanUPIMandate(tx.QueryRow(ctx, query, merchantID, mandateID, status))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UPIMandate{}, ErrUPIMandateNotFound
		}
		return UPIMandate{}, err
	}
	if err := r.insertUPIMandateEventTx(ctx, tx, merchantID, mandateID, eventType, actorType, actorID, reason, "", nil); err != nil {
		return UPIMandate{}, err
	}
	eventName := map[UPIMandateEventType]string{
		MandateEventActivated: "upi_mandate.activated",
		MandateEventPaused:    "upi_mandate.paused",
		MandateEventRevoked:   "upi_mandate.revoked",
		MandateEventExpired:   "upi_mandate.expired",
	}[eventType]
	if err := r.writeUPIMandateOutboxTx(ctx, tx, mandate, eventName); err != nil {
		return UPIMandate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UPIMandate{}, err
	}
	return mandate, nil
}

func (r *PostgresRepository) ListUPIMandateEvents(ctx context.Context, merchantID, mandateID string, limit int) ([]UPIMandateEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, mandate_id, event_type, actor_type, actor_id, COALESCE(reason, ''), COALESCE(payment_id, ''), metadata_json, created_at
FROM paygate_billing.upi_mandate_events
WHERE merchant_id = $1 AND mandate_id = $2
ORDER BY created_at DESC
LIMIT $3
`, merchantID, mandateID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UPIMandateEvent
	for rows.Next() {
		item, err := scanUPIMandateEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) RecordUPIMandateChargeResult(ctx context.Context, merchantID, mandateID string, eventType UPIMandateEventType, paymentID, reason string, metadata map[string]any) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := r.insertUPIMandateEventTx(ctx, tx, merchantID, mandateID, eventType, "system", "subscription_runner", reason, paymentID, metadata); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (r *PostgresRepository) insertUPIMandateEventTx(ctx context.Context, tx pgx.Tx, merchantID, mandateID string, eventType UPIMandateEventType, actorType, actorID, reason, paymentID string, metadata map[string]any) error {
	meta, _ := json.Marshal(metadata)
	_, err := tx.Exec(ctx, `
INSERT INTO paygate_billing.upi_mandate_events
    (id, merchant_id, mandate_id, event_type, actor_type, actor_id, reason, payment_id, metadata_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8, ''),$9)
`, idgen.New("mtev"), merchantID, mandateID, eventType, actorType, actorID, reason, paymentID, meta)
	return err
}

func (r *PostgresRepository) writeUPIMandateOutboxTx(ctx context.Context, tx pgx.Tx, mandate UPIMandate, eventType string) error {
	if r.outbox == nil {
		return nil
	}
	return r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "upi_mandate",
		AggregateID:   mandate.ID,
		EventType:     eventType,
		MerchantID:    mandate.MerchantID,
		Payload: map[string]any{
			"mandate_id":         mandate.ID,
			"customer_id":        mandate.CustomerID,
			"status":             mandate.Status,
			"reference":          mandate.Reference,
			"amount_limit":       mandate.AmountLimit,
			"currency":           mandate.Currency,
			"interval_unit":      mandate.IntervalUnit,
			"interval_count":     mandate.IntervalCount,
			"retry_window_hours": mandate.RetryWindowHours,
		},
	})
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
	remitterAccount, err := r.protector.SealStringForDomain(protect.DomainBillingInboundCollection, collection.RemitterAccount)
	if err != nil {
		return InboundCollection{}, err
	}
	remitterIFSC, err := r.protector.SealStringForDomain(protect.DomainBillingInboundCollection, collection.RemitterIFSC)
	if err != nil {
		return InboundCollection{}, err
	}
	remitterVPA, err := r.protector.SealStringForDomain(protect.DomainBillingInboundCollection, collection.RemitterVPA)
	if err != nil {
		return InboundCollection{}, err
	}
	err = r.db.QueryRow(ctx, `
INSERT INTO paygate_billing.inbound_collections
    (id, merchant_id, virtual_account_id, customer_id, order_id, amount, currency, remitter_name, remitter_account, remitter_ifsc, remitter_vpa, utr, status, review_notes, matched_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14, CASE WHEN $13 = 'matched' THEN NOW() ELSE NULL END)
RETURNING created_at, updated_at, matched_at
`, collection.ID, collection.MerchantID, collection.VirtualAccountID, collection.CustomerID, collection.OrderID, collection.Amount, collection.Currency,
		collection.RemitterName, remitterAccount, remitterIFSC, remitterVPA, collection.UTR, collection.Status, collection.ReviewNotes).
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
	return r.decryptInboundCollection(item)
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
		item, err = r.decryptInboundCollection(item)
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
	return r.decryptInboundCollection(item)
}

func (r *PostgresRepository) decryptInboundCollection(item InboundCollection) (InboundCollection, error) {
	var err error
	item.RemitterAccount, err = r.protector.OpenStringForDomain(protect.DomainBillingInboundCollection, item.RemitterAccount)
	if err != nil {
		return InboundCollection{}, err
	}
	item.RemitterIFSC, err = r.protector.OpenStringForDomain(protect.DomainBillingInboundCollection, item.RemitterIFSC)
	if err != nil {
		return InboundCollection{}, err
	}
	item.RemitterVPA, err = r.protector.OpenStringForDomain(protect.DomainBillingInboundCollection, item.RemitterVPA)
	if err != nil {
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
	externalReference, err := r.protector.SealStringForDomain(protect.DomainBillingConnectedAccount, account.ExternalReference)
	if err != nil {
		return ConnectedAccount{}, err
	}
	err = r.db.QueryRow(ctx, `
INSERT INTO paygate_billing.connected_accounts
    (id, merchant_id, linked_merchant_id, beneficiary_id, display_name, external_reference, status, metadata_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING created_at, updated_at
`, account.ID, account.MerchantID, account.LinkedMerchantID, account.BeneficiaryID, account.DisplayName, externalReference, account.Status, meta).
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
	return r.decryptConnectedAccount(item)
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
		item, err = r.decryptConnectedAccount(item)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) decryptConnectedAccount(item ConnectedAccount) (ConnectedAccount, error) {
	var err error
	item.ExternalReference, err = r.protector.OpenStringForDomain(protect.DomainBillingConnectedAccount, item.ExternalReference)
	if err != nil {
		return ConnectedAccount{}, err
	}
	return item, nil
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
    (id, merchant_id, customer_id, plan_name, collection_method, payment_method_token_id, upi_mandate_id, amount, currency, interval_unit, interval_count,
     status, next_billing_at, max_retry_count, retry_interval_hours, cancel_at_period_end, pause_reason, metadata_json)
VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7, ''),$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
RETURNING retry_count, created_at, updated_at
`, subscription.ID, subscription.MerchantID, subscription.CustomerID, subscription.PlanName, subscription.CollectionMethod, subscription.PaymentMethodTokenID, subscription.UPIMandateID, subscription.Amount, subscription.Currency, subscription.IntervalUnit, subscription.IntervalCount,
		subscription.Status, subscription.NextBillingAt, subscription.MaxRetryCount, subscription.RetryIntervalHours, subscription.CancelAtPeriodEnd, subscription.PauseReason, meta).
		Scan(&subscription.RetryCount, &subscription.CreatedAt, &subscription.UpdatedAt)
	return subscription, err
}

func scanSubscription(row pgx.Row) (Subscription, error) {
	var subscription Subscription
	var canceledAt *time.Time
	var meta []byte
	err := row.Scan(&subscription.ID, &subscription.MerchantID, &subscription.CustomerID, &subscription.PlanName, &subscription.CollectionMethod, &subscription.PaymentMethodTokenID, &subscription.UPIMandateID,
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
SELECT id, merchant_id, customer_id, plan_name, collection_method, payment_method_token_id, COALESCE(upi_mandate_id, ''), amount, currency, interval_unit, interval_count,
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
SELECT id, merchant_id, customer_id, plan_name, collection_method, payment_method_token_id, COALESCE(upi_mandate_id, ''), amount, currency, interval_unit, interval_count,
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
RETURNING id, merchant_id, customer_id, plan_name, collection_method, payment_method_token_id, COALESCE(upi_mandate_id, ''), amount, currency, interval_unit, interval_count,
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
SELECT id, merchant_id, customer_id, plan_name, collection_method, payment_method_token_id, COALESCE(upi_mandate_id, ''), amount, currency, interval_unit, interval_count,
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
    (id, merchant_id, customer_id, subscription_id, external_reference, description, amount, currency, status, billing_reason, period_start, period_end, due_at, order_id, payment_id, payment_link_id, virtual_account_id, reminder_count)
VALUES ($1,$2,$3,NULLIF($4, ''),$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
RETURNING created_at, updated_at
`, invoice.ID, invoice.MerchantID, invoice.CustomerID, invoice.SubscriptionID, invoice.ExternalReference, invoice.Description, invoice.Amount, invoice.Currency, invoice.Status, invoice.BillingReason, invoice.PeriodStart, invoice.PeriodEnd, invoice.DueAt, invoice.OrderID, invoice.PaymentID, invoice.PaymentLinkID, invoice.VirtualAccountID, invoice.ReminderCount).
		Scan(&invoice.CreatedAt, &invoice.UpdatedAt)
	return invoice, err
}

func scanInvoice(row pgx.Row) (Invoice, error) {
	var invoice Invoice
	err := row.Scan(&invoice.ID, &invoice.MerchantID, &invoice.CustomerID, &invoice.SubscriptionID, &invoice.ExternalReference, &invoice.Description, &invoice.Amount, &invoice.Currency, &invoice.Status,
		&invoice.BillingReason, &invoice.PeriodStart, &invoice.PeriodEnd, &invoice.DueAt, &invoice.OrderID, &invoice.PaymentID, &invoice.PaymentLinkID, &invoice.VirtualAccountID, &invoice.ReminderCount, &invoice.LastRemindedAt,
		&invoice.FailureCode, &invoice.FailureMessage, &invoice.CreatedAt, &invoice.UpdatedAt)
	return invoice, err
}

func (r *PostgresRepository) GetInvoice(ctx context.Context, merchantID, invoiceID string) (Invoice, error) {
	invoice, err := scanInvoice(r.db.QueryRow(ctx, `
SELECT id, merchant_id, customer_id, COALESCE(subscription_id, ''), COALESCE(external_reference, ''), COALESCE(description, ''), amount, currency, status, billing_reason, period_start, period_end, due_at,
       order_id, payment_id, COALESCE(payment_link_id, ''), COALESCE(virtual_account_id, ''), reminder_count, last_reminded_at, failure_code, failure_message, created_at, updated_at
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
SELECT id, merchant_id, customer_id, COALESCE(subscription_id, ''), COALESCE(external_reference, ''), COALESCE(description, ''), amount, currency, status, billing_reason, period_start, period_end, due_at,
       order_id, payment_id, COALESCE(payment_link_id, ''), COALESCE(virtual_account_id, ''), reminder_count, last_reminded_at, failure_code, failure_message, created_at, updated_at
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

func (r *PostgresRepository) MarkInvoiceReminded(ctx context.Context, merchantID, invoiceID string, remindedAt time.Time) (Invoice, error) {
	invoice, err := scanInvoice(r.db.QueryRow(ctx, `
UPDATE paygate_billing.invoices
SET reminder_count = reminder_count + 1,
    last_reminded_at = $3,
    updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
RETURNING id, merchant_id, customer_id, COALESCE(subscription_id, ''), COALESCE(external_reference, ''), COALESCE(description, ''), amount, currency, status, billing_reason, period_start, period_end, due_at,
          order_id, payment_id, COALESCE(payment_link_id, ''), COALESCE(virtual_account_id, ''), reminder_count, last_reminded_at, failure_code, failure_message, created_at, updated_at
`, merchantID, invoiceID, remindedAt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invoice{}, ErrInvoiceNotFound
		}
		return Invoice{}, err
	}
	return invoice, nil
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
RETURNING id, merchant_id, customer_id, COALESCE(subscription_id, ''), COALESCE(external_reference, ''), COALESCE(description, ''), amount, currency, status, billing_reason, period_start, period_end, due_at,
       order_id, payment_id, COALESCE(payment_link_id, ''), COALESCE(virtual_account_id, ''), reminder_count, last_reminded_at, failure_code, failure_message, created_at, updated_at
`, merchantID, invoiceID, orderID, paymentID).Scan(&invoice.ID, &invoice.MerchantID, &invoice.CustomerID, &subscriptionID, &invoice.ExternalReference, &invoice.Description, &invoice.Amount, &invoice.Currency, &invoice.Status, &invoice.BillingReason, &invoice.PeriodStart, &invoice.PeriodEnd, &invoice.DueAt, &invoice.OrderID, &invoice.PaymentID, &invoice.PaymentLinkID, &invoice.VirtualAccountID, &invoice.ReminderCount, &invoice.LastRemindedAt, &invoice.FailureCode, &invoice.FailureMessage, &invoice.CreatedAt, &invoice.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invoice{}, ErrInvoiceNotFound
		}
		return Invoice{}, err
	}
	invoice.SubscriptionID = subscriptionID
	if subscriptionID != "" {
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
RETURNING id, merchant_id, customer_id, COALESCE(subscription_id, ''), COALESCE(external_reference, ''), COALESCE(description, ''), amount, currency, status, billing_reason, period_start, period_end, due_at,
       order_id, payment_id, COALESCE(payment_link_id, ''), COALESCE(virtual_account_id, ''), reminder_count, last_reminded_at, failure_code, failure_message, created_at, updated_at
`, merchantID, invoiceID, failureCode, failureMessage).Scan(&invoice.ID, &invoice.MerchantID, &invoice.CustomerID, &subscriptionID, &invoice.ExternalReference, &invoice.Description, &invoice.Amount, &invoice.Currency, &invoice.Status, &invoice.BillingReason, &invoice.PeriodStart, &invoice.PeriodEnd, &invoice.DueAt, &invoice.OrderID, &invoice.PaymentID, &invoice.PaymentLinkID, &invoice.VirtualAccountID, &invoice.ReminderCount, &invoice.LastRemindedAt, &invoice.FailureCode, &invoice.FailureMessage, &invoice.CreatedAt, &invoice.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invoice{}, ErrInvoiceNotFound
		}
		return Invoice{}, err
	}
	invoice.SubscriptionID = subscriptionID
	if subscriptionID != "" {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_billing.subscriptions
SET retry_count = $3,
    next_billing_at = $4,
    updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
`, merchantID, subscriptionID, retryCount, nextBillingAt); err != nil {
			return Invoice{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, err
	}
	return invoice, nil
}
