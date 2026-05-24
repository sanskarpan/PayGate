package risk

import (
	"context"
	"errors"
	"time"
)

// RiskAction is the outcome of a risk evaluation.
type RiskAction string
type ReviewStatus string

const (
	RiskActionAllow RiskAction = "allow"
	RiskActionHold  RiskAction = "hold"
	RiskActionBlock RiskAction = "block"
)

// VelocityWindow is a rolling time window for velocity checks.
type VelocityWindow string

const (
	VelocityWindow1H  VelocityWindow = "1h"
	VelocityWindow24H VelocityWindow = "24h"
)

const (
	ReviewStatusNotRequired ReviewStatus = "not_required"
	ReviewStatusPending     ReviewStatus = "pending"
	ReviewStatusApproved    ReviewStatus = "approved"
	ReviewStatusBlocked     ReviewStatus = "blocked"
)

var (
	ErrRiskEventNotFound = errors.New("risk event not found")
)

// AlertFunc is called when a risk evaluation results in a hold or block action.
// It receives the risk event so callers can write outbox events or send notifications.
// A nil AlertFunc means alerts are disabled.
type AlertFunc func(ctx context.Context, ev RiskEvent)

// RiskEvent records the risk evaluation for a single payment.
type RiskEvent struct {
	ID                    string
	MerchantID            string
	PaymentID             string
	Score                 int
	Action                RiskAction
	TriggeredRules        []string
	DeviceFingerprintHash string
	BrowserLanguage       string
	UserAgent             string
	CardBIN               string
	CardNetwork           string
	IssuerCountry         string
	CardCountry           string
	FundingType           string
	ReviewStatus          ReviewStatus
	AssignedTo            string
	AssignedAt            *time.Time
	ReviewNotes           string
	ManualDecision        string
	Resolved              bool
	ResolvedBy            string
	ResolvedAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// VelocityCounter is a rolling count/amount per dimension and window.
type VelocityCounter struct {
	ID        string
	Dimension string
	DimValue  string
	Window    VelocityWindow
	Count     int
	Amount    int64
	WindowEnd time.Time
	UpdatedAt time.Time
}

// EvalInput carries the parameters for a risk evaluation.
type EvalInput struct {
	MerchantID        string
	PaymentID         string
	Amount            int64
	Currency          string
	IPAddress         string
	CardToken         string
	MerchantAvgTxn    int64
	DeviceFingerprint string
	BrowserLanguage   string
	UserAgent         string
	CardBIN           string
	CardNetwork       string
	IssuerCountry     string
	CardCountry       string
	FundingType       string
	MerchantCountry   string
}

// EvalResult is the outcome of a risk evaluation.
type EvalResult struct {
	Score          int
	Action         RiskAction
	TriggeredRules []string
}

type MerchantFraudConfig struct {
	MerchantID                string
	IPVelocityThreshold       int
	DeviceVelocityThreshold   int
	MerchantVelocityThreshold int
	AmountSpikeFactor         int
	ReviewThreshold           int
	BlockThreshold            int
	BlockedCountries          []string
	BlockedBINs               []string
	ReviewOnCountryMismatch   bool
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

// DefaultThresholds are the out-of-the-box risk rule thresholds.
// These can be overridden per-merchant in a future settings table.
const (
	ThresholdMerchantTxnPerHour = 200 // max transactions per merchant per hour
	ThresholdIPTxnPerHour       = 20  // max transactions per IP per hour
	ThresholdDeviceTxnPerHour   = 6
	ThresholdAmountSpikeFactor  = 3    // flag if amount > 3x merchant average
	ThresholdAmountSpikeMinAvg  = 1000 // only apply spike check when avg > 10 INR (in paise)
	ScoreVelocityMerchant       = 50
	ScoreVelocityIP             = 50
	ScoreVelocityDevice         = 40
	ScoreAmountSpike            = 50
	ScoreCountryMismatch        = 40
	ScoreCountryBlocked         = 90
	ScoreBINBlocked             = 90
	ScoreHoldThreshold          = 40 // score >= 40 → hold; any single rule triggers hold
	ScoreBlockThreshold         = 90 // score >= 90 → block; any two rules trigger block
)

func DefaultMerchantFraudConfig(merchantID string) MerchantFraudConfig {
	return MerchantFraudConfig{
		MerchantID:                merchantID,
		IPVelocityThreshold:       ThresholdIPTxnPerHour,
		DeviceVelocityThreshold:   ThresholdDeviceTxnPerHour,
		MerchantVelocityThreshold: ThresholdMerchantTxnPerHour,
		AmountSpikeFactor:         ThresholdAmountSpikeFactor,
		ReviewThreshold:           ScoreHoldThreshold,
		BlockThreshold:            ScoreBlockThreshold,
		BlockedCountries:          nil,
		BlockedBINs:               nil,
		ReviewOnCountryMismatch:   true,
	}
}

// Evaluate computes a risk score and action for the given input.
// It does NOT query the database — callers must pass in pre-fetched counters.
func Evaluate(in EvalInput, cfg MerchantFraudConfig, merchantHourlyCount, ipHourlyCount, deviceHourlyCount int) EvalResult {
	var score int
	var rules []string

	if merchantHourlyCount >= cfg.MerchantVelocityThreshold {
		score += ScoreVelocityMerchant
		rules = append(rules, "merchant_velocity_1h")
	}

	if in.IPAddress != "" && ipHourlyCount >= cfg.IPVelocityThreshold {
		score += ScoreVelocityIP
		rules = append(rules, "ip_velocity_1h")
	}

	if in.DeviceFingerprint != "" && deviceHourlyCount >= cfg.DeviceVelocityThreshold {
		score += ScoreVelocityDevice
		rules = append(rules, "device_velocity_1h")
	}

	if in.MerchantAvgTxn >= ThresholdAmountSpikeMinAvg && in.Amount > in.MerchantAvgTxn*int64(cfg.AmountSpikeFactor) {
		score += ScoreAmountSpike
		rules = append(rules, "amount_spike_3x")
	}

	if in.CardCountry != "" && in.MerchantCountry != "" && in.CardCountry != in.MerchantCountry && cfg.ReviewOnCountryMismatch {
		score += ScoreCountryMismatch
		rules = append(rules, "country_mismatch")
	}

	for _, country := range cfg.BlockedCountries {
		if in.CardCountry != "" && country == in.CardCountry {
			score += ScoreCountryBlocked
			rules = append(rules, "blocked_country")
			break
		}
	}

	for _, bin := range cfg.BlockedBINs {
		if in.CardBIN != "" && bin == in.CardBIN {
			score += ScoreBINBlocked
			rules = append(rules, "blocked_bin")
			break
		}
	}

	action := RiskActionAllow
	switch {
	case score >= cfg.BlockThreshold:
		action = RiskActionBlock
	case score >= cfg.ReviewThreshold:
		action = RiskActionHold
	}

	return EvalResult{
		Score:          score,
		Action:         action,
		TriggeredRules: rules,
	}
}
