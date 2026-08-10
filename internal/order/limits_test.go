package order

import (
	"errors"
	"strings"
	"testing"
)

func baseOrder() Order {
	return Order{MerchantID: "merch_1", Amount: 1000, Currency: "INR"}
}

// docs/API-CONTRACTS.md publishes these limits; they were documented but never
// enforced, so a 100,000-character receipt was accepted and stored in full.
func TestValidateForCreateEnforcesReceiptLength(t *testing.T) {
	o := baseOrder()
	o.Receipt = strings.Repeat("x", MaxReceiptLength)
	if err := o.ValidateForCreate(); err != nil {
		t.Fatalf("a receipt at the limit must be accepted, got %v", err)
	}

	o.Receipt = strings.Repeat("x", MaxReceiptLength+1)
	if err := o.ValidateForCreate(); !errors.Is(err, ErrReceiptTooLong) {
		t.Fatalf("expected ErrReceiptTooLong, got %v", err)
	}
}

func TestValidateForCreateEnforcesNoteCount(t *testing.T) {
	o := baseOrder()
	o.Notes = map[string]any{}
	for i := 0; i < MaxNoteKeys; i++ {
		o.Notes[string(rune('a'+i))] = "v"
	}
	if err := o.ValidateForCreate(); err != nil {
		t.Fatalf("notes at the key limit must be accepted, got %v", err)
	}

	o.Notes["overflow"] = "v"
	if err := o.ValidateForCreate(); !errors.Is(err, ErrTooManyNotes) {
		t.Fatalf("expected ErrTooManyNotes, got %v", err)
	}
}

func TestValidateForCreateEnforcesNoteLength(t *testing.T) {
	o := baseOrder()
	o.Notes = map[string]any{"k": strings.Repeat("z", MaxNoteLength)}
	if err := o.ValidateForCreate(); err != nil {
		t.Fatalf("a note value at the limit must be accepted, got %v", err)
	}

	o.Notes = map[string]any{"k": strings.Repeat("z", MaxNoteLength+1)}
	if err := o.ValidateForCreate(); !errors.Is(err, ErrNoteTooLong) {
		t.Fatalf("expected ErrNoteTooLong for a long value, got %v", err)
	}

	o.Notes = map[string]any{strings.Repeat("k", MaxNoteLength+1): "v"}
	if err := o.ValidateForCreate(); !errors.Is(err, ErrNoteTooLong) {
		t.Fatalf("expected ErrNoteTooLong for a long key, got %v", err)
	}
}

// Length is counted in runes, so multi-byte text is not penalised for its
// encoding: 40 emoji are 40 characters, not 160 bytes' worth.
func TestValidateForCreateCountsRunesNotBytes(t *testing.T) {
	o := baseOrder()
	o.Receipt = strings.Repeat("é", MaxReceiptLength)
	if err := o.ValidateForCreate(); err != nil {
		t.Fatalf("a multi-byte receipt at the rune limit must be accepted, got %v", err)
	}
}

// Non-string note values carry no length to check and must not be rejected.
func TestValidateForCreateAllowsNonStringNoteValues(t *testing.T) {
	o := baseOrder()
	o.Notes = map[string]any{"count": 42, "ok": true, "nested": map[string]any{"a": "b"}}
	if err := o.ValidateForCreate(); err != nil {
		t.Fatalf("non-string note values must be accepted, got %v", err)
	}
}
