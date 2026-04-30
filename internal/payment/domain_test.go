package payment

import "testing"

func TestTransition(t *testing.T) {
	cases := []struct {
		from PaymentState
		ev   PaymentEvent
		ok   bool
	}{
		{StateCreated, EventAuthSuccess, true},
		{StateCreated, EventAuthFailed, true},
		{StateAuthorized, EventCapture, true},
		{StateAuthorized, EventCaptureExpiry, true},
		{StateCaptured, EventCapture, false},
	}
	for _, tc := range cases {
		_, err := Transition(tc.from, tc.ev)
		if tc.ok && err != nil {
			t.Fatalf("expected nil error for %s/%s", tc.from, tc.ev)
		}
		if !tc.ok && err == nil {
			t.Fatalf("expected error for %s/%s", tc.from, tc.ev)
		}
	}
}
