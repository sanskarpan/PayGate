//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/merchant"
)

func TestIntegrationRetentionRunAndLegalHoldLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name:         "Retention Merchant",
		Email:        uniqueTestEmail(t, "retention"),
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeTest,
		Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create admin key: %v", err)
	}

	var applicationID string
	if err := db.QueryRow(ctx, `SELECT id FROM paygate_merchants.merchant_onboarding_applications WHERE merchant_id = $1`, createdMerchant.ID).Scan(&applicationID); err != nil {
		t.Fatalf("query onboarding application: %v", err)
	}

	old := time.Now().UTC().Add(-120 * 24 * time.Hour)
	if _, err := db.Exec(ctx, `
INSERT INTO paygate_reporting.export_jobs
    (id, merchant_id, report_type, format, status, file_name, content_type, file_size_bytes, content_text, download_token, download_expires_at, created_at, completed_at)
VALUES
    ($1,$2,'payments','csv','completed','payments.csv','text/csv',12,'id,amount\npay_1,4200','tok_retention',NOW()+INTERVAL '24 hours',$3,$3)
`, "exp_retention_1", createdMerchant.ID, old); err != nil {
		t.Fatalf("insert export job: %v", err)
	}

	if _, err := db.Exec(ctx, `
INSERT INTO paygate_webhooks.webhook_subscriptions
    (id, merchant_id, url, events, secret, status, created_at, updated_at)
VALUES
    ($1,$2,'https://merchant.example.com/hook',ARRAY['payment.captured'],'secret','active',$3,$3)
ON CONFLICT (id) DO NOTHING
`, "whsub_retention_1", createdMerchant.ID, old); err != nil {
		t.Fatalf("insert webhook subscription: %v", err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO paygate_webhooks.webhook_delivery_attempts
    (id, event_id, subscription_id, merchant_id, status, request_url, request_body, response_code, response_body, error_message, attempt_number, created_at)
VALUES
    ($1,'evt_retention_1',$2,$3,'failed','https://merchant.example.com/hook',$4,500,'timeout','network timeout',2,$5)
`, "whattempt_retention_1", "whsub_retention_1", createdMerchant.ID, []byte(`{"id":"evt_retention_1"}`), old); err != nil {
		t.Fatalf("insert webhook attempt: %v", err)
	}

	if _, err := db.Exec(ctx, `
INSERT INTO paygate_merchants.merchant_onboarding_documents
    (id, application_id, merchant_id, document_type, file_name, content_type, storage_key, status, requested_at, uploaded_at, created_at, updated_at)
VALUES
    ($1,$2,$3,'kyb_certificate','kyb.pdf','application/pdf','bucket/secret/path.pdf','approved',$4,$4,$4,$4)
`, "doc_retention_1", applicationID, createdMerchant.ID, old); err != nil {
		t.Fatalf("insert onboarding document: %v", err)
	}

	postJSON := func(path string, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(payload)))
		req.Header.Set("Authorization", basicAuth(key.KeyID, key.KeySecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	exportRun := postJSON("/v1/retention/run", `{"artifact_class":"report_export","actor_type":"ops","actor_id":"retention-test"}`)
	if exportRun.Code != http.StatusAccepted {
		t.Fatalf("run export retention: %d body=%s", exportRun.Code, exportRun.Body.String())
	}

	var exportContent string
	var exportStatus string
	if err := db.QueryRow(ctx, `SELECT content_text, retention_status FROM paygate_reporting.export_jobs WHERE id = $1`, "exp_retention_1").Scan(&exportContent, &exportStatus); err != nil {
		t.Fatalf("query export after retention: %v", err)
	}
	if exportContent != "" || exportStatus != "redacted" {
		t.Fatalf("expected export to be redacted, got content=%q status=%s", exportContent, exportStatus)
	}

	createHold := postJSON("/v1/retention/holds", `{"artifact_class":"webhook_delivery_attempt","merchant_id":"`+createdMerchant.ID+`","artifact_id":"whattempt_retention_1","reason":"investigation","created_by":"ops@test"}`)
	if createHold.Code != http.StatusCreated {
		t.Fatalf("create legal hold: %d body=%s", createHold.Code, createHold.Body.String())
	}
	var holdResp map[string]any
	if err := json.Unmarshal(createHold.Body.Bytes(), &holdResp); err != nil {
		t.Fatalf("decode hold response: %v", err)
	}
	holdID, _ := holdResp["id"].(string)
	if holdID == "" {
		t.Fatal("expected hold id")
	}

	webhookRunHeld := postJSON("/v1/retention/run", `{"artifact_class":"webhook_delivery_attempt"}`)
	if webhookRunHeld.Code != http.StatusAccepted {
		t.Fatalf("run webhook retention with hold: %d body=%s", webhookRunHeld.Code, webhookRunHeld.Body.String())
	}

	var heldReqBody []byte
	var heldStatus string
	if err := db.QueryRow(ctx, `SELECT request_body, retention_status FROM paygate_webhooks.webhook_delivery_attempts WHERE id = $1`, "whattempt_retention_1").Scan(&heldReqBody, &heldStatus); err != nil {
		t.Fatalf("query held webhook attempt: %v", err)
	}
	if string(heldReqBody) == "" || heldStatus != "active" {
		t.Fatalf("expected held webhook attempt to remain active, got request_body=%q status=%s", string(heldReqBody), heldStatus)
	}

	releaseReq := httptest.NewRequest(http.MethodPost, "/v1/retention/holds/"+holdID+"/release", bytes.NewReader([]byte(`{}`)))
	releaseReq.Header.Set("Authorization", basicAuth(key.KeyID, key.KeySecret))
	releaseRec := httptest.NewRecorder()
	mux.ServeHTTP(releaseRec, releaseReq)
	if releaseRec.Code != http.StatusOK {
		t.Fatalf("release legal hold: %d body=%s", releaseRec.Code, releaseRec.Body.String())
	}

	webhookRun := postJSON("/v1/retention/run", `{"artifact_class":"webhook_delivery_attempt"}`)
	if webhookRun.Code != http.StatusAccepted {
		t.Fatalf("run webhook retention: %d body=%s", webhookRun.Code, webhookRun.Body.String())
	}

	var requestBody []byte
	var responseBody *string
	var errorMessage string
	if err := db.QueryRow(ctx, `
SELECT request_body, response_body, error_message, retention_status
FROM paygate_webhooks.webhook_delivery_attempts
WHERE id = $1`, "whattempt_retention_1").Scan(&requestBody, &responseBody, &errorMessage, &heldStatus); err != nil {
		t.Fatalf("query webhook attempt after retention: %v", err)
	}
	if requestBody != nil || responseBody != nil || errorMessage != "[redacted]" || heldStatus != "redacted" {
		t.Fatalf("expected webhook payload redaction, got request_body=%v response_body=%v error=%q status=%s", requestBody, responseBody, errorMessage, heldStatus)
	}

	docRun := postJSON("/v1/retention/run", `{"artifact_class":"onboarding_document"}`)
	if docRun.Code != http.StatusAccepted {
		t.Fatalf("run onboarding retention: %d body=%s", docRun.Code, docRun.Body.String())
	}

	var storageKey string
	var docStatus string
	if err := db.QueryRow(ctx, `SELECT storage_key, retention_status FROM paygate_merchants.merchant_onboarding_documents WHERE id = $1`, "doc_retention_1").Scan(&storageKey, &docStatus); err != nil {
		t.Fatalf("query onboarding document after retention: %v", err)
	}
	if storageKey != "" || docStatus != "redacted" {
		t.Fatalf("expected onboarding document locator redaction, got storage_key=%q status=%s", storageKey, docStatus)
	}
}
