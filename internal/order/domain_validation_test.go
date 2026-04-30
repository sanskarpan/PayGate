package order

import "testing"

func TestOrderValidateForCreate(t *testing.T) {
	cases := []struct {
		name    string
		order   Order
		wantErr bool
	}{
		{"valid", Order{MerchantID: "m1", Amount: 100, Currency: "INR"}, false},
		{"invalid amount", Order{MerchantID: "m1", Amount: 0, Currency: "INR"}, true},
		{"invalid currency", Order{MerchantID: "m1", Amount: 100, Currency: "EUR"}, true},
		{"missing merchant", Order{Amount: 100, Currency: "INR"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.order.ValidateForCreate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
