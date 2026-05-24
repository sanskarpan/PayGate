package protect

import "testing"

func TestSealAndOpenString(t *testing.T) {
	p := &Protector{
		activeVersion: "v1",
		keys:          map[string][]byte{"v1": []byte("12345678901234567890123456789012")},
		enabled:       true,
	}
	plaintext := "secret-value"
	sealed, err := p.SealString(plaintext)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if sealed == plaintext {
		t.Fatal("expected ciphertext output")
	}
	out, err := p.OpenString(sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if out != plaintext {
		t.Fatalf("expected %q, got %q", plaintext, out)
	}
}

func TestOpenStringAllowsLegacyPlaintext(t *testing.T) {
	p := &Protector{}
	out, err := p.OpenString("legacy")
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if out != "legacy" {
		t.Fatalf("unexpected output %q", out)
	}
}
