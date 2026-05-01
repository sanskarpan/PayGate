//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/recon"
	"github.com/sanskarpan/PayGate/internal/settlement"
)

func TestIntegrationReconLedgerBalanceHappyPath(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	_, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Recon Merchant", Email: "recon@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}

	// Capture a payment to create balanced ledger entries.
	o, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID, Amount: 8000, Currency: "INR", Receipt: "recon-1",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	auth, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: createdMerchant.ID, OrderID: o.ID,
		Amount: o.Amount, Currency: o.Currency, Method: "card",
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, auth.PaymentID, o.Amount); err != nil {
		t.Fatalf("capture: %v", err)
	}

	// Ledger balance check: should find zero mismatches on a balanced ledger.
	worker := recon.NewWorker(db, nil)
	n, err := worker.RunLedgerBalanceCheck(ctx)
	if err != nil {
		t.Fatalf("ledger balance check: %v", err)
	}
	if n > 0 {
		t.Fatalf("expected 0 ledger mismatches on happy path, got %d", n)
	}
}

func TestIntegrationReconPaymentLedgerHappyPath(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	_, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Recon PL Merchant", Email: "recon-pl@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}

	o, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID, Amount: 6000, Currency: "INR", Receipt: "recon-pl-1",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	auth, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: createdMerchant.ID, OrderID: o.ID,
		Amount: o.Amount, Currency: o.Currency, Method: "card",
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, auth.PaymentID, o.Amount); err != nil {
		t.Fatalf("capture: %v", err)
	}

	start := time.Now().Add(-time.Minute)
	end := time.Now().Add(time.Minute)
	worker := recon.NewWorker(db, nil)
	n, err := worker.RunPaymentLedgerCheck(ctx, start, end)
	if err != nil {
		t.Fatalf("payment-ledger check: %v", err)
	}
	if n > 0 {
		t.Fatalf("expected 0 payment-ledger mismatches on happy path, got %d", n)
	}
}

func TestIntegrationReconThreeWayHappyPath(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	_, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Recon TW Merchant", Email: "recon-tw@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}

	o, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID, Amount: 4000, Currency: "INR", Receipt: "recon-tw-1",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	auth, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: createdMerchant.ID, OrderID: o.ID,
		Amount: o.Amount, Currency: o.Currency, Method: "card",
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, auth.PaymentID, o.Amount); err != nil {
		t.Fatalf("capture: %v", err)
	}

	// Settle the payment.
	sttlSvc := settlement.NewService(settlement.NewPostgresRepository(db, ledger.NewService(ledger.NewRepository(db))))
	if _, err := sttlSvc.RunBatch(ctx, createdMerchant.ID, time.Unix(0, 0), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("settlement batch: %v", err)
	}

	start := time.Now().Add(-time.Minute)
	end := time.Now().Add(time.Minute)
	worker := recon.NewWorker(db, nil)
	n, err := worker.RunThreeWayCheck(ctx, start, end)
	if err != nil {
		t.Fatalf("three-way check: %v", err)
	}
	if n > 0 {
		t.Fatalf("expected 0 three-way mismatches on happy path, got %d", n)
	}
}

func TestIntegrationReconDetectsMismatch(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	_, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Recon Mismatch Merchant", Email: "recon-mm@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}

	o, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID, Amount: 3000, Currency: "INR", Receipt: "recon-mm-1",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	auth, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: createdMerchant.ID, OrderID: o.ID,
		Amount: o.Amount, Currency: o.Currency, Method: "card",
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, auth.PaymentID, o.Amount); err != nil {
		t.Fatalf("capture: %v", err)
	}

	// Inject a mismatch: mark payment as settled without creating a settlement_item.
	if _, err := db.Exec(ctx,
		`UPDATE paygate_payments.payments SET settled = true, settlement_id = 'sttl_fake' WHERE id = $1`,
		auth.PaymentID,
	); err != nil {
		t.Fatalf("inject mismatch: %v", err)
	}

	start := time.Now().Add(-time.Minute)
	end := time.Now().Add(time.Minute)
	worker := recon.NewWorker(db, nil)
	n, err := worker.RunThreeWayCheck(ctx, start, end)
	if err != nil {
		t.Fatalf("three-way check: %v", err)
	}
	if n == 0 {
		t.Fatal("expected at least 1 three-way mismatch for intentionally bad data, got 0")
	}

	// Verify mismatch was recorded in DB.
	var count int
	if err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM paygate_recon.recon_mismatches WHERE entity_id = $1`,
		auth.PaymentID,
	).Scan(&count); err != nil {
		t.Fatalf("query recon mismatches: %v", err)
	}
	if count == 0 {
		t.Fatal("expected mismatch to be persisted in recon_mismatches table")
	}
}
