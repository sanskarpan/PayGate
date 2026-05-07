package payout

import (
	"testing"
)

func TestTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    PayoutState
		event   PayoutEvent
		want    PayoutState
		wantErr bool
	}{
		// Valid transitions
		{name: "pending+initiate→processing", from: StatePending, event: EventInitiate, want: StateProcessing},
		{name: "processing+complete→completed", from: StateProcessing, event: EventComplete, want: StateCompleted},
		{name: "processing+fail→failed", from: StateProcessing, event: EventFail, want: StateFailed},

		// Invalid transitions
		{name: "pending+complete is invalid", from: StatePending, event: EventComplete, wantErr: true},
		{name: "pending+fail is invalid", from: StatePending, event: EventFail, wantErr: true},
		{name: "completed+anything is terminal", from: StateCompleted, event: EventInitiate, wantErr: true},
		{name: "failed+anything is terminal", from: StateFailed, event: EventInitiate, wantErr: true},
		{name: "processing+initiate is invalid", from: StateProcessing, event: EventInitiate, wantErr: true},
		{name: "completed+complete is invalid", from: StateCompleted, event: EventComplete, wantErr: true},
		{name: "completed+fail is invalid", from: StateCompleted, event: EventFail, wantErr: true},
		{name: "failed+complete is invalid", from: StateFailed, event: EventComplete, wantErr: true},
		{name: "failed+fail is invalid", from: StateFailed, event: EventFail, wantErr: true},
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
