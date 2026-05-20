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

	"github.com/sanskarpan/PayGate/internal/eventschema"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/outbox"
)

func TestIntegrationEventSchemaRegistryAndDualPublish(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `DELETE FROM public.outbox WHERE published_at IS NULL`); err != nil {
		t.Fatalf("clear unpublished outbox rows: %v", err)
	}
	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name:         "Schema Merchant",
		Email:        "schema@test.com",
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	adminKey, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeTest,
		Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create admin api key: %v", err)
	}

	doJSON := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			payload, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal request body: %v", err)
			}
			reader = bytes.NewReader(payload)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("Authorization", basicAuth(adminKey.KeyID, adminKey.KeySecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	createSchema := doJSON(http.MethodPost, "/v1/event-schemas", map[string]any{
		"subject":     "payment.captured",
		"event_type":  "payment.captured",
		"topic_name":  "paygate.payments",
		"owner":       "payments-platform",
		"review_link": "https://example.test/reviews/payment-captured",
	})
	if createSchema.Code != http.StatusCreated {
		t.Fatalf("expected create schema 201, got %d body=%s", createSchema.Code, createSchema.Body.String())
	}

	baseVersion := doJSON(http.MethodPost, "/v1/event-schemas/payment.captured/versions", map[string]any{
		"version":        "1.0.0",
		"schema":         paymentCapturedSchema(false),
		"sample_payload": paymentCapturedSample(false),
		"review_link":    "https://example.test/reviews/payment-captured-v1",
	})
	if baseVersion.Code != http.StatusCreated {
		t.Fatalf("expected base schema version 201, got %d body=%s", baseVersion.Code, baseVersion.Body.String())
	}

	activateBase := doJSON(http.MethodPost, "/v1/event-schemas/payment.captured/versions/1.0.0/activate", nil)
	if activateBase.Code != http.StatusAccepted {
		t.Fatalf("expected activate base version 202, got %d body=%s", activateBase.Code, activateBase.Body.String())
	}

	incompatible := doJSON(http.MethodPost, "/v1/event-schemas/payment.captured/versions", map[string]any{
		"version":        "2.0.0",
		"schema":         paymentCapturedIncompatibleSchema(),
		"sample_payload": paymentCapturedSample(false),
		"review_link":    "https://example.test/reviews/payment-captured-v2",
	})
	if incompatible.Code != http.StatusConflict {
		t.Fatalf("expected incompatible schema 409, got %d body=%s", incompatible.Code, incompatible.Body.String())
	}

	additive := doJSON(http.MethodPost, "/v1/event-schemas/payment.captured/versions", map[string]any{
		"version":        "1.1.0",
		"schema":         paymentCapturedSchema(true),
		"sample_payload": paymentCapturedSample(true),
		"review_link":    "https://example.test/reviews/payment-captured-v1-1",
	})
	if additive.Code != http.StatusCreated {
		t.Fatalf("expected additive schema version 201, got %d body=%s", additive.Code, additive.Body.String())
	}

	compare := doJSON(http.MethodGet, "/v1/event-schemas/payment.captured/compare?from=1.0.0&to=1.1.0", nil)
	if compare.Code != http.StatusOK {
		t.Fatalf("expected compare 200, got %d body=%s", compare.Code, compare.Body.String())
	}

	rollout := doJSON(http.MethodPost, "/v1/event-schemas/payment.captured/rollouts", map[string]any{
		"from_version": "1.0.0",
		"to_version":   "1.1.0",
		"notes":        "dual publish for webhook-service and analytics",
	})
	if rollout.Code != http.StatusCreated {
		t.Fatalf("expected rollout create 201, got %d body=%s", rollout.Code, rollout.Body.String())
	}
	var rolloutResp map[string]any
	if err := json.Unmarshal(rollout.Body.Bytes(), &rolloutResp); err != nil {
		t.Fatalf("decode rollout response: %v", err)
	}
	rolloutID, _ := rolloutResp["id"].(string)
	if rolloutID == "" {
		t.Fatalf("expected rollout id in response: %#v", rolloutResp)
	}

	ack := doJSON(http.MethodPost, "/v1/event-schema-rollouts/"+rolloutID+"/ack", map[string]any{
		"consumer_name":        "webhook-service",
		"acknowledged_version": "1.1.0",
	})
	if ack.Code != http.StatusCreated {
		t.Fatalf("expected rollout ack 201, got %d body=%s", ack.Code, ack.Body.String())
	}

	if _, err := db.Exec(ctx, `
INSERT INTO public.outbox (id, aggregate_type, aggregate_id, event_type, merchant_id, payload)
VALUES ('evt_schema_dual_publish', 'payment', 'pay_schema_test', 'payment.captured', $1, '{"payment_id":"pay_schema_test","order_id":"order_schema_test"}')
ON CONFLICT (id) DO UPDATE SET published_at = NULL, payload = EXCLUDED.payload
`, m.ID); err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}

	schemaSvc := eventschema.NewService(eventschema.NewPostgresRepository(db), nil)
	if err := schemaSvc.BootstrapFromFixtures(ctx, "../../schemas/events", "schema-bootstrap"); err != nil {
		t.Fatalf("bootstrap schema fixtures: %v", err)
	}
	publisher := &fakePublisher{}
	relay := outbox.NewRelay(db, publisher, 0, nil).WithSchemaVersionResolver(schemaSvc)
	published, err := relay.PublishBatch(ctx, 10)
	if err != nil {
		t.Fatalf("publish outbox batch with schema registry: %v", err)
	}
	if published != 1 {
		t.Fatalf("expected one outbox row to publish, got %d", published)
	}
	if len(publisher.body) != 2 {
		t.Fatalf("expected dual publish to emit 2 envelopes, got %d", len(publisher.body))
	}

	seenVersions := map[string]bool{}
	for _, body := range publisher.body {
		var envelope map[string]any
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Fatalf("decode published envelope: %v", err)
		}
		if envelope["schema_subject"] != "payment.captured" {
			t.Fatalf("expected schema_subject payment.captured, got %#v", envelope["schema_subject"])
		}
		version, _ := envelope["schema_version"].(string)
		seenVersions[version] = true
		if envelope["event_id"] != "evt_schema_dual_publish" {
			t.Fatalf("expected event_id to match outbox row, got %#v", envelope["event_id"])
		}
		if envelope["occurred_at"] == "" || envelope["correlation_id"] == "" || envelope["causation_id"] == "" {
			t.Fatalf("expected envelope metadata to be populated, got %#v", envelope)
		}
	}
	if !seenVersions["1.0.0"] || !seenVersions["1.1.0"] {
		t.Fatalf("expected published schema versions 1.0.0 and 1.1.0, got %#v", seenVersions)
	}
}

