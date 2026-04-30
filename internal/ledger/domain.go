package ledger

import "errors"

var (
	ErrUnbalancedEntries = errors.New("ledger entries are not balanced")
	ErrInvalidEntrySide  = errors.New("each ledger entry must have exactly one non-zero side")
)

type Entry struct {
	AccountCode  string
	DebitAmount  int64
	CreditAmount int64
	Currency     string
	Description  string
}

func ValidateEntries(entries []Entry) error {
	var debit, credit int64
	for _, e := range entries {
		debit += e.DebitAmount
		credit += e.CreditAmount
		if (e.DebitAmount > 0 && e.CreditAmount > 0) || (e.DebitAmount == 0 && e.CreditAmount == 0) {
			return ErrInvalidEntrySide
		}
	}
	if debit != credit {
		return ErrUnbalancedEntries
	}
	return nil
}
