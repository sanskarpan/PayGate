package ledger

import (
	"context"

	ledgerpb "github.com/sanskarpan/PayGate/internal/ledger/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GRPCServer struct {
	ledgerpb.UnimplementedLedgerServiceServer
	svc *Service
}

func NewGRPCServer(svc *Service) *GRPCServer {
	return &GRPCServer{svc: svc}
}

func (s *GRPCServer) CreateEntries(ctx context.Context, req *ledgerpb.CreateEntriesRequest) (*ledgerpb.CreateEntriesResponse, error) {
	entries := make([]Entry, 0, len(req.Entries))
	for _, e := range req.Entries {
		entries = append(entries, Entry{
			AccountCode:  e.AccountCode,
			DebitAmount:  e.DebitAmount,
			CreditAmount: e.CreditAmount,
			Currency:     e.Currency,
			Description:  e.Description,
		})
	}
	txnID, err := s.svc.CreateEntries(ctx, req.MerchantId, req.SourceType, req.SourceId, "grpc create entries", entries)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "create entries: %v", err)
	}
	return &ledgerpb.CreateEntriesResponse{TransactionId: txnID}, nil
}

func (s *GRPCServer) GetBalance(ctx context.Context, req *ledgerpb.GetBalanceRequest) (*ledgerpb.GetBalanceResponse, error) {
	bal, err := s.svc.GetBalance(ctx, req.MerchantId, req.AccountCode)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get balance: %v", err)
	}
	return &ledgerpb.GetBalanceResponse{MerchantId: req.MerchantId, AccountCode: req.AccountCode, Balance: bal}, nil
}
