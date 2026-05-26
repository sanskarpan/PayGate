package paygate

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

const Version = "0.1.0"

type Client struct {
	baseURL    string
	httpClient *http.Client
	authHeader string
}

func New(baseURL, keyID, keySecret string) *Client {
	auth := base64.StdEncoding.EncodeToString([]byte(keyID + ":" + keySecret))
	return &Client{
		baseURL:    baseURL,
		httpClient: http.DefaultClient,
		authHeader: "Basic " + auth,
	}
}

type Order struct {
	ID       string `json:"id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Receipt  string `json:"receipt"`
	Status   string `json:"status"`
}

type Payment struct {
	ID                   string `json:"id"`
	OrderID              string `json:"order_id"`
	Amount               int64  `json:"amount"`
	Currency             string `json:"currency"`
	Method               string `json:"method"`
	Status               string `json:"status"`
	PaymentMethodTokenID string `json:"payment_method_token_id"`
}

type Refund struct {
	ID        string `json:"id"`
	PaymentID string `json:"payment_id"`
	Amount    int64  `json:"amount"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
}

type CardToken struct {
	ID               string `json:"id"`
	Brand            string `json:"brand"`
	Last4            string `json:"last4"`
	PaymentTokenType string `json:"token_type"`
	Reusable         bool   `json:"reusable"`
}

type WebhookSubscription struct {
	ID            string   `json:"id"`
	URL           string   `json:"url"`
	Events        []string `json:"events"`
	Status        string   `json:"status"`
	SignatureMode string   `json:"signature_mode"`
	//nolint:gosec // webhook API returns a signing secret by design.
	Secret        string   `json:"secret"`
}

func (c *Client) CreateOrder(ctx context.Context, payload map[string]any, idempotencyKey string) (Order, error) {
	var out Order
	if err := c.doJSON(ctx, http.MethodPost, "/v1/orders", payload, idempotencyKey, &out); err != nil {
		return Order{}, err
	}
	return out, nil
}

func (c *Client) CreatePayment(ctx context.Context, payload map[string]any, idempotencyKey string) (Payment, error) {
	var out Payment
	if err := c.doJSON(ctx, http.MethodPost, "/v1/payments/authorize", payload, idempotencyKey, &out); err != nil {
		return Payment{}, err
	}
	return out, nil
}

func (c *Client) CreateCardToken(ctx context.Context, payload map[string]any) (CardToken, error) {
	var out CardToken
	if err := c.doJSON(ctx, http.MethodPost, "/v1/card-tokens", payload, "", &out); err != nil {
		return CardToken{}, err
	}
	return out, nil
}

func (c *Client) CreateWebhookSubscription(ctx context.Context, payload map[string]any) (WebhookSubscription, error) {
	var out WebhookSubscription
	if err := c.doJSON(ctx, http.MethodPost, "/v1/webhooks", payload, "", &out); err != nil {
		return WebhookSubscription{}, err
	}
	return out, nil
}

func (c *Client) CapturePayment(ctx context.Context, paymentID string, payload map[string]any, idempotencyKey string) (Payment, error) {
	var out Payment
	if err := c.doJSON(ctx, http.MethodPost, "/v1/payments/"+paymentID+"/capture", payload, idempotencyKey, &out); err != nil {
		return Payment{}, err
	}
	return out, nil
}

func (c *Client) CreateRefund(ctx context.Context, paymentID string, payload map[string]any, idempotencyKey string) (Refund, error) {
	var out Refund
	if err := c.doJSON(ctx, http.MethodPost, "/v1/payments/"+paymentID+"/refunds", payload, idempotencyKey, &out); err != nil {
		return Refund{}, err
	}
	return out, nil
}

func VerifyWebhookSignature(secret string, body []byte, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, idempotencyKey string, out any) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
