package risk

import "testing"

func TestEvaluate(t *testing.T) {
	base := EvalInput{
		MerchantID:     "merch_1",
		PaymentID:      "pay_1",
		Amount:         2000,  // 20 INR
		Currency:       "INR",
		IPAddress:      "1.2.3.4",
		MerchantAvgTxn: 1000, // >= ThresholdAmountSpikeMinAvg
	}

	tests := []struct {
		name              string
		in                EvalInput
		merchantHourly    int
		ipHourly          int
		wantAction        RiskAction
		wantRules         []string
	}{
		{
			name:           "clean transaction",
			in:             base,
			merchantHourly: 10,
			ipHourly:       2,
			wantAction:     RiskActionAllow,
			wantRules:      nil,
		},
		{
			name:           "merchant velocity breach",
			in:             base,
			merchantHourly: ThresholdMerchantTxnPerHour,
			ipHourly:       2,
			wantAction:     RiskActionHold,
			wantRules:      []string{"merchant_velocity_1h"},
		},
		{
			name:           "ip velocity breach",
			in:             base,
			merchantHourly: 5,
			ipHourly:       ThresholdIPTxnPerHour,
			wantAction:     RiskActionHold,
			wantRules:      []string{"ip_velocity_1h"},
		},
		{
			name: "amount spike — 3x average",
			in: EvalInput{
				MerchantID:     "merch_1",
				PaymentID:      "pay_2",
				Amount:         1000*ThresholdAmountSpikeFactor + 1, // > 3x avg of 1000
				Currency:       "INR",
				IPAddress:      "1.2.3.4",
				MerchantAvgTxn: 1000, // >= ThresholdAmountSpikeMinAvg
			},
			merchantHourly: 5,
			ipHourly:       2,
			wantAction:     RiskActionHold,
			wantRules:      []string{"amount_spike_3x"},
		},
		{
			name: "amount spike skipped when avg below minimum",
			in: EvalInput{
				MerchantID:     "merch_1",
				PaymentID:      "pay_3",
				Amount:         50000,
				Currency:       "INR",
				IPAddress:      "1.2.3.4",
				MerchantAvgTxn: 100, // below ThresholdAmountSpikeMinAvg
			},
			merchantHourly: 5,
			ipHourly:       2,
			wantAction:     RiskActionAllow,
			wantRules:      nil,
		},
		{
			name: "block on combined high score",
			in: EvalInput{
				MerchantID:     "merch_1",
				PaymentID:      "pay_4",
				Amount:         1000*ThresholdAmountSpikeFactor + 1,
				Currency:       "INR",
				IPAddress:      "1.2.3.4",
				MerchantAvgTxn: 1000,
			},
			merchantHourly: ThresholdMerchantTxnPerHour,
			ipHourly:       ThresholdIPTxnPerHour,
			// ScoreVelocityMerchant(50) + ScoreVelocityIP(50) = 100 → block (>= 90)
			wantAction: RiskActionBlock,
			wantRules:  []string{"merchant_velocity_1h", "ip_velocity_1h", "amount_spike_3x"},
		},
		{
			name: "ip velocity skipped when no IP",
			in: EvalInput{
				MerchantID:     "merch_1",
				PaymentID:      "pay_5",
				Amount:         1000,
				Currency:       "INR",
				IPAddress:      "", // no IP
				MerchantAvgTxn: 500,
			},
			merchantHourly: 5,
			ipHourly:       ThresholdIPTxnPerHour + 10,
			wantAction:     RiskActionAllow,
			wantRules:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Evaluate(tc.in, tc.merchantHourly, tc.ipHourly)
			if res.Action != tc.wantAction {
				t.Errorf("action: got %q, want %q (score=%d)", res.Action, tc.wantAction, res.Score)
			}
			if len(res.TriggeredRules) != len(tc.wantRules) {
				t.Errorf("rules: got %v, want %v", res.TriggeredRules, tc.wantRules)
				return
			}
			for i, r := range res.TriggeredRules {
				if r != tc.wantRules[i] {
					t.Errorf("rules[%d]: got %q, want %q", i, r, tc.wantRules[i])
				}
			}
		})
	}
}
