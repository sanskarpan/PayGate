package upiverify

import "context"

type Repository interface {
	GetLatest(ctx context.Context, merchantID, vpa string, purpose Purpose) (Verification, error)
	Record(ctx context.Context, verification Verification) (Verification, error)
}
