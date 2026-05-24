package billing

import (
	"errors"
	"strings"
	"time"
)

type SubscriptionStatus string
type IntervalUnit string
type InvoiceStatus string
type InvoiceAttemptStatus string
type VirtualAccountStatus string
type CollectionStatus string
type ConnectedAccountStatus string
type SplitDestinationType string

const (
	SubscriptionActive   SubscriptionStatus = "active"
	SubscriptionPaused   SubscriptionStatus = "paused"
	SubscriptionCanceled SubscriptionStatus = "canceled"
)

const (
	IntervalDay   IntervalUnit = "day"
	IntervalWeek  IntervalUnit = "week"
	IntervalMonth IntervalUnit = "month"
)

const (
	InvoiceOpen   InvoiceStatus = "open"
	InvoicePaid   InvoiceStatus = "paid"
	InvoiceFailed InvoiceStatus = "failed"
	InvoiceVoid   InvoiceStatus = "void"
)

const (
	InvoiceAttemptStarted    InvoiceAttemptStatus = "started"
	InvoiceAttemptAuthorized InvoiceAttemptStatus = "authorized"
	InvoiceAttemptCaptured   InvoiceAttemptStatus = "captured"
	InvoiceAttemptFailed     InvoiceAttemptStatus = "failed"
)

const (
	VirtualAccountActive   VirtualAccountStatus = "active"
	VirtualAccountInactive VirtualAccountStatus = "inactive"
)

const (
	CollectionMatched        CollectionStatus = "matched"
	CollectionReviewRequired CollectionStatus = "review_required"
)

const (
	ConnectedAccountActive   ConnectedAccountStatus = "active"
	ConnectedAccountInactive ConnectedAccountStatus = "inactive"
)

const (
	SplitDestinationMerchant         SplitDestinationType = "merchant"
	SplitDestinationConnectedAccount SplitDestinationType = "connected_account"
)

var (
	ErrCustomerNotFound         = errors.New("customer not found")
	ErrVirtualAccountNotFound   = errors.New("virtual account not found")
	ErrCollectionNotFound       = errors.New("collection not found")
	ErrConnectedAccountNotFound = errors.New("connected account not found")
	ErrSubscriptionNotFound     = errors.New("subscription not found")
	ErrInvoiceNotFound          = errors.New("invoice not found")
	ErrInvalidSubscription      = errors.New("invalid subscription")
	ErrSubscriptionNotActive    = errors.New("subscription is not active")
	ErrCardTokenNotReusable     = errors.New("subscription payment method token must be reusable")
	ErrCustomerTokenMismatch    = errors.New("customer and payment method token do not match")
	ErrSubscriptionAlreadyDone  = errors.New("subscription is already terminal")
	ErrInvalidVirtualAccount    = errors.New("invalid virtual account")
	ErrInvalidCollection        = errors.New("invalid inbound collection")
	ErrInvalidConnectedAccount  = errors.New("invalid connected account")
	ErrInvalidSplitInstruction  = errors.New("invalid split instruction")
)

