package protect

import "testing"

func TestSealAndOpenString(t *testing.T) {
	p := &Protector{
		activeVersion: "v1",
		keys:          map[string][]byte{"v1": []byte("12345678901234567890123456789012")},
		enabled:       true,
		provider:      "env",
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

func TestSealAndOpenStringForDomain(t *testing.T) {
	p := &Protector{
		activeVersion: "v1",
		keys:          map[string][]byte{"v1": []byte("12345678901234567890123456789012")},
		enabled:       true,
		provider:      "env",
	}
	sealed, err := p.SealStringForDomain(DomainWebhookSecret, "secret-value")
	if err != nil {
		t.Fatalf("seal with domain: %v", err)
	}
	if _, err := p.OpenStringForDomain(DomainPayoutBeneficiaryIdentity, sealed); err == nil {
		t.Fatal("expected domain mismatch error")
	}
	out, err := p.OpenStringForDomain(DomainWebhookSecret, sealed)
	if err != nil {
		t.Fatalf("open with domain: %v", err)
	}
	if out != "secret-value" {
		t.Fatalf("unexpected output %q", out)
	}
}
