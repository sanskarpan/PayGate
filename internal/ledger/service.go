package ledger

import (
	"context"
	"fmt"

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
