package risk

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/outbox"
)

// OutboxAlertFunc returns an AlertFunc that writes a risk.alert outbox event
// for every hold or block action. This allows downstream consumers (e.g. the
// webhook service) to notify merchants of flagged payments in real time.
func OutboxAlertFunc(db *pgxpool.Pool) AlertFunc {
	writer := outbox.NewWriter()
	return func(ctx context.Context, ev RiskEvent) {
		// Begin a standalone transaction for the outbox write.
		tx, err := db.Begin(ctx)
		if err != nil {
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()
		_ = writer.WriteTx(ctx, tx, outbox.Event{
			AggregateType: "risk",
			AggregateID:   ev.ID,
			EventType:     "risk.alert",
			MerchantID:    ev.MerchantID,
			Payload: map[string]any{
				"risk_event_id": ev.ID,
				"payment_id":    ev.PaymentID,
				"score":         ev.Score,
				"action":        ev.Action,
				"rules":         ev.TriggeredRules,
			},
		})
		_ = tx.Commit(ctx)
	}
}
