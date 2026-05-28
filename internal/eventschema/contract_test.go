package eventschema

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/webhook"
)

func TestWebhookConsumerAcceptsSchemaFixtureEnvelope(t *testing.T) {
	schema, sample, _ := loadFixturePair(t, filepath.Join("..", "..", "schemas", "events", "payment.captured", "1.1.0.schema.json"))
	if err := ValidateDocument(schema); err != nil {
		t.Fatalf("validate schema: %v", err)
	}

	delivered := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode delivered webhook body: %v", err)
		}
		delivered <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := &contractWebhookRepo{
		subscriptions: []webhook.WebhookSubscription{
			{
				ID:         "sub_1",
				MerchantID: "merch_456",
				URL:        server.URL,
				Events:     []string{"payment.captured"},
				Secret:     "contract-secret",
				Status:     webhook.StatusActive,
			},
		},
	}
	svc := webhook.NewService(repo)
	consumer := webhook.NewNamedConsumer(svc, noopKafkaConsumer{}, "webhook-contract")

	payloadMap, ok := sample["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload fixture map, got %#v", sample["payload"])
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		t.Fatalf("marshal sample payload: %v", err)
	}
	envelope, err := json.Marshal(map[string]any{
		"id":             "evt_payment_captured_contract",
		"event_id":       "evt_payment_captured_contract",
		"aggregate_type": "payment",
		"aggregate_id":   "pay_456",
		"event_type":     "payment.captured",
		"merchant_id":    "merch_456",
		"payload":        json.RawMessage(payload),
		"created_at":     time.Now().Unix(),
		"occurred_at":    time.Now().UTC().Format(time.RFC3339),
		"correlation_id": "pay_456",
		"causation_id":   "evt_payment_captured_contract",
		"schema_subject": "payment.captured",
		"schema_version": sample["schema_version"],
	})
	if err != nil {
		t.Fatalf("marshal fixture envelope: %v", err)
	}

	if err := consumer.HandleMessage(context.Background(), "paygate.payments", "merch_456", envelope); err != nil {
		t.Fatalf("handle schema fixture envelope: %v", err)
	}

	select {
	case body := <-delivered:
		if body["schema_subject"] != "payment.captured" {
			t.Fatalf("expected delivered schema_subject payment.captured, got %#v", body["schema_subject"])
		}
		if body["schema_version"] != "1.1.0" {
			t.Fatalf("expected delivered schema_version 1.1.0, got %#v", body["schema_version"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected webhook delivery from contract test")
	}
}

type noopKafkaConsumer struct{}

func (noopKafkaConsumer) Subscribe(context.Context, []string, func(topic, key string, payload []byte) error) error {
	return nil
}

type contractWebhookRepo struct {
	subscriptions []webhook.WebhookSubscription
	attempts      []webhook.WebhookDeliveryAttempt
}

func (r *contractWebhookRepo) CreateSubscription(context.Context, webhook.CreateInput) (webhook.WebhookSubscription, error) {
	panic("not used")
}
func (r *contractWebhookRepo) GetSubscription(ctx context.Context, merchantID, id string) (webhook.WebhookSubscription, error) {
	for _, sub := range r.subscriptions {
		if sub.MerchantID == merchantID && sub.ID == id {
			return sub, nil
		}
	}
	return webhook.WebhookSubscription{}, webhook.ErrSubscriptionNotFound
}
func (r *contractWebhookRepo) ListSubscriptions(context.Context, string) ([]webhook.WebhookSubscription, error) {
	return r.subscriptions, nil
}
func (r *contractWebhookRepo) UpdateSubscription(context.Context, string, string, webhook.UpdateInput) (webhook.WebhookSubscription, error) {
	panic("not used")
}
func (r *contractWebhookRepo) TransitionSubscription(context.Context, string, string, webhook.SubscriptionEvent) (webhook.WebhookSubscription, error) {
	panic("not used")
}
func (r *contractWebhookRepo) DeleteSubscription(context.Context, string, string) error {
	panic("not used")
}
func (r *contractWebhookRepo) FindActiveSubscriptions(_ context.Context, merchantID, eventType string) ([]webhook.WebhookSubscription, error) {
	var out []webhook.WebhookSubscription
	for _, sub := range r.subscriptions {
		if sub.MerchantID == merchantID && sub.MatchesEvent(eventType) {
			out = append(out, sub)
		}
	}
	return out, nil
}
func (r *contractWebhookRepo) CreateDeliveryAttempt(_ context.Context, attempt webhook.WebhookDeliveryAttempt) (webhook.WebhookDeliveryAttempt, error) {
	r.attempts = append(r.attempts, attempt)
	return attempt, nil
}
func (r *contractWebhookRepo) ReserveDeliveryAttempt(_ context.Context, attempt webhook.WebhookDeliveryAttempt) (webhook.WebhookDeliveryAttempt, error) {
	for _, existing := range r.attempts {
		if existing.EventID == attempt.EventID && existing.SubscriptionID == attempt.SubscriptionID {
			return webhook.WebhookDeliveryAttempt{}, webhook.ErrDeliveryAlreadyReserved
		}
	}
	r.attempts = append(r.attempts, attempt)
	return attempt, nil
}
func (r *contractWebhookRepo) UpdateDeliveryAttempt(_ context.Context, id string, status webhook.DeliveryStatus, responseCode int, responseBody, errorMessage string, _ *string, attemptNumber int) (webhook.WebhookDeliveryAttempt, error) {
	for i := range r.attempts {
		if r.attempts[i].ID != id {
			continue
		}
		r.attempts[i].Status = status
		r.attempts[i].ResponseCode = responseCode
		r.attempts[i].ResponseBody = responseBody
		r.attempts[i].ErrorMessage = errorMessage
		r.attempts[i].AttemptNumber = attemptNumber
		return r.attempts[i], nil
	}
	return webhook.WebhookDeliveryAttempt{}, webhook.ErrDeliveryAttemptNotFound
}
func (r *contractWebhookRepo) ListDeliveryAttempts(context.Context, string, string) ([]webhook.WebhookDeliveryAttempt, error) {
	return r.attempts, nil
}
func (r *contractWebhookRepo) GetDeliveryAttempt(context.Context, string) (webhook.WebhookDeliveryAttempt, error) {
	panic("not used")
}
func (r *contractWebhookRepo) LeasePendingRetries(context.Context, int) ([]webhook.WebhookDeliveryAttempt, error) {
	return nil, nil
}
func (r *contractWebhookRepo) IsDelivered(_ context.Context, eventID, subscriptionID string) (bool, error) {
	for _, attempt := range r.attempts {
		if attempt.EventID == eventID && attempt.SubscriptionID == subscriptionID && attempt.Status == webhook.DeliverySucceeded {
			return true, nil
		}
	}
	return false, nil
}
func (r *contractWebhookRepo) FindDeliveryByEvent(context.Context, string, string) ([]webhook.WebhookDeliveryAttempt, error) {
	return r.attempts, nil
}
func (r *contractWebhookRepo) CancelDeliveryRetries(context.Context, string, string, string) (int64, error) {
	return 0, nil
}
func (r *contractWebhookRepo) RotateSecret(context.Context, string, string) (webhook.WebhookSubscription, error) {
	panic("not used")
}
