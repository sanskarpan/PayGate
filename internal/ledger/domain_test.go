package ledger

import "testing"

func TestValidateEntriesBalanced(t *testing.T) {
	entries := []Entry{{AccountCode: "A", DebitAmount: 100}, {AccountCode: "B", CreditAmount: 100}}
	if err := ValidateEntries(entries); err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
}

func TestValidateEntriesUnbalanced(t *testing.T) {
	entries := []Entry{{AccountCode: "A", DebitAmount: 100}, {AccountCode: "B", CreditAmount: 80}}
	if err := ValidateEntries(entries); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateEntriesSingleSideConstraint(t *testing.T) {
	entries := []Entry{{AccountCode: "A", DebitAmount: 100, CreditAmount: 1}}
	if err := ValidateEntries(entries); err == nil {
		t.Fatal("expected error")
	}
}
