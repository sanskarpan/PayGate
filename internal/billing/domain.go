package billing

import (
	"errors"
	"strings"
	"time"

	"github.com/sanskarpan/PayGate/internal/upiverify"
)

type SubscriptionStatus string
type IntervalUnit string
type InvoiceStatus string
type InvoiceAttemptStatus string
type VirtualAccountStatus string
type CollectionStatus string
type ConnectedAccountStatus string
type SplitDestinationType string
type CollectionMethod string
type UPIMandateStatus string
type UPIMandateEventType string
type PaymentLinkStatus string

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

const (
	CollectionMethodCard       CollectionMethod = "card"
	CollectionMethodUPIMandate CollectionMethod = "upi_mandate"
)

const (
	UPIMandatePendingApproval UPIMandateStatus = "pending_approval"
	UPIMandateActive          UPIMandateStatus = "active"
	UPIMandatePaused          UPIMandateStatus = "paused"
	UPIMandateRevoked         UPIMandateStatus = "revoked"
	UPIMandateExpired         UPIMandateStatus = "expired"
	UPIMandateFailed          UPIMandateStatus = "failed"
)

const (
	MandateEventCreated   UPIMandateEventType = "created"
	MandateEventActivated UPIMandateEventType = "activated"
	MandateEventPaused    UPIMandateEventType = "paused"
	MandateEventResumed   UPIMandateEventType = "resumed"
	MandateEventRevoked   UPIMandateEventType = "revoked"
	MandateEventExpired   UPIMandateEventType = "expired"
	MandateEventChargeOK  UPIMandateEventType = "charge_succeeded"
	MandateEventChargeErr UPIMandateEventType = "charge_failed"
)

