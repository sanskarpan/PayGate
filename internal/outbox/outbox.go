package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

type Writer struct{}

type Event struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	MerchantID    string
	Payload       map[string]any
	CreatedAt     time.Time
}

func NewWriter() *Writer {
	return &Writer{}
}

func (w *Writer) WriteTx(ctx context.Context, tx pgx.Tx, event Event) error {
	if event.ID == "" {
		event.ID = idgen.New("evt")
	}
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}

	_, err = tx.Exec(ctx, `
INSERT INTO public.outbox (id, aggregate_type, aggregate_id, event_type, payload, merchant_id)
VALUES ($1, $2, $3, $4, $5, $6)
`, event.ID, event.AggregateType, event.AggregateID, event.EventType, payload, event.MerchantID)
	if err != nil {
		return fmt.Errorf("insert outbox row: %w", err)
	}
	return nil
}