type Customer struct {
	ID                    string
	MerchantID            string
	Name                  string
	Email                 string
	Phone                 string
	ExternalReference     string
	DefaultPaymentTokenID string
	Metadata              map[string]any
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type Subscription struct {
	ID                   string
	MerchantID           string
	CustomerID           string
	PlanName             string
	PaymentMethodTokenID string
	Amount               int64
	Currency             string
	IntervalUnit         IntervalUnit
	IntervalCount        int
	Status               SubscriptionStatus
	NextBillingAt        time.Time
	RetryCount           int
	MaxRetryCount        int
	RetryIntervalHours   int
	CancelAtPeriodEnd    bool
	PauseReason          string
	CanceledAt           *time.Time
	Metadata             map[string]any
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Invoice struct {
	ID             string
	MerchantID     string
	CustomerID     string
	SubscriptionID string
	Amount         int64
	Currency       string
	Status         InvoiceStatus
	BillingReason  string
	PeriodStart    time.Time
	PeriodEnd      time.Time
	DueAt          time.Time
	OrderID        string
	PaymentID      string
	FailureCode    string
	FailureMessage string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type InvoiceAttempt struct {
	ID             string
	InvoiceID      string
	MerchantID     string
	SubscriptionID string
	AttemptNumber  int
	Status         InvoiceAttemptStatus
	OrderID        string
	PaymentID      string
	FailureCode    string
	FailureMessage string
	CreatedAt      time.Time
}

type VirtualAccount struct {
	ID            string
	MerchantID    string
	CustomerID    string
	OrderID       string
	Reference     string
	Provider      string
	BankName      string
	AccountNumber string
	IFSC          string
	UPIVPA        string
	Status        VirtualAccountStatus
	Metadata      map[string]any
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type InboundCollection struct {
	ID               string
	MerchantID       string
	VirtualAccountID string
	CustomerID       string
	OrderID          string
	Amount           int64
	Currency         string
	RemitterName     string
	RemitterAccount  string
	RemitterIFSC     string
	RemitterVPA      string
	UTR              string
	Status           CollectionStatus
	ReviewNotes      string
	MatchedAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ConnectedAccount struct {
	ID                string
	MerchantID        string
	LinkedMerchantID  string
	BeneficiaryID     string
	DisplayName       string
	ExternalReference string
	Status            ConnectedAccountStatus
	Metadata          map[string]any
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type SplitInstruction struct {
	DestinationType  SplitDestinationType
	DestinationRef   string
	BeneficiaryLabel string
	Amount           int64
	Currency         string
}

func (c Customer) Validate() error {
	if strings.TrimSpace(c.MerchantID) == "" {
		return errors.New("merchant id is required")
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("customer name is required")
	}
	return nil
}

func (s Subscription) Validate() error {
	if strings.TrimSpace(s.MerchantID) == "" || strings.TrimSpace(s.CustomerID) == "" {
		return ErrInvalidSubscription
	}
	if strings.TrimSpace(s.PlanName) == "" || strings.TrimSpace(s.PaymentMethodTokenID) == "" {
		return ErrInvalidSubscription
	}
	if s.Amount <= 0 || s.IntervalCount <= 0 || s.NextBillingAt.IsZero() {
		return ErrInvalidSubscription
	}
	if s.IntervalUnit != IntervalDay && s.IntervalUnit != IntervalWeek && s.IntervalUnit != IntervalMonth {
		return ErrInvalidSubscription
	}
	if s.Currency == "" {
		s.Currency = "INR"
	}
	return nil
}

func (v VirtualAccount) Validate() error {
	if strings.TrimSpace(v.MerchantID) == "" || strings.TrimSpace(v.Reference) == "" {
		return ErrInvalidVirtualAccount
	}
	if v.Status != "" && v.Status != VirtualAccountActive && v.Status != VirtualAccountInactive {
		return ErrInvalidVirtualAccount
	}
	return nil
}

func (c InboundCollection) Validate() error {
	if strings.TrimSpace(c.MerchantID) == "" || strings.TrimSpace(c.VirtualAccountID) == "" || c.Amount <= 0 {
		return ErrInvalidCollection
	}
	if c.Currency == "" {
		c.Currency = "INR"
	}
	return nil
}

func (c ConnectedAccount) Validate() error {
	if strings.TrimSpace(c.MerchantID) == "" || strings.TrimSpace(c.DisplayName) == "" {
		return ErrInvalidConnectedAccount
	}
	if c.Status != "" && c.Status != ConnectedAccountActive && c.Status != ConnectedAccountInactive {
		return ErrInvalidConnectedAccount
	}
	return nil
}

func ValidateSplitInstructions(amount, fee int64, splits []SplitInstruction) error {
	if len(splits) == 0 {
		return nil
	}
	var total int64
	for _, split := range splits {
		if split.Amount < 0 {
			return ErrInvalidSplitInstruction
		}
		if split.DestinationType != SplitDestinationConnectedAccount {
			return ErrInvalidSplitInstruction
		}
		if strings.TrimSpace(split.DestinationRef) == "" {
			return ErrInvalidSplitInstruction
		}
		total += split.Amount
	}
	if total > amount-fee {
		return ErrInvalidSplitInstruction
	}
	return nil
}

func nextPeriodStart(from time.Time, unit IntervalUnit, count int) time.Time {
	switch unit {
	case IntervalDay:
		return from.Add(time.Duration(count) * 24 * time.Hour)
	case IntervalWeek:
		return from.Add(time.Duration(count*7) * 24 * time.Hour)
	default:
		return from.AddDate(0, count, 0)
	}
}
