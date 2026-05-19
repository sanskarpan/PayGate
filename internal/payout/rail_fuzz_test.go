package payout

import "testing"

func FuzzVerifyRailPayload(f *testing.F) {
	secret := "paygate-dev-payout-rail-secret"
	timestamp := "1716115200"
	body := []byte(`{"event_id":"evt_1","payout_id":"pout_1","status":"completed"}`)
	signature := SignRailPayload(secret, timestamp, body)

	f.Add(secret, timestamp, body, signature)
	f.Add(secret, "", []byte(""), "")
	f.Add("", timestamp, []byte("{}"), signature)

	f.Fuzz(func(t *testing.T, secret, timestamp string, body []byte, signature string) {
		ok := VerifyRailPayload(secret, timestamp, body, signature)
		if ok {
			expected := SignRailPayload(secret, timestamp, body)
			if signature != expected {
				t.Fatalf("verify accepted mismatched signature")
			}
		}
	})
}
