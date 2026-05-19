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

	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/webhook"
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

	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	merchantPayable, err := ledgerSvc.GetBalanceByCurrency(ctx, createdMerchant.ID, "MERCHANT_PAYABLE", "INR")
	if err != nil {
		t.Fatalf("merchant payable balance: %v", err)
	}
	if merchantPayable != -9800 {
		t.Fatalf("expected merchant payable balance -9800 after reversal, got %d", merchantPayable)
	}
	refundClearing, err := ledgerSvc.GetBalanceByCurrency(ctx, createdMerchant.ID, "REFUND_CLEARING", "INR")
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
	_, authHeader, sttl := createSettledMerchantFlow(t, ctx, db, merchantSvc, orderSvc, paymentSvc)

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

	payoutReq := httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/payout", nil)
	payoutReq.Header.Set("Authorization", authHeader)
	payoutRec := httptest.NewRecorder()
	mux.ServeHTTP(payoutRec, payoutReq)
	if payoutRec.Code != http.StatusConflict {
		t.Fatalf("expected payout initiation to be blocked with 409, got %d body=%s", payoutRec.Code, payoutRec.Body.String())
	}
	if !bytes.Contains(payoutRec.Body.Bytes(), []byte(`"code":"SETTLEMENT_ROLLBACK_MARKED"`)) {
		t.Fatalf("expected SETTLEMENT_ROLLBACK_MARKED error, got %s", payoutRec.Body.String())
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM public.outbox WHERE aggregate_id = $1 AND event_type = 'settlement.rollback_marked'`, sttl.ID).Scan(&count); err != nil {
		t.Fatalf("query settlement.rollback_marked outbox event: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one settlement.rollback_marked event, got %d", count)
	}
}

func TestIntegrationPayoutCancellationPreventsLateCompletionPosting(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildPayoutRailMux(db, func(context.Context, string, string, string) (map[string]any, error) {
		return map[string]any{"status": "processing"}, nil
	})
	merchantID, authHeader, sttl := createSettledMerchantFlow(t, ctx, db, merchantSvc, orderSvc, paymentSvc)

	req := httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/payout", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("initiate payout: expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode payout create response: %v", err)
	}
	payoutID, _ := created["id"].(string)
	if payoutID == "" {
		t.Fatal("expected payout id")
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/v1/payouts/"+payoutID+"/cancel", bytes.NewReader([]byte(`{"reason":"manual_abort_before_completion"}`)))
	cancelReq.Header.Set("Authorization", authHeader)
	cancelReq.Header.Set("Content-Type", "application/json")
	cancelRec := httptest.NewRecorder()
	mux.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("cancel payout: expected 200, got %d body=%s", cancelRec.Code, cancelRec.Body.String())
	}
	if !bytes.Contains(cancelRec.Body.Bytes(), []byte(`"status":"cancelled"`)) {
		t.Fatalf("expected cancelled payout response, got %s", cancelRec.Body.String())
	}

	completePayload := map[string]any{
		"event_id":       "rail_evt_cancelled_late_complete",
		"payout_id":      payoutID,
		"merchant_id":    merchantID,
		"status":         "completed",
		"rail_reference": "BNK_CANCELLED_LATE",
		"occurred_at":    time.Now().UTC().Format(time.RFC3339),
	}
	completeRec := doRailCallback(t, mux, completePayload, true)
	if completeRec.Code != http.StatusAccepted {
		t.Fatalf("late completion callback: expected 202, got %d body=%s", completeRec.Code, completeRec.Body.String())
	}

	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM paygate_payouts.payouts WHERE id = $1`, payoutID).Scan(&status); err != nil {
		t.Fatalf("query payout status: %v", err)
	}
	if status != "cancelled" {
		t.Fatalf("expected payout to remain cancelled, got %s", status)
	}

	var ledgerEntries int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM paygate_ledger.ledger_entries WHERE source_id = $1`, payoutID).Scan(&ledgerEntries); err != nil {
		t.Fatalf("count payout ledger entries: %v", err)
	}
	if ledgerEntries != 0 {
		t.Fatalf("expected no payout ledger postings after cancellation, got %d", ledgerEntries)
	}
}

func TestIntegrationWebhookRetryCancellationStopsReplayableFailures(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	createdMerchant, authHeader := createMerchantAndWriteKey(t, ctx, merchantSvc, "webhook-cancel@test.com")

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	createReq := httptest.NewRequest(http.MethodPost, "/v1/webhooks", bytes.NewReader([]byte(`{"url":"`+failingServer.URL+`","events":["payment.captured"]}`)))
	createReq.Header.Set("Authorization", authHeader)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create webhook: expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	webhookSvc := webhook.NewService(webhook.NewPostgresRepository(db))
	if err := webhookSvc.DeliverEvent(ctx, "evt_cancel_retry", createdMerchant.ID, "payment.captured", map[string]any{
		"event_type": "payment.captured",
		"payment_id": "pay_cancel_retry",
		"order_id":   "order_cancel_retry",
	}); err != nil {
		t.Fatalf("deliver event: %v", err)
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/v1/webhooks/events/evt_cancel_retry/cancel-retries", bytes.NewReader([]byte(`{"reason":"operator_cancelled_retry_queue"}`)))
	cancelReq.Header.Set("Authorization", authHeader)
	cancelReq.Header.Set("Content-Type", "application/json")
	cancelRec := httptest.NewRecorder()
	mux.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("cancel retries: expected 200, got %d body=%s", cancelRec.Code, cancelRec.Body.String())
	}

	var status string
	var nextRetryAt *time.Time
	var cancelReason string
	if err := db.QueryRow(ctx, `
SELECT status, next_retry_at, cancel_reason
FROM paygate_webhooks.webhook_delivery_attempts
WHERE merchant_id = $1 AND event_id = $2
LIMIT 1
`, createdMerchant.ID, "evt_cancel_retry").Scan(&status, &nextRetryAt, &cancelReason); err != nil {
		t.Fatalf("query delivery attempt: %v", err)
	}
	if status != string(webhook.DeliveryCancelled) {
		t.Fatalf("expected cancelled delivery status, got %s", status)
	}
	if nextRetryAt != nil {
		t.Fatalf("expected next_retry_at to be cleared after cancellation, got %v", nextRetryAt)
	}
	if cancelReason != "operator_cancelled_retry_queue" {
		t.Fatalf("expected cancel reason to persist, got %q", cancelReason)
	}

	processed, err := webhookSvc.RetryPendingDeliveries(ctx, 20)
	if err != nil {
		t.Fatalf("retry pending deliveries: %v", err)
	}
	if processed != 0 {
		t.Fatalf("expected no retry work after cancellation, got %d", processed)
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
