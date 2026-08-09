//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/sanskarpan/PayGate/internal/gateway"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
)

// An order must never be chargeable for more than its own amount. Before this
// guard existed an order could be authorized repeatedly for the full amount
// while it sat in "attempted", and every one of those authorizations could then
// be captured — a 4x overcharge with real ledger movement.
func TestIntegrationOrderCannotBeAuthorizedBeyondItsAmount(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	merchantID := "merch_exposure_1"
	if _, err := db.Exec(ctx, `INSERT INTO paygate_merchants.merchants(id,name,email,business_type,status,settings)
VALUES($1,'Exposure','exposure@test.com','company','active','{}') ON CONFLICT (id) DO NOTHING`, merchantID); err != nil {
		t.Fatalf("seed merchant: %v", err)
	}

	orderSvc := order.NewService(order.NewPostgresRepository(db))
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	paySvc := payment.NewService(payment.NewPostgresRepository(db, ledgerSvc, orderSvc), gateway.NewSimulator())

	o, err := orderSvc.Create(ctx, order.CreateInput{MerchantID: merchantID, Amount: 50000, Currency: "INR"})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	if _, err := paySvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: merchantID, OrderID: o.ID, Amount: 50000, Currency: "INR", Method: "card",
	}); err != nil {
		t.Fatalf("first authorize should succeed: %v", err)
	}

	// The order is still "attempted" at this point — this is the window the bug lived in.
	for i := 0; i < 3; i++ {
		_, err := paySvc.Authorize(ctx, payment.AuthorizeInput{
			MerchantID: merchantID, OrderID: o.ID, Amount: 50000, Currency: "INR", Method: "card",
		})
		if !errors.Is(err, payment.ErrOrderAmountExceeded) {
			t.Fatalf("authorize %d should have been refused with ErrOrderAmountExceeded, got %v", i+2, err)
		}
	}

	var count int
	var total int64
	if err := db.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(amount),0) FROM paygate_payments.payments WHERE order_id=$1`, o.ID,
	).Scan(&count, &total); err != nil {
		t.Fatalf("query payments: %v", err)
	}
	if count != 1 || total != 50000 {
		t.Fatalf("expected exactly one payment totalling 50000, got %d payments totalling %d", count, total)
	}
}

// Partial-payment orders must still accept multiple payments, up to the total.
func TestIntegrationPartialPaymentOrderAcceptsMultiplePaymentsUpToTotal(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	merchantID := "merch_exposure_2"
	if _, err := db.Exec(ctx, `INSERT INTO paygate_merchants.merchants(id,name,email,business_type,status,settings)
VALUES($1,'Exposure2','exposure2@test.com','company','active','{}') ON CONFLICT (id) DO NOTHING`, merchantID); err != nil {
		t.Fatalf("seed merchant: %v", err)
	}

	orderSvc := order.NewService(order.NewPostgresRepository(db))
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	paySvc := payment.NewService(payment.NewPostgresRepository(db, ledgerSvc, orderSvc), gateway.NewSimulator())

	o, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: merchantID, Amount: 100000, Currency: "INR", PartialPayment: true,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, err := paySvc.Authorize(ctx, payment.AuthorizeInput{
			MerchantID: merchantID, OrderID: o.ID, Amount: 40000, Currency: "INR", Method: "card",
		}); err != nil {
			t.Fatalf("partial authorize %d should succeed: %v", i+1, err)
		}
	}

	// A third 40000 would reach 120000 against a 100000 order.
	_, err = paySvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: merchantID, OrderID: o.ID, Amount: 40000, Currency: "INR", Method: "card",
	})
	if !errors.Is(err, payment.ErrOrderAmountExceeded) {
		t.Fatalf("expected the over-committing partial payment to be refused, got %v", err)
	}
}

// An unsupported method must be a client error, not a constraint violation.
func TestIntegrationAuthorizeRejectsUnknownMethod(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	merchantID := "merch_exposure_3"
	if _, err := db.Exec(ctx, `INSERT INTO paygate_merchants.merchants(id,name,email,business_type,status,settings)
VALUES($1,'Exposure3','exposure3@test.com','company','active','{}') ON CONFLICT (id) DO NOTHING`, merchantID); err != nil {
		t.Fatalf("seed merchant: %v", err)
	}

	orderSvc := order.NewService(order.NewPostgresRepository(db))
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	paySvc := payment.NewService(payment.NewPostgresRepository(db, ledgerSvc, orderSvc), gateway.NewSimulator())

	o, err := orderSvc.Create(ctx, order.CreateInput{MerchantID: merchantID, Amount: 1000, Currency: "INR"})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	for _, method := range []string{"CARD", "telepathy", "<script>alert(1)</script>"} {
		_, err := paySvc.Authorize(ctx, payment.AuthorizeInput{
			MerchantID: merchantID, OrderID: o.ID, Amount: 1000, Currency: "INR", Method: method,
		})
		if !errors.Is(err, payment.ErrInvalidPaymentMethod) {
			t.Fatalf("method %q should be refused as invalid, got %v", method, err)
		}
	}
}
