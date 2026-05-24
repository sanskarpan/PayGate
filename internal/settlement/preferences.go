package settlement

import "time"

type ScheduleType string
type WeekendPolicy string

const (
	ScheduleDaily  ScheduleType = "daily"
	ScheduleWeekly ScheduleType = "weekly"
	ScheduleManual ScheduleType = "manual"
)

const (
	WeekendPolicyNextBusinessDay WeekendPolicy = "next_business_day"
	WeekendPolicySameDay         WeekendPolicy = "same_day"
)

type Preferences struct {
	MerchantID              string
	ScheduleType            ScheduleType
	WeeklyDayOfWeek         *int
	PayoutMinimum           int64
	ApprovalThresholdAmount int64
	WeekendPolicy           WeekendPolicy
	AutoPayout              bool
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type Statement struct {
	ID           string
	MerchantID   string
	SettlementID string
	Format       string
	FileName     string
	Content      string
	Totals       map[string]any
	CreatedAt    time.Time
}

type Adjustment struct {
	ID             string
	MerchantID     string
	SettlementID   string
	PaymentID      string
	RefundID       string
	AdjustmentType string
	Amount         int64
	Currency       string
	Reason         string
	CreatedAt      time.Time
}

func DefaultPreferences(merchantID string) Preferences {
	return Preferences{
		MerchantID:              merchantID,
		ScheduleType:            ScheduleManual,
		WeekendPolicy:           WeekendPolicyNextBusinessDay,
		PayoutMinimum:           0,
		ApprovalThresholdAmount: 0,
		AutoPayout:              false,
	}
}
