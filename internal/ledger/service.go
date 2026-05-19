package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateEntriesTx(ctx context.Context, tx pgx.Tx, merchantID, sourceType, sourceID, description string, entries []Entry) (string, error) {
	if err := ValidateEntries(entries); err != nil {
		return "", err
	}
	txnID := idgen.New("txn")
	if err := s.repo.InsertTransactionWithEntriesTx(ctx, tx, txnID, merchantID, sourceType, sourceID, description, entries); err != nil {
		return "", fmt.Errorf("insert ledger entries: %w", err)
	}
	return txnID, nil
}

func (s *Service) CreateEntries(ctx context.Context, merchantID, sourceType, sourceID, description string, entries []Entry) (string, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txnID, err := s.CreateEntriesTx(ctx, tx, merchantID, sourceType, sourceID, description, entries)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return txnID, nil
}

func (s *Service) GetBalance(ctx context.Context, merchantID, accountCode string) (int64, error) {
	return s.repo.GetBalance(ctx, merchantID, accountCode)
}

func (s *Service) GetBalanceByCurrency(ctx context.Context, merchantID, accountCode, currency string) (int64, error) {
	if currency == "" {
		currency = "INR"
	}
	return s.repo.GetBalanceByCurrency(ctx, merchantID, accountCode, currency)
}

func (s *Service) CreateHold(ctx context.Context, in CreateHoldInput) (Hold, error) {
	if in.Currency == "" {
		in.Currency = "INR"
	}
	in.AccountCode = strings.TrimSpace(in.AccountCode)
	in.SourceType = strings.TrimSpace(in.SourceType)
	in.SourceID = strings.TrimSpace(in.SourceID)
	if in.AccountCode == "" || in.SourceType == "" || in.SourceID == "" || in.Amount <= 0 {
		return Hold{}, fmt.Errorf("invalid hold input")
	}
	return s.repo.CreateHold(ctx, in)
}

func (s *Service) GetHold(ctx context.Context, merchantID, holdID string) (Hold, error) {
	return s.repo.GetHold(ctx, merchantID, holdID)
}

func (s *Service) ListHolds(ctx context.Context, merchantID string, limit int) ([]Hold, error) {
	return s.repo.ListHolds(ctx, merchantID, limit)
}

func (s *Service) ReleaseHold(ctx context.Context, in ReleaseHoldInput) (Hold, error) {
	return s.repo.ReleaseHold(ctx, in.MerchantID, in.HoldID)
}

func (s *Service) ExtendHold(ctx context.Context, in ExtendHoldInput) (Hold, error) {
	return s.repo.ExtendHold(ctx, in)
}

func (s *Service) CommitHold(ctx context.Context, in CommitHoldInput) (Hold, error) {
	if strings.TrimSpace(in.TargetAccountCode) == "" {
		return Hold{}, fmt.Errorf("target account code is required")
	}
	if strings.TrimSpace(in.Description) == "" {
		in.Description = "ledger hold commit"
	}
	return s.repo.CommitHold(ctx, in)
}

func (s *Service) ExpireDueHolds(ctx context.Context, limit int) (int64, error) {
	return s.repo.ExpireDueHolds(ctx, limit)
}

func (s *Service) ExpireHold(ctx context.Context, in ExpireHoldInput) (Hold, error) {
	return s.repo.ExpireHold(ctx, in.MerchantID, in.HoldID)
}

func (s *Service) GetReservedBalance(ctx context.Context, merchantID, accountCode, currency string) (int64, error) {
	if currency == "" {
		currency = "INR"
	}
	return s.repo.SumActiveHolds(ctx, merchantID, accountCode, currency)
}

func (s *Service) GetPayoutableBalance(ctx context.Context, merchantID, currency string) (int64, error) {
	balance, err := s.GetBalanceByCurrency(ctx, merchantID, "SETTLEMENT_CLEARING", currency)
	if err != nil {
		return 0, err
	}
	reserved, err := s.GetReservedBalance(ctx, merchantID, "SETTLEMENT_CLEARING", currency)
	if err != nil {
		return 0, err
	}
	available := int64(0)
	if balance < 0 {
		available = -balance
	}
	available -= reserved
	if available < 0 {
		return 0, nil
	}
	return available, nil
}

func (s *Service) CanReserveForPayout(ctx context.Context, merchantID, currency string, amount int64) error {
	available, err := s.GetPayoutableBalance(ctx, merchantID, currency)
	if err != nil {
		return err
	}
	if available < amount {
		return ErrHoldInsufficient
	}
	return nil
}

func (s *Service) DefaultHoldExpiry(after time.Duration) *time.Time {
	if after <= 0 {
		return nil
	}
	tm := time.Now().Add(after).UTC()
	return &tm
}
