//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/settlement"
)

func TestIntegrationPaymentAuthorizationReverseBlocksCaptureAndWritesEvent(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	createdMerchant, authHeader := createMerchantAndWriteKey(t, ctx, merchantSvc, "auth-reverse@test.com")
	createdOrder := createOrderForMerchant(t, ctx, orderSvc, createdMerchant.ID, 6500, "auth-reverse-order")
	authorized := authorizeAndCaptureOptional(t, ctx, paymentSvc, createdMerchant.ID, createdOrder, false)

	reverseReq := httptest.NewRequest(http.MethodPost, "/v1/payments/"+authorized.PaymentID+"/reverse-authorization", bytes.NewReader([]byte(`{"reason":"operator_requested_reversal"}`)))
	reverseReq.Header.Set("Authorization", authHeader)
	reverseReq.Header.Set("Content-Type", "application/json")
	reverseRec := httptest.NewRecorder()
	mux.ServeHTTP(reverseRec, reverseReq)
	if reverseRec.Code != http.StatusOK {
		t.Fatalf("reverse authorization: expected 200, got %d body=%s", reverseRec.Code, reverseRec.Body.String())
	}
	if !bytes.Contains(reverseRec.Body.Bytes(), []byte(`"status":"authorization_reversed"`)) {
		t.Fatalf("expected authorization_reversed response, got %s", reverseRec.Body.String())
	}

	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, authorized.PaymentID, createdOrder.Amount); !errors.Is(err, payment.ErrInvalidTransition) {
		t.Fatalf("expected capture to be blocked after reversal, got %v", err)
	}

	var paymentStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM paygate_payments.payments WHERE id = $1`, authorized.PaymentID).Scan(&paymentStatus); err != nil {
		t.Fatalf("query payment status: %v", err)
	}
	if paymentStatus != string(payment.StateAuthorizationReversed) {
		t.Fatalf("expected payment status %s, got %s", payment.StateAuthorizationReversed, paymentStatus)
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM public.outbox WHERE aggregate_id = $1 AND event_type = 'payment.authorization_reversed'`, authorized.PaymentID).Scan(&count); err != nil {
		t.Fatalf("query outbox event: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one payment.authorization_reversed event, got %d", count)
	}
}

