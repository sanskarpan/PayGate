package settlement

import (
	"errors"
	"time"
)

// SettlementState is the explicit state machine type for settlements.
type SettlementState string

// SettlementEvent drives transitions in the settlement state machine.
type SettlementEvent string

const (
	StateCreated    SettlementState = "created"
	StateProcessing SettlementState = "processing"
	StateProcessed  SettlementState = "processed"
	StateFailed     SettlementState = "failed"
)

const (
	EventProcess  SettlementEvent = "process"  // created → processing
	EventComplete SettlementEvent = "complete" // processing → processed
	EventFail     SettlementEvent = "fail"     // processing → failed
)

var (
	ErrSettlementNotFound  = errors.New("settlement not found")
	ErrInvalidTransition   = errors.New("invalid settlement state transition")
	ErrNoEligiblePayments  = errors.New("no eligible payments found for settlement")
	ErrSettlementOnHold    = errors.New("settlement is already on hold")
	ErrSettlementNotOnHold = errors.New("settlement is not on hold")
)

// Transition returns the next SettlementState for the given event,
// or ErrInvalidTransition if the event is invalid from the current state.
func Transition(from SettlementState, ev SettlementEvent) (SettlementState, error) {
	table := map[SettlementState]map[SettlementEvent]SettlementState{
		StateCreated: {
			EventProcess: StateProcessing,
		},
		StateProcessing: {
			EventComplete: StateProcessed,
			EventFail:     StateFailed,
		},
	}
	m, ok := table[from]
	if !ok {
		return "", ErrInvalidTransition
	}
	next, ok := m[ev]
	if !ok {
		return "", ErrInvalidTransition
	}
	return next, nil
}

// Settlement groups payments for a merchant into a single payout batch.
type Settlement struct {
	ID           string
	MerchantID   string
	Status       SettlementState
	PeriodStart  time.Time
	PeriodEnd    time.Time
	TotalAmount  int64 // sum of captured payment amounts
	TotalFees    int64 // sum of platform fees deducted
	TotalRefunds int64 // sum of refunded amounts
	NetAmount    int64 // TotalAmount - TotalFees - TotalRefunds
	PaymentCount int
	Currency     string
	ProcessedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	OnHold       bool
	HoldReason   string
	HeldAt       *time.Time
	ReleasedAt   *time.Time
}

// SettlementItem is one payment's contribution to a settlement batch.
type SettlementItem struct {
	ID           string
	SettlementID string
	PaymentID    string
	MerchantID   string
	Amount       int64 // gross captured amount
	Fee          int64 // platform fee taken at capture (2% of amount)
	Refunds      int64 // total amount_refunded on payment
	Net          int64 // Amount - Fee - Refunds
	Currency     string
	CreatedAt    time.Time
}

// CalculateFee returns the platform fee for a given payment amount.
// The fee rate is 2% (200 basis points), consistent with the capture ledger entries.
func CalculateFee(amount int64) int64 {
	return amount * 2 / 100
}

// CalculateNet returns the net merchant payout for an item.
func CalculateNet(amount, fee, refunds int64) int64 {
	return amount - fee - refunds
}

// CalculateRefundFeeReversal returns the cumulative portion of the original fee
// that should be reversed for a cumulative refunded amount. It prorates by the
// refunded share of the payment and guarantees full fee reversal on a full refund.
func CalculateRefundFeeReversal(paymentAmount, paymentFee, refundedAmount int64) int64 {
	switch {
	case paymentAmount <= 0, paymentFee <= 0, refundedAmount <= 0:
		return 0
	case refundedAmount >= paymentAmount:
		return paymentFee
	default:
		reversed := (paymentFee * refundedAmount) / paymentAmount
		if reversed > paymentFee {
			return paymentFee
		}
		return reversed
	}
}

// CalculateRefundNetImpact returns the cumulative reduction to merchant payable
// after accounting for the portion of platform fee that must be reversed.
func CalculateRefundNetImpact(paymentAmount, paymentFee, refundedAmount int64) int64 {
	feeReversal := CalculateRefundFeeReversal(paymentAmount, paymentFee, refundedAmount)
	return refundedAmount - feeReversal
}

// EligiblePayment carries the fields needed to settle a single payment.
type EligiblePayment struct {
	PaymentID      string
	Amount         int64
	Fee            int64
	AmountRefunded int64
	Currency       string
}
