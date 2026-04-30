//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/sanskarpan/PayGate/internal/gateway"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
)

func TestIntegrationOrderPaymentCaptureFlow(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	_, _ = db.Exec(ctx, `INSERT INTO paygate_merchants.merchants(id,name,email,business_type,status,settings) VALUES('merch_int_2','M2','m2@test.com','company','active','{}') ON CONFLICT (id) DO NOTHING`)

	orderRepo := order.NewPostgresRepository(db)
	orderSvc := order.NewService(orderRepo)
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	payRepo := payment.NewPostgresRepository(db, ledgerSvc, orderSvc)
	paySvc := payment.NewService(payRepo, gateway.NewSimulator())

	o, err := orderSvc.Create(ctx, order.CreateInput{MerchantID: "merch_int_2", Amount: 10000, Currency: "INR"})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	auth, err := paySvc.Authorize(ctx, payment.AuthorizeInput{MerchantID: "merch_int_2", OrderID: o.ID, Amount: 10000, Currency: "INR", Method: "card"})
	if err != nil {
		t.Fatalf("authorize payment: %v", err)
	}
	if auth.Status != payment.StateAuthorized {
		t.Fatalf("expected authorized, got %s", auth.Status)
	}

	captured, err := paySvc.CaptureForMerchant(ctx, "merch_int_2", auth.PaymentID, 10000)
	if err != nil {
		t.Fatalf("capture payment: %v", err)
	}
	if captured.Status != payment.StateCaptured {
		t.Fatalf("expected captured, got %s", captured.Status)
	}

	var orderStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM paygate_orders.orders WHERE id=$1`, o.ID).Scan(&orderStatus); err != nil {
		t.Fatalf("query order status: %v", err)
	}
	if orderStatus != "paid" {
		t.Fatalf("expected order paid, got %s", orderStatus)
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM paygate_ledger.ledger_entries WHERE source_type='payment' AND source_id=$1`, captured.PaymentID).Scan(&count); err != nil {
		t.Fatalf("query ledger entries: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 ledger entries for capture, got %d", count)
	}
}

func TestIntegrationCaptureRejectsCrossMerchantAccess(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	_, _ = db.Exec(ctx, `INSERT INTO paygate_merchants.merchants(id,name,email,business_type,status,settings) VALUES('merch_int_5','M5','m5@test.com','company','active','{}') ON CONFLICT (id) DO NOTHING`)
	_, _ = db.Exec(ctx, `INSERT INTO paygate_merchants.merchants(id,name,email,business_type,status,settings) VALUES('merch_int_6','M6','m6@test.com','company','active','{}') ON CONFLICT (id) DO NOTHING`)

	orderRepo := order.NewPostgresRepository(db)
	orderSvc := order.NewService(orderRepo)
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	payRepo := payment.NewPostgresRepository(db, ledgerSvc, orderSvc)
	paySvc := payment.NewService(payRepo, gateway.NewSimulator())

	o, err := orderSvc.Create(ctx, order.CreateInput{MerchantID: "merch_int_5", Amount: 10000, Currency: "INR"})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	auth, err := paySvc.Authorize(ctx, payment.AuthorizeInput{MerchantID: "merch_int_5", OrderID: o.ID, Amount: 10000, Currency: "INR", Method: "card"})
	if err != nil {
		t.Fatalf("authorize payment: %v", err)
	}

	_, err = paySvc.CaptureForMerchant(ctx, "merch_int_6", auth.PaymentID, 10000)
	if err == nil {
		t.Fatal("expected cross-merchant capture to fail")
	}
}
