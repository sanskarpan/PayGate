package settlement

import "testing"

func TestSettlementTransition(t *testing.T) {
	tests := []struct {
		from    SettlementState
		event   SettlementEvent
		want    SettlementState
		wantErr bool
	}{
		{StateCreated, EventProcess, StateProcessing, false},
		{StateProcessing, EventComplete, StateProcessed, false},
		{StateProcessing, EventFail, StateFailed, false},
		// Invalid transitions
		{StateCreated, EventComplete, "", true},
		{StateCreated, EventFail, "", true},
		{StateProcessed, EventProcess, "", true},
		{StateProcessed, EventComplete, "", true},
		{StateFailed, EventProcess, "", true},
		{StateFailed, EventComplete, "", true},
	}

	for _, tc := range tests {
		got, err := Transition(tc.from, tc.event)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Transition(%s, %s): expected error, got %s", tc.from, tc.event, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Transition(%s, %s): unexpected error: %v", tc.from, tc.event, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Transition(%s, %s) = %s, want %s", tc.from, tc.event, got, tc.want)
		}
	}
}

func TestCalculateFee(t *testing.T) {
	tests := []struct {
		amount int64
		want   int64
	}{
		{10000, 200},  // 2% of ₹100.00 = ₹2.00
		{5000, 100},   // 2% of ₹50.00 = ₹1.00
		{99, 1},       // 2% of ₹0.99 = ₹0.01 (integer division)
		{100, 2},
		{0, 0},
	}
	for _, tc := range tests {
		got := CalculateFee(tc.amount)
		if got != tc.want {
			t.Errorf("CalculateFee(%d) = %d, want %d", tc.amount, got, tc.want)
		}
	}
}

func TestCalculateNet(t *testing.T) {
	tests := []struct {
		amount  int64
		fee     int64
		refunds int64
		want    int64
	}{
		{10000, 200, 0, 9800},
		{10000, 200, 5000, 4800},
		{10000, 200, 10000, -200}, // full refund — net is negative
		{5000, 100, 2500, 2400},
	}
	for _, tc := range tests {
		got := CalculateNet(tc.amount, tc.fee, tc.refunds)
		if got != tc.want {
			t.Errorf("CalculateNet(%d, %d, %d) = %d, want %d", tc.amount, tc.fee, tc.refunds, got, tc.want)
		}
	}
}
