package billing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

func (r *PostgresRepository) CreatePaymentLink(ctx context.Context, link PaymentLink) (PaymentLink, error) {
	link.ID = idgen.New("plink")
	notes, err := json.Marshal(link.Notes)
	if err != nil {
		return PaymentLink{}, err
	}
	err = r.db.QueryRow(ctx, `
INSERT INTO paygate_billing.payment_links
    (id, merchant_id, customer_id, order_id, external_reference, title, description, amount, currency, status, callback_url, notes, expires_at)
VALUES ($1,$2,NULLIF($3, ''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING created_at, updated_at
`, link.ID, link.MerchantID, link.CustomerID, link.OrderID, link.ExternalReference, link.Title, link.Description, link.Amount, link.Currency, link.Status, link.CallbackURL, string(notes), link.ExpiresAt).Scan(&link.CreatedAt, &link.UpdatedAt)
	return link, err
}

func scanPaymentLink(row pgx.Row) (PaymentLink, error) {
	var link PaymentLink
	var rawNotes []byte
	err := row.Scan(&link.ID, &link.MerchantID, &link.CustomerID, &link.OrderID, &link.ExternalReference, &link.Title, &link.Description, &link.Amount, &link.Currency, &link.Status, &link.CallbackURL, &rawNotes, &link.ExpiresAt, &link.LastVisitedAt, &link.CreatedAt, &link.UpdatedAt)
	if err != nil {
		return PaymentLink{}, err
	}
	if len(rawNotes) > 0 {
		_ = json.Unmarshal(rawNotes, &link.Notes)
	}
	return link, nil
}

func (r *PostgresRepository) GetPaymentLink(ctx context.Context, merchantID, linkID string) (PaymentLink, error) {
	link, err := scanPaymentLink(r.db.QueryRow(ctx, `
SELECT id, merchant_id, COALESCE(customer_id, ''), order_id, external_reference, title, description, amount, currency, status, callback_url, notes, expires_at, last_visited_at, created_at, updated_at
FROM paygate_billing.payment_links
WHERE merchant_id = $1 AND id = $2
`, merchantID, linkID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentLink{}, ErrPaymentLinkNotFound
		}
		return PaymentLink{}, err
	}
	return link, nil
}

func (r *PostgresRepository) ListPaymentLinks(ctx context.Context, merchantID string, limit int) ([]PaymentLink, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, COALESCE(customer_id, ''), order_id, external_reference, title, description, amount, currency, status, callback_url, notes, expires_at, last_visited_at, created_at, updated_at
FROM paygate_billing.payment_links
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PaymentLink
	for rows.Next() {
		link, err := scanPaymentLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdatePaymentLinkStatus(ctx context.Context, merchantID, linkID string, status PaymentLinkStatus) (PaymentLink, error) {
	link, err := scanPaymentLink(r.db.QueryRow(ctx, `
UPDATE paygate_billing.payment_links
SET status = $3, updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
RETURNING id, merchant_id, COALESCE(customer_id, ''), order_id, external_reference, title, description, amount, currency, status, callback_url, notes, expires_at, last_visited_at, created_at, updated_at
`, merchantID, linkID, status))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentLink{}, ErrPaymentLinkNotFound
		}
		return PaymentLink{}, err
	}
	return link, nil
}

func (r *PostgresRepository) MarkPaymentLinkVisited(ctx context.Context, merchantID, linkID string, visitedAt time.Time) (PaymentLink, error) {
	link, err := scanPaymentLink(r.db.QueryRow(ctx, `
UPDATE paygate_billing.payment_links
SET last_visited_at = $3, updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
RETURNING id, merchant_id, COALESCE(customer_id, ''), order_id, external_reference, title, description, amount, currency, status, callback_url, notes, expires_at, last_visited_at, created_at, updated_at
`, merchantID, linkID, visitedAt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentLink{}, ErrPaymentLinkNotFound
		}
		return PaymentLink{}, err
	}
	return link, nil
}
