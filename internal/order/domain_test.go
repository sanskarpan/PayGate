package order

import "testing"

func TestTransition(t *testing.T) {
	cases := []struct {
		name    string
		from    State
		event   Event
		to      State
		wantErr bool
	}{
		{"created to attempted", StateCreated, EventAttempted, StateAttempted, false},
		{"created to expired", StateCreated, EventExpired, StateExpired, false},
		{"attempted to paid", StateAttempted, EventPaid, StatePaid, false},
		{"attempted to failed", StateAttempted, EventFailed, StateFailed, false},
		{"invalid from paid", StatePaid, EventFailed, "", true},
		{"invalid event from created", StateCreated, EventPaid, "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, err := Transition(tc.from, tc.event)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if next != tc.to {
				t.Fatalf("expected %s, got %s", tc.to, next)
			}
		})
	}
}
