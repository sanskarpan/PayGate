package order

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/outbox"
)

type cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type PostgresRepository struct {
	db     *pgxpool.Pool
	outbox *outbox.Writer
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db, outbox: outbox.NewWriter()}
}

func (r *PostgresRepository) Create(ctx context.Context, order Order) (Order, error) {
	notes := order.Notes
	if notes == nil {
		notes = map[string]any{}
	}
	notesJSON, err := json.Marshal(notes)
	if err != nil {
		return Order{}, fmt.Errorf("marshal notes: %w", err)
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin order create tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := `
INSERT INTO paygate_orders.orders
(id, merchant_id, idempotency_key, amount, amount_paid, amount_due, currency, receipt, status, partial_payment, notes, expires_at)
VALUES
($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING created_at, updated_at`

	if err := tx.QueryRow(ctx, q,
		order.ID,
		order.MerchantID,
		order.IdempotencyKey,
		order.Amount,
		order.AmountPaid,
		order.AmountDue,
		order.Currency,
		order.Receipt,
		order.Status,
		order.PartialPayment,
		notesJSON,
		order.ExpiresAt,
	).Scan(&order.CreatedAt, &order.UpdatedAt); err != nil {
		var pgErr *pgconn.PgError
		if order.IdempotencyKey != "" && errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return r.getByIdempotencyKey(ctx, order.MerchantID, order.IdempotencyKey)
		}
		return Order{}, fmt.Errorf("insert order: %w", err)
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "order",
		AggregateID:   order.ID,
		EventType:     "order.created",
		MerchantID:    order.MerchantID,
		Payload: map[string]any{
			"order_id":    order.ID,
			"merchant_id": order.MerchantID,
			"amount":      order.Amount,
			"currency":    order.Currency,
			"status":      order.Status,
		},
	}); err != nil {
		return Order{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit order create tx: %w", err)
	}

	order.Notes = notes
	return order, nil
}

func (r *PostgresRepository) getByIdempotencyKey(ctx context.Context, merchantID, idempotencyKey string) (Order, error) {
	var orderID string
	if err := r.db.QueryRow(ctx, `
SELECT id
FROM paygate_orders.orders
WHERE merchant_id = $1 AND idempotency_key = $2
`, merchantID, idempotencyKey).Scan(&orderID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Order{}, ErrOrderNotFound
		}
		return Order{}, fmt.Errorf("get order by idempotency key: %w", err)
	}
	return r.GetByID(ctx, merchantID, orderID)
}

func (r *PostgresRepository) GetByID(ctx context.Context, merchantID, orderID string) (Order, error) {
	q := `
SELECT id, merchant_id, amount, amount_paid, amount_due, currency, receipt, status, partial_payment, notes, expires_at, created_at, updated_at
FROM paygate_orders.orders
WHERE merchant_id = $1 AND id = $2`

	var o Order
	var notesRaw []byte
	if err := r.db.QueryRow(ctx, q, merchantID, orderID).Scan(
		&o.ID,
		&o.MerchantID,
		&o.Amount,
		&o.AmountPaid,
		&o.AmountDue,
		&o.Currency,
		&o.Receipt,
		&o.Status,
		&o.PartialPayment,
		&notesRaw,
		&o.ExpiresAt,
		&o.CreatedAt,
		&o.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Order{}, ErrOrderNotFound
		}
		return Order{}, fmt.Errorf("get order by id: %w", err)
	}

	if len(notesRaw) > 0 {
		if err := json.Unmarshal(notesRaw, &o.Notes); err != nil {
			return Order{}, fmt.Errorf("unmarshal notes: %w", err)
		}
	}
	if o.Notes == nil {
		o.Notes = map[string]any{}
	}
	return o, nil
}

func (r *PostgresRepository) List(ctx context.Context, f ListFilter) (ListResult, error) {
	if f.Count <= 0 || f.Count > 100 {
		f.Count = 10
	}

	args := []any{f.MerchantID}
	query := `
SELECT id, merchant_id, amount, amount_paid, amount_due, currency, receipt, status, partial_payment, notes, expires_at, created_at, updated_at
FROM paygate_orders.orders
WHERE merchant_id = $1`

	argPos := 2
	if f.From > 0 {
		query += fmt.Sprintf(" AND created_at >= to_timestamp($%d)", argPos)
		args = append(args, f.From)
		argPos++
	}
	if f.To > 0 {
		query += fmt.Sprintf(" AND created_at <= to_timestamp($%d)", argPos)
		args = append(args, f.To)
		argPos++
	}

	if f.Cursor != "" {
		c, err := decodeCursor(f.Cursor)
		if err != nil {
			return ListResult{}, fmt.Errorf("decode cursor: %w", err)
		}
		query += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", argPos, argPos+1)
		args = append(args, c.CreatedAt, c.ID)
		argPos += 2
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", argPos)
	args = append(args, f.Count+1)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list orders query: %w", err)
	}
	defer rows.Close()

	items := make([]Order, 0, f.Count+1)
	for rows.Next() {
		var o Order
		var notesRaw []byte
		if err := rows.Scan(
			&o.ID,
			&o.MerchantID,
			&o.Amount,
			&o.AmountPaid,
			&o.AmountDue,
			&o.Currency,
			&o.Receipt,
			&o.Status,
			&o.PartialPayment,
			&notesRaw,
			&o.ExpiresAt,
			&o.CreatedAt,
			&o.UpdatedAt,
		); err != nil {
			return ListResult{}, fmt.Errorf("scan listed order: %w", err)
		}
		if len(notesRaw) > 0 {
			if err := json.Unmarshal(notesRaw, &o.Notes); err != nil {
				return ListResult{}, fmt.Errorf("unmarshal listed notes: %w", err)
			}
		}
		if o.Notes == nil {
			o.Notes = map[string]any{}
		}
		items = append(items, o)
	}

	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate listed orders: %w", err)
	}

	result := ListResult{HasMore: len(items) > f.Count}
	if result.HasMore {
		items = items[:f.Count]
	}
	result.Items = items
	if len(items) > 0 {
		last := items[len(items)-1]
		result.NextCursor = encodeCursor(cursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}

	return result, nil
}

func (r *PostgresRepository) ExpireDueOrders(ctx context.Context) (int64, error) {
	// Use a CTE with SKIP LOCKED and a batch limit to be safe under concurrent sweepers.
	cmd, err := r.db.Exec(ctx, `
WITH to_expire AS (
  SELECT id FROM paygate_orders.orders
  WHERE status = 'created' AND expires_at <= NOW()
  LIMIT 500
  FOR UPDATE SKIP LOCKED
)
UPDATE paygate_orders.orders
SET status = 'expired', updated_at = NOW()
WHERE id IN (SELECT id FROM to_expire)`)
	if err != nil {
		return 0, fmt.Errorf("expire orders: %w", err)
	}
	return cmd.RowsAffected(), nil
}

func (r *PostgresRepository) MarkOrderPaid(ctx context.Context, merchantID, orderID string) error {
	cmd, err := r.db.Exec(ctx, `
UPDATE paygate_orders.orders
SET status = 'paid', amount_paid = amount, amount_due = 0, updated_at = NOW()
WHERE id = $1 AND merchant_id = $2`, orderID, merchantID)
	if err != nil {
		return fmt.Errorf("mark order paid: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrOrderNotFound
	}
	return nil
}

func encodeCursor(c cursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(v string) (cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return cursor{}, err
	}
	var c cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return cursor{}, err
	}
	return c, nil
}
