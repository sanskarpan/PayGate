package tokenization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

type CardTokenClass string
type CardTokenStatus string

const (
	CardTokenClassSingleUse CardTokenClass = "single_use"
	CardTokenClassReusable  CardTokenClass = "reusable"
)

const (
	CardTokenStatusActive   CardTokenStatus = "active"
	CardTokenStatusConsumed CardTokenStatus = "consumed"
	CardTokenStatusDisabled CardTokenStatus = "disabled"
)

var (
	ErrInvalidCardNumber = errors.New("invalid card number")
	ErrInvalidExpiry     = errors.New("invalid card expiry")
	ErrCardTokenNotFound = errors.New("card token not found")
	ErrCardTokenInactive = errors.New("card token is not active")
)

type CardToken struct {
	ID               string
	MerchantID       string
	TokenClass       CardTokenClass
	Status           CardTokenStatus
	Last4            string
	BIN              string
	Brand            string
	ExpMonth         int
	ExpYear          int
	CustomerRef      string
	NetworkReference string
	CreatedAt        time.Time
	LastUsedAt       *time.Time
	ConsumedAt       *time.Time
	DisabledAt       *time.Time
}

type CardTokenReference struct {
	TokenID      string
	TokenClass   CardTokenClass
	Brand        string
	Last4        string
	ExpMonth     int
	ExpYear      int
	CustomerRef  string
	LastUsedAt   *time.Time
	ConsumedAt   *time.Time
	DisabledAt   *time.Time
	NetworkToken string
}

type CreateCardTokenInput struct {
	MerchantID       string
	CardNumber       string
	ExpMonth         int
	ExpYear          int
	CardholderName   string
	CustomerRef      string
	Reusable         bool
	NetworkReference string
}

type Repository interface {
	CreateCardToken(ctx context.Context, in CreateCardTokenRecordInput) (CardToken, error)
	GetCardToken(ctx context.Context, merchantID, tokenID, action, reason, actor string) (CardToken, error)
	PrepareCardTokenAuthorization(ctx context.Context, merchantID, tokenID, paymentID string, usedAt time.Time) (CardToken, error)
	DisableCardToken(ctx context.Context, merchantID, tokenID, reason, actor string, disabledAt time.Time) (CardToken, error)
}

type CreateCardTokenRecordInput struct {
	ID               string
	MerchantID       string
	TokenClass       CardTokenClass
	FingerprintHash  string
	Last4            string
	BIN              string
	Brand            string
	ExpMonth         int
	ExpYear          int
	CustomerRef      string
	NetworkReference string
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateCardToken(ctx context.Context, in CreateCardTokenInput) (CardToken, error) {
	pan := digitsOnly(in.CardNumber)
	if !validLuhn(pan) || len(pan) < 12 || len(pan) > 19 {
		return CardToken{}, ErrInvalidCardNumber
	}
	if !validExpiry(in.ExpMonth, in.ExpYear, time.Now().UTC()) {
		return CardToken{}, ErrInvalidExpiry
	}
	tokenClass := CardTokenClassSingleUse
	if in.Reusable {
		tokenClass = CardTokenClassReusable
	}
	if in.MerchantID == "" {
		return CardToken{}, ErrCardTokenNotFound
	}
	brand := inferBrand(pan)
	fingerprint := fingerprintHash(in.MerchantID, pan, in.ExpMonth, in.ExpYear)
	return s.repo.CreateCardToken(ctx, CreateCardTokenRecordInput{
		ID:               idgen.New("ctok"),
		MerchantID:       in.MerchantID,
		TokenClass:       tokenClass,
		FingerprintHash:  fingerprint,
		Last4:            pan[len(pan)-4:],
		BIN:              pan[:6],
		Brand:            brand,
		ExpMonth:         in.ExpMonth,
		ExpYear:          in.ExpYear,
		CustomerRef:      strings.TrimSpace(in.CustomerRef),
		NetworkReference: strings.TrimSpace(in.NetworkReference),
	})
}

func (s *Service) GetCardToken(ctx context.Context, merchantID, tokenID string) (CardToken, error) {
	return s.repo.GetCardToken(ctx, merchantID, tokenID, "view", "", "merchant_api")
}

func (s *Service) DisableCardToken(ctx context.Context, merchantID, tokenID, reason string) (CardToken, error) {
	return s.repo.DisableCardToken(ctx, merchantID, tokenID, reason, "merchant_api", time.Now().UTC())
}

func (s *Service) PrepareAuthorization(ctx context.Context, merchantID, tokenID, paymentID string) (CardTokenReference, error) {
	token, err := s.repo.PrepareCardTokenAuthorization(ctx, merchantID, tokenID, paymentID, time.Now().UTC())
	if err != nil {
		return CardTokenReference{}, err
	}
	return CardTokenReference{
		TokenID:      token.ID,
		TokenClass:   token.TokenClass,
		Brand:        token.Brand,
		Last4:        token.Last4,
		ExpMonth:     token.ExpMonth,
		ExpYear:      token.ExpYear,
		CustomerRef:  token.CustomerRef,
		LastUsedAt:   token.LastUsedAt,
		ConsumedAt:   token.ConsumedAt,
		DisabledAt:   token.DisabledAt,
		NetworkToken: token.NetworkReference,
	}, nil
}

func digitsOnly(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func validLuhn(pan string) bool {
	if pan == "" {
		return false
	}
	sum := 0
	double := false
	for i := len(pan) - 1; i >= 0; i-- {
		d := int(pan[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

func validExpiry(month, year int, now time.Time) bool {
	if month < 1 || month > 12 {
		return false
	}
	if year < now.Year() {
		return false
	}
	if year == now.Year() && month < int(now.Month()) {
		return false
	}
	return year <= now.Year()+25
}

func inferBrand(pan string) string {
	switch {
	case strings.HasPrefix(pan, "4"):
		return "visa"
	case strings.HasPrefix(pan, "34"), strings.HasPrefix(pan, "37"):
		return "amex"
	case strings.HasPrefix(pan, "36"), strings.HasPrefix(pan, "38"), strings.HasPrefix(pan, "60"):
		return "discover"
	case len(pan) >= 2 && betweenPrefix(pan[:2], 51, 55):
		return "mastercard"
	case len(pan) >= 4 && betweenPrefix(pan[:4], 2221, 2720):
		return "mastercard"
	case strings.HasPrefix(pan, "65"):
		return "rupay"
	default:
		return "card"
	}
}

func betweenPrefix(v string, min, max int) bool {
	n, err := strconv.Atoi(v)
	return err == nil && n >= min && n <= max
}

func fingerprintHash(merchantID, pan string, expMonth, expYear int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%02d:%04d", merchantID, pan, expMonth, expYear)))
	return hex.EncodeToString(sum[:])
}
