package scrubber

import (
	"strings"
	"testing"
)

func TestScrub(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantHas []string // substrings that must appear in output
		wantNot []string // substrings that must NOT appear in output
	}{
		{
			name:    "empty string",
			input:   "",
			wantHas: []string{""},
		},
		{
			name:    "clean string",
			input:   `{"amount":5000,"currency":"INR"}`,
			wantHas: []string{"5000", "INR"},
			wantNot: []string{"SCRUBBED"},
		},
		{
			name:    "password field",
			input:   `{"email":"user@example.com","password":"hunter2"}`,
			wantHas: []string{"email", "SCRUBBED"},
			wantNot: []string{"hunter2"},
		},
		{
			name:    "secret field",
			input:   `{"key_id":"rzp_test_abc","key_secret":"super_secret_123"}`,
			wantHas: []string{"key_id", "rzp_test_abc", "SCRUBBED"},
			wantNot: []string{"super_secret_123"},
		},
		{
			name:    "card number in json",
			input:   `{"card_number":"4111111111111111","amount":1000}`,
			wantHas: []string{"SCRUBBED", "amount"},
			wantNot: []string{"4111111111111111"},
		},
		{
			name:    "raw card number sequence",
			input:   "payment made with card 4111111111111111 today",
			wantHas: []string{"payment", "CARD_SCRUBBED"},
			wantNot: []string{"4111111111111111"},
		},
		{
			name:    "cvv field",
			input:   `{"cvv":"123","amount":500}`,
			wantHas: []string{"amount", "SCRUBBED"},
			wantNot: []string{`"123"`},
		},
		{
			name:    "token field",
			input:   `{"token":"tok_test_abc123xyz","merchant_id":"merch_1"}`,
			wantHas: []string{"merchant_id", "SCRUBBED"},
			wantNot: []string{"tok_test_abc123xyz"},
		},
		{
			name:    "short number not scrubbed (not a card)",
			input:   "order_id: 12345",
			wantHas: []string{"12345"},
			wantNot: []string{"CARD_SCRUBBED"},
		},
		{
			name:    "case insensitive key matching",
			input:   `{"Password":"secret123"}`,
			wantHas: []string{"SCRUBBED"},
			wantNot: []string{"secret123"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Scrub(tc.input)
			for _, want := range tc.wantHas {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in output %q", want, got)
				}
			}
			for _, notWant := range tc.wantNot {
				if strings.Contains(got, notWant) {
					t.Errorf("expected %q NOT in output %q", notWant, got)
				}
			}
		})
	}
}