var (
	ErrCustomerNotFound         = errors.New("customer not found")
	ErrVirtualAccountNotFound   = errors.New("virtual account not found")
	ErrCollectionNotFound       = errors.New("collection not found")
	ErrConnectedAccountNotFound = errors.New("connected account not found")
	ErrSubscriptionNotFound     = errors.New("subscription not found")
	ErrInvoiceNotFound          = errors.New("invoice not found")
	ErrUPIMandateNotFound       = errors.New("upi mandate not found")
	ErrInvalidSubscription      = errors.New("invalid subscription")
	ErrSubscriptionNotActive    = errors.New("subscription is not active")
	ErrCardTokenNotReusable     = errors.New("subscription payment method token must be reusable")
	ErrCustomerTokenMismatch    = errors.New("customer and payment method token do not match")
	ErrSubscriptionAlreadyDone  = errors.New("subscription is already terminal")
	ErrInvalidUPIMandate        = errors.New("invalid upi mandate")
	ErrUPIMandateNotActive      = errors.New("upi mandate is not active")
	ErrMandateCustomerMismatch  = errors.New("customer and upi mandate do not match")
	ErrInvalidVirtualAccount    = errors.New("invalid virtual account")
	ErrInvalidCollection        = errors.New("invalid inbound collection")
	ErrInvalidConnectedAccount  = errors.New("invalid connected account")
	ErrInvalidSplitInstruction  = errors.New("invalid split instruction")
	ErrVPAVerificationRequired  = errors.New("upi vpa verification is required")
	ErrPaymentLinkNotFound      = errors.New("payment link not found")
	ErrInvalidPaymentLink       = errors.New("invalid payment link")
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
	CollectionMethod     CollectionMethod
	PaymentMethodTokenID string
	UPIMandateID         string
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

type UPIMandate struct {
	ID                           string
	MerchantID                   string
	CustomerID                   string
	Reference                    string
	DisplayName                  string
	VPA                          string
	AmountLimit                  int64
	Currency                     string
	IntervalUnit                 IntervalUnit
	IntervalCount                int
	RetryWindowHours             int
	Status                       UPIMandateStatus
	ApprovalToken                string
	ApprovedAt                   *time.Time
	PausedAt                     *time.Time
	RevokedAt                    *time.Time
	ExpiresAt                    *time.Time
	Metadata                     map[string]any
	LatestVerificationID         string
	LatestVerificationVersion    int
	LatestVerificationStatus     upiverify.Status
	LatestVerificationProvider   string
	LatestVerificationVerifiedAt *time.Time
	LatestVerificationExpiresAt  *time.Time
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

type UPIMandateEvent struct {
	ID         string
	MerchantID string
	MandateID  string
	EventType  UPIMandateEventType
	ActorType  string
	ActorID    string
	Reason     string
	PaymentID  string
	Metadata   map[string]any
	CreatedAt  time.Time
}

type Invoice struct {
	ID                string
	MerchantID        string
	CustomerID        string
	SubscriptionID    string
	ExternalReference string
	Description       string
	Amount            int64
	Currency          string
	Status            InvoiceStatus
	Overdue           bool
	BillingReason     string
	PeriodStart       time.Time
	PeriodEnd         time.Time
	DueAt             time.Time
	OrderID           string
	PaymentID         string
	PaymentLinkID     string
	VirtualAccountID  string
	ReminderCount     int
	LastRemindedAt    *time.Time
	FailureCode       string
	FailureMessage    string
	CreatedAt         time.Time
	UpdatedAt         time.Time
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

const (
	PaymentLinkActive   PaymentLinkStatus = "active"
	PaymentLinkDisabled PaymentLinkStatus = "disabled"
	PaymentLinkExpired  PaymentLinkStatus = "expired"
	PaymentLinkPaid     PaymentLinkStatus = "paid"
)

type PaymentLink struct {
	ID                string
	MerchantID        string
	CustomerID        string
	OrderID           string
	ExternalReference string
	Title             string
	Description       string
	Amount            int64
	Currency          string
	Status            PaymentLinkStatus
	CallbackURL       string
	Notes             map[string]any
	ExpiresAt         time.Time
	LastVisitedAt     *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
	if strings.TrimSpace(s.PlanName) == "" {
		return ErrInvalidSubscription
	}
	switch s.CollectionMethod {
	case "", CollectionMethodCard:
		if strings.TrimSpace(s.PaymentMethodTokenID) == "" {
			return ErrInvalidSubscription
		}
	case CollectionMethodUPIMandate:
		if strings.TrimSpace(s.UPIMandateID) == "" {
			return ErrInvalidSubscription
		}
	default:
		return ErrInvalidSubscription
	}
	if s.Amount <= 0 || s.IntervalCount <= 0 || s.NextBillingAt.IsZero() {
		return ErrInvalidSubscription
	}
	if s.IntervalUnit != IntervalDay && s.IntervalUnit != IntervalWeek && s.IntervalUnit != IntervalMonth {
		return ErrInvalidSubscription
	}
	if strings.TrimSpace(s.Currency) == "" {
		return ErrInvalidSubscription
	}
	return nil
}

func (m UPIMandate) Validate() error {
	if strings.TrimSpace(m.MerchantID) == "" || strings.TrimSpace(m.CustomerID) == "" || strings.TrimSpace(m.Reference) == "" {
		return ErrInvalidUPIMandate
	}
	if strings.TrimSpace(m.DisplayName) == "" || strings.TrimSpace(m.VPA) == "" || strings.TrimSpace(m.Currency) == "" {
		return ErrInvalidUPIMandate
	}
	if m.AmountLimit <= 0 || m.IntervalCount <= 0 {
		return ErrInvalidUPIMandate
	}
	if m.IntervalUnit != IntervalDay && m.IntervalUnit != IntervalWeek && m.IntervalUnit != IntervalMonth {
		return ErrInvalidUPIMandate
	}
	switch m.Status {
	case "", UPIMandatePendingApproval, UPIMandateActive, UPIMandatePaused, UPIMandateRevoked, UPIMandateExpired, UPIMandateFailed:
	default:
		return ErrInvalidUPIMandate
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
	if strings.TrimSpace(c.Currency) == "" {
		return ErrInvalidCollection
	}
	return nil
}

func (p PaymentLink) Validate() error {
	if strings.TrimSpace(p.MerchantID) == "" || strings.TrimSpace(p.OrderID) == "" || p.Amount <= 0 || strings.TrimSpace(p.Currency) == "" {
		return ErrInvalidPaymentLink
	}
	switch p.Status {
	case "", PaymentLinkActive, PaymentLinkDisabled, PaymentLinkExpired, PaymentLinkPaid:
	default:
		return ErrInvalidPaymentLink
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