func TestIntegrationEventSchemaDeprecatedVersionAlerts(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()
	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name:         "Schema Alert Merchant",
		Email:        "schema-alert@test.com",
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	adminKey, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeTest,
		Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create admin api key: %v", err)
	}

	doJSON := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			payload, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal request body: %v", err)
			}
			reader = bytes.NewReader(payload)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("Authorization", basicAuth(adminKey.KeyID, adminKey.KeySecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := doJSON(http.MethodPost, "/v1/event-schemas/payout.completed/versions/1.0.0/activate", nil); rec.Code == http.StatusAccepted {
		// Schema may already exist from previous tests; activation is fine if it does.
	}
	createSchema := doJSON(http.MethodPost, "/v1/event-schemas", map[string]any{
		"subject":     "payout.completed",
		"event_type":  "payout.completed",
		"topic_name":  "paygate.payouts",
		"owner":       "payouts-platform",
		"review_link": "https://example.test/reviews/payout-completed",
	})
	if createSchema.Code != http.StatusCreated && createSchema.Code != http.StatusConflict {
		t.Fatalf("expected create schema 201/409, got %d body=%s", createSchema.Code, createSchema.Body.String())
	}
	baseVersion := doJSON(http.MethodPost, "/v1/event-schemas/payout.completed/versions", map[string]any{
		"version":        "1.0.0",
		"schema":         payoutCompletedSchema(false),
		"sample_payload": payoutCompletedSample(false),
		"review_link":    "https://example.test/reviews/payout-completed-v1",
	})
	if baseVersion.Code != http.StatusCreated && baseVersion.Code != http.StatusConflict {
		t.Fatalf("expected base version 201/409, got %d body=%s", baseVersion.Code, baseVersion.Body.String())
	}
	if rec := doJSON(http.MethodPost, "/v1/event-schemas/payout.completed/versions/1.0.0/activate", nil); rec.Code != http.StatusAccepted {
		t.Fatalf("activate base version: expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	additive := doJSON(http.MethodPost, "/v1/event-schemas/payout.completed/versions", map[string]any{
		"version":        "1.1.0",
		"schema":         payoutCompletedSchema(true),
		"sample_payload": payoutCompletedSample(true),
		"review_link":    "https://example.test/reviews/payout-completed-v1-1",
	})
	if additive.Code != http.StatusCreated && additive.Code != http.StatusConflict {
		t.Fatalf("expected additive version 201/409, got %d body=%s", additive.Code, additive.Body.String())
	}

	deadline := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	rollout := doJSON(http.MethodPost, "/v1/event-schemas/payout.completed/rollouts", map[string]any{
		"from_version":     "1.0.0",
		"to_version":       "1.1.0",
		"cutover_deadline": deadline,
		"notes":            "deliberately overdue",
	})
	if rollout.Code != http.StatusCreated {
		t.Fatalf("expected rollout create 201, got %d body=%s", rollout.Code, rollout.Body.String())
	}
	var rolloutResp map[string]any
	if err := json.Unmarshal(rollout.Body.Bytes(), &rolloutResp); err != nil {
		t.Fatalf("decode rollout response: %v", err)
	}
	rolloutID, _ := rolloutResp["id"].(string)
	if rolloutID == "" {
		t.Fatalf("expected rollout id, got %#v", rolloutResp)
	}

	ack := doJSON(http.MethodPost, "/v1/event-schema-rollouts/"+rolloutID+"/ack", map[string]any{
		"consumer_name":        "analytics-service",
		"acknowledged_version": "1.0.0",
	})
	if ack.Code != http.StatusCreated {
		t.Fatalf("expected rollout ack 201, got %d body=%s", ack.Code, ack.Body.String())
	}

	svc := eventschema.NewService(eventschema.NewPostgresRepository(db), nil)
	alerts, err := svc.ListDeprecatedVersionAlerts(ctx)
	if err != nil {
		t.Fatalf("list deprecated version alerts: %v", err)
	}
	found := false
	for _, alert := range alerts {
		if alert.RolloutID == rolloutID && alert.ConsumerName == "analytics-service" && alert.AcknowledgedVersion == "1.0.0" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected overdue deprecated-version alert for rollout %s, got %#v", rolloutID, alerts)
	}
}

func paymentCapturedSchema(withOptionalRisk bool) map[string]any {
	payloadProps := map[string]any{
		"payment_id": map[string]any{"type": "string"},
		"order_id":   map[string]any{"type": "string"},
	}
	if withOptionalRisk {
		payloadProps["risk_bucket"] = map[string]any{"type": "string"}
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"event_id":       map[string]any{"type": "string"},
			"event_type":     map[string]any{"type": "string"},
			"occurred_at":    map[string]any{"type": "string"},
			"correlation_id": map[string]any{"type": "string"},
			"causation_id":   map[string]any{"type": "string"},
			"schema_version": map[string]any{"type": "string"},
			"merchant_id":    map[string]any{"type": "string"},
			"payload": map[string]any{
				"type":       "object",
				"properties": payloadProps,
				"required":   []string{"payment_id", "order_id"},
			},
		},
		"required": []string{"event_id", "event_type", "occurred_at", "correlation_id", "causation_id", "schema_version", "merchant_id", "payload"},
	}
}

func paymentCapturedIncompatibleSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"event_id":       map[string]any{"type": "string"},
			"event_type":     map[string]any{"type": "string"},
			"occurred_at":    map[string]any{"type": "string"},
			"correlation_id": map[string]any{"type": "string"},
			"causation_id":   map[string]any{"type": "string"},
			"schema_version": map[string]any{"type": "string"},
			"merchant_id":    map[string]any{"type": "string"},
			"payload": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"payment_id": map[string]any{"type": "string"},
				},
				"required": []string{"payment_id"},
			},
		},
		"required": []string{"event_id", "event_type", "occurred_at", "correlation_id", "causation_id", "schema_version", "merchant_id", "payload"},
	}
}

func paymentCapturedSample(withOptionalRisk bool) map[string]any {
	sample := map[string]any{
		"event_id":       "evt_sample_payment_captured",
		"event_type":     "payment.captured",
		"occurred_at":    "2026-05-12T10:00:00Z",
		"correlation_id": "pay_schema_test",
		"causation_id":   "evt_sample_payment_captured",
		"schema_version": "1.0.0",
		"merchant_id":    "merch_schema_test",
		"payload": map[string]any{
			"payment_id": "pay_schema_test",
			"order_id":   "order_schema_test",
		},
	}
	if withOptionalRisk {
		sample["schema_version"] = "1.1.0"
		payload := sample["payload"].(map[string]any)
		payload["risk_bucket"] = "normal"
	}
	return sample
}

func payoutCompletedSchema(withOptionalMetadata bool) map[string]any {
	payloadProps := map[string]any{
		"payout_id":      map[string]any{"type": "string"},
		"bank_reference": map[string]any{"type": "string"},
	}
	if withOptionalMetadata {
		payloadProps["rail_reference"] = map[string]any{"type": "string"}
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"event_id":       map[string]any{"type": "string"},
			"event_type":     map[string]any{"type": "string"},
			"occurred_at":    map[string]any{"type": "string"},
			"correlation_id": map[string]any{"type": "string"},
			"causation_id":   map[string]any{"type": "string"},
			"schema_version": map[string]any{"type": "string"},
			"merchant_id":    map[string]any{"type": "string"},
			"payload": map[string]any{
				"type":       "object",
				"properties": payloadProps,
				"required":   []string{"payout_id", "bank_reference"},
			},
		},
		"required": []string{"event_id", "event_type", "occurred_at", "correlation_id", "causation_id", "schema_version", "merchant_id", "payload"},
	}
}

func payoutCompletedSample(withOptionalMetadata bool) map[string]any {
	sample := map[string]any{
		"event_id":       "evt_sample_payout_completed",
		"event_type":     "payout.completed",
		"occurred_at":    "2026-05-13T10:00:00Z",
		"correlation_id": "payout_123",
		"causation_id":   "evt_sample_payout_completed",
		"schema_version": "1.0.0",
		"merchant_id":    "merch_payout_schema_test",
		"payload": map[string]any{
			"payout_id":      "payout_123",
			"bank_reference": "BNK_123",
		},
	}
	if withOptionalMetadata {
		sample["schema_version"] = "1.1.0"
		payload := sample["payload"].(map[string]any)
		payload["rail_reference"] = "RAIL_123"
	}
	return sample
}