func TestIntegrationRefundReverseRestoresBalancesAndWritesCorrectiveEntries(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	createdMerchant, authHeader := createMerchantAndWriteKey(t, ctx, merchantSvc, "refund-reverse@test.com")
	createdOrder := createOrderForMerchant(t, ctx, orderSvc, createdMerchant.ID, 10000, "refund-reverse-order")
	captured := authorizeAndCaptureOptional(t, ctx, paymentSvc, createdMerchant.ID, createdOrder, true)

	refundReq := httptest.NewRequest(http.MethodPost, "/v1/payments/"+captured.PaymentID+"/refunds", bytes.NewReader([]byte(`{"amount":10000,"reason":"duplicate_refund","notes":{"source":"integration"}}`)))
	refundReq.Header.Set("Authorization", authHeader)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	mux.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusCreated {
		t.Fatalf("create refund: expected 201, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}

	var refundBody map[string]any
	if err := json.Unmarshal(refundRec.Body.Bytes(), &refundBody); err != nil {
		t.Fatalf("decode refund create response: %v", err)
	}
	refundID, _ := refundBody["id"].(string)
	if refundID == "" {
		t.Fatal("expected refund id")
	}

	reverseReq := httptest.NewRequest(http.MethodPost, "/v1/refunds/"+refundID+"/reverse", bytes.NewReader([]byte(`{"reason":"compensate_duplicate_refund"}`)))
	reverseReq.Header.Set("Authorization", authHeader)
	reverseReq.Header.Set("Content-Type", "application/json")
	reverseRec := httptest.NewRecorder()
	mux.ServeHTTP(reverseRec, reverseReq)
	if reverseRec.Code != http.StatusOK {
		t.Fatalf("reverse refund: expected 200, got %d body=%s", reverseRec.Code, reverseRec.Body.String())
	}
	if !bytes.Contains(reverseRec.Body.Bytes(), []byte(`"status":"reversed"`)) {
		t.Fatalf("expected reversed refund response, got %s", reverseRec.Body.String())
	}

	var refundedAmount int64
	var refundStatus string
	if err := db.QueryRow(ctx, `SELECT amount_refunded, refund_status FROM paygate_payments.payments WHERE id = $1`, captured.PaymentID).Scan(&refundedAmount, &refundStatus); err != nil {
		t.Fatalf("query payment refund counters: %v", err)
	}
	if refundedAmount != 0 || refundStatus != "none" {
		t.Fatalf("expected payment refund counters reset, got amount_refunded=%d refund_status=%s", refundedAmount, refundStatus)
	}

	merchantPayable, err := ledgerBalanceByCurrency(ctx, db, createdMerchant.ID, "MERCHANT_PAYABLE", "INR")
	if err != nil {
		t.Fatalf("merchant payable balance: %v", err)
	}
	if merchantPayable != -9800 {
		t.Fatalf("expected merchant payable balance -9800 after reversal, got %d", merchantPayable)
	}
	refundClearing, err := ledgerBalanceByCurrency(ctx, db, createdMerchant.ID, "REFUND_CLEARING", "INR")
	if err != nil {
		t.Fatalf("refund clearing balance: %v", err)
	}
	if refundClearing != 0 {
		t.Fatalf("expected refund clearing balance 0 after reversal, got %d", refundClearing)
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM public.outbox WHERE aggregate_id = $1 AND event_type = 'refund.reversed'`, refundID).Scan(&count); err != nil {
		t.Fatalf("query refund.reversed outbox event: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one refund.reversed outbox event, got %d", count)
	}
}

func TestIntegrationSettlementRollbackMarkerBlocksPayout(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	_, authHeader, sttl := createSettledMerchantFlowForCompensation(t, ctx, db, merchantSvc, orderSvc, paymentSvc)

	markReq := httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/rollback-marker", bytes.NewReader([]byte(`{"reason":"reconciliation_gap_detected"}`)))
	markReq.Header.Set("Authorization", authHeader)
	markReq.Header.Set("Content-Type", "application/json")
	markRec := httptest.NewRecorder()
	mux.ServeHTTP(markRec, markReq)
	if markRec.Code != http.StatusOK {
		t.Fatalf("mark rollback: expected 200, got %d body=%s", markRec.Code, markRec.Body.String())
	}
	if !bytes.Contains(markRec.Body.Bytes(), []byte(`"rollback_reason":"reconciliation_gap_detected"`)) {
		t.Fatalf("expected rollback reason in response, got %s", markRec.Body.String())
	}

	beneficiaryID := createApprovedBeneficiary(t, mux, authHeader)
	payoutReq := httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/payout", bytes.NewReader([]byte(`{"beneficiary_id":"`+beneficiaryID+`"}`)))
	payoutReq.Header.Set("Content-Type", "application/json")
	payoutReq.Header.Set("Authorization", authHeader)
	payoutRec := httptest.NewRecorder()
	mux.ServeHTTP(payoutRec, payoutReq)
	if payoutRec.Code != http.StatusConflict {
		t.Fatalf("expected payout initiation to be blocked with 409, got %d body=%s", payoutRec.Code, payoutRec.Body.String())
	}
	if !bytes.Contains(payoutRec.Body.Bytes(), []byte(`"code":"SETTLEMENT_ROLLBACK_MARKED"`)) &&
		!bytes.Contains(payoutRec.Body.Bytes(), []byte(`"code":"SETTLEMENT_ON_HOLD"`)) {
		t.Fatalf("expected payout blocking error, got %s", payoutRec.Body.String())
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM public.outbox WHERE aggregate_id = $1 AND event_type = 'settlement.rollback_marked'`, sttl.ID).Scan(&count); err != nil {
		t.Fatalf("query settlement.rollback_marked outbox event: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one settlement.rollback_marked event, got %d", count)
	}
}

func createMerchantAndWriteKey(t *testing.T, ctx context.Context, merchantSvc *merchant.Service, email string) (merchant.Merchant, string) {
	t.Helper()
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Integration Merchant", Email: email, BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeTest,
		Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return createdMerchant, basicAuth(key.KeyID, key.KeySecret)
}

func createOrderForMerchant(t *testing.T, ctx context.Context, orderSvc *order.Service, merchantID string, amount int64, receipt string) order.Order {
	t.Helper()
	createdOrder, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: merchantID,
		Amount:     amount,
		Currency:   "INR",
		Receipt:    receipt,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	return createdOrder
}

func authorizeAndCaptureOptional(t *testing.T, ctx context.Context, paymentSvc *payment.Service, merchantID string, createdOrder order.Order, capture bool) payment.CaptureResult {
	t.Helper()
	out, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: merchantID,
		OrderID:    createdOrder.ID,
		Amount:     createdOrder.Amount,
		Currency:   createdOrder.Currency,
		Method:     "card",
	})
	if err != nil {
		t.Fatalf("authorize payment: %v", err)
	}
	if !capture {
		return out
	}
	captured, err := paymentSvc.CaptureForMerchant(ctx, merchantID, out.PaymentID, createdOrder.Amount)
	if err != nil {
		t.Fatalf("capture payment: %v", err)
	}
	return captured
}

func ledgerBalanceByCurrency(ctx context.Context, db *pgxpool.Pool, merchantID, accountCode, currency string) (int64, error) {
	var balance int64
	err := db.QueryRow(ctx, `
SELECT COALESCE(SUM(debit_amount - credit_amount), 0)
FROM paygate_ledger.ledger_entries
WHERE merchant_id = $1 AND account_code = $2 AND currency = $3
`, merchantID, accountCode, currency).Scan(&balance)
	return balance, err
}

func createSettledMerchantFlowForCompensation(t *testing.T, ctx context.Context, db *pgxpool.Pool, merchantSvc *merchant.Service, orderSvc *order.Service, paymentSvc *payment.Service) (string, string, settlementSnapshot) {
	t.Helper()

	createdMerchant, authHeader := createMerchantAndWriteKey(t, ctx, merchantSvc, "settlement-rollback@test.com")
	createdOrder := createOrderForMerchant(t, ctx, orderSvc, createdMerchant.ID, 9500, "settlement-rollback-order")
	authorizeAndCaptureOptional(t, ctx, paymentSvc, createdMerchant.ID, createdOrder, true)

	settlementSvc := settlement.NewService(settlement.NewPostgresRepository(db, ledger.NewService(ledger.NewRepository(db))))
	sttl, err := settlementSvc.RunBatch(ctx, createdMerchant.ID, time.Unix(0, 0), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("run settlement batch: %v", err)
	}

	return createdMerchant.ID, authHeader, settlementSnapshot{ID: sttl.ID}
}

type settlementSnapshot struct {
	ID string
}
