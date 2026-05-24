package tokenization

import "testing"

func TestValidLuhn(t *testing.T) {
	if !validLuhn("4111111111111111") {
		t.Fatal("expected visa test card to pass luhn")
	}
	if validLuhn("4111111111111112") {
		t.Fatal("expected invalid pan to fail luhn")
	}
}

func TestInferBrand(t *testing.T) {
	cases := map[string]string{
		"4111111111111111": "visa",
		"5555555555554444": "mastercard",
		"378282246310005":  "amex",
		"6011111111111117": "discover",
	}
	for pan, want := range cases {
		if got := inferBrand(pan); got != want {
			t.Fatalf("inferBrand(%s) = %s, want %s", pan, got, want)
		}
	}
}
