package refund

import (
	"testing"
)

func TestTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    RefundState
		event   RefundEvent
		want    RefundState
		wantErr bool
	}{
		// Valid transitions
		{name: "created+initiate→processing", from: StateCreated, event: EventInitiate, want: StateProcessing},
		{name: "processing+success→processed", from: StateProcessing, event: EventSuccess, want: StateProcessed},
		{name: "processing+failure→failed", from: StateProcessing, event: EventFailure, want: StateFailed},

		// Invalid transitions
		{name: "created+success is invalid", from: StateCreated, event: EventSuccess, wantErr: true},
		{name: "created+failure is invalid", from: StateCreated, event: EventFailure, wantErr: true},
		{name: "processed+anything is terminal", from: StateProcessed, event: EventInitiate, wantErr: true},
		{name: "failed+anything is terminal", from: StateFailed, event: EventInitiate, wantErr: true},
		{name: "processing+initiate is invalid", from: StateProcessing, event: EventInitiate, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Transition(tc.from, tc.event)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got state %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestPaymentSnapshotRefundableBalance(t *testing.T) {
	tests := []struct {
		name    string
		snap    PaymentSnapshot
		want    int64
	}{
		{
			name: "no refunds yet",
			snap: PaymentSnapshot{Amount: 10000, AmountRefunded: 0, AmountRefundedPending: 0},
			want: 10000,
		},
		{
			name: "partial confirmed refund",
			snap: PaymentSnapshot{Amount: 10000, AmountRefunded: 3000, AmountRefundedPending: 0},
			want: 7000,
		},
		{
			name: "pending reservation reduces balance",
			snap: PaymentSnapshot{Amount: 10000, AmountRefunded: 2000, AmountRefundedPending: 3000},
			want: 5000,
		},
		{
			name: "fully refunded",
			snap: PaymentSnapshot{Amount: 10000, AmountRefunded: 10000, AmountRefundedPending: 0},
			want: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.snap.RefundableBalance()
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}
