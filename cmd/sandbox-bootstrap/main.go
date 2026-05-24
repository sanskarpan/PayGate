package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	baseURL := flag.String("base-url", "http://127.0.0.1:8090", "PayGate API base URL")
	mode := flag.String("mode", "test", "API key mode")
	scope := flag.String("scope", "admin", "API key scope")
	suffix := flag.String("suffix", fmt.Sprintf("%d", time.Now().Unix()), "suffix for generated resources")
	seedDemo := flag.Bool("seed-demo", true, "create sample dashboard user, webhook, order, and payment")
	flag.Parse()

	merchantPayload := map[string]any{
		"name":          "Sandbox Merchant " + *suffix,
		"email":         "merchant+" + *suffix + "@sandbox.paygate.local",
		"business_type": "company",
		"settings": map[string]any{
			"suite": "sandbox_bootstrap_cli",
		},
	}

	merchantResp := createJSON(*baseURL+"/v1/merchants", merchantPayload)
	merchantID, _ := merchantResp["id"].(string)
	if merchantID == "" {
		fatalf("merchant response missing id")
	}

	keyResp := createJSON(*baseURL+"/v1/merchants/"+merchantID+"/keys", map[string]any{
		"mode":  *mode,
		"scope": *scope,
	})
	keyID, _ := keyResp["key_id"].(string)
	keySecret, _ := keyResp["key_secret"].(string)
	if keyID == "" || keySecret == "" {
		fatalf("key response missing credentials")
	}

	authB64 := base64.StdEncoding.EncodeToString([]byte(keyID + ":" + keySecret))
	fmt.Printf("MERCHANT_ID=%q\n", merchantID)
	fmt.Printf("API_KEY=%q\n", keyID)
	fmt.Printf("API_SECRET=%q\n", keySecret)
	fmt.Printf("AUTH_B64=%q\n", authB64)
	fmt.Printf("AUTH_HEADER=%q\n", "Basic "+authB64)

	if !*seedDemo {
		return
	}

	authHeader := "Basic " + authB64
	userResp := createAuthorizedJSON(*baseURL+"/v1/merchants/"+merchantID+"/users", authHeader, map[string]any{
		"email":    "operator+" + *suffix + "@sandbox.paygate.local",
		"role":     "owner",
		"password": "PaygateSandbox!123",
	})
	orderResp := createAuthorizedJSON(*baseURL+"/v1/orders", authHeader, map[string]any{
		"amount":   4200,
		"currency": "INR",
		"receipt":  "sandbox-order-" + *suffix,
	})
	orderID, _ := orderResp["id"].(string)
	paymentResp := createAuthorizedJSON(*baseURL+"/v1/payments/authorize", authHeader, map[string]any{
		"order_id":                orderID,
		"method":                  "card",
		"payment_method_token_id": "tok_sandbox_single_use",
		"auto_capture":            true,
	})
	webhookResp := createAuthorizedJSON(*baseURL+"/v1/webhooks", authHeader, map[string]any{
		"url":    "https://merchant.example.com/webhooks/paygate",
		"events": []string{"payment.captured", "refund.processed"},
	})

	fmt.Printf("DASHBOARD_EMAIL=%q\n", userResp["email"])
	fmt.Printf("DASHBOARD_PASSWORD=%q\n", "PaygateSandbox!123")
	fmt.Printf("ORDER_ID=%q\n", orderResp["id"])
	fmt.Printf("PAYMENT_ID=%q\n", paymentResp["id"])
	fmt.Printf("WEBHOOK_ID=%q\n", webhookResp["id"])
}

func createJSON(url string, payload map[string]any) map[string]any {
	return createRequestJSON(http.MethodPost, url, "", payload)
}

func createAuthorizedJSON(url, authHeader string, payload map[string]any) map[string]any {
	return createRequestJSON(http.MethodPost, url, authHeader, payload)
}

func createRequestJSON(method, url, authHeader string, payload map[string]any) map[string]any {
	body, err := json.Marshal(payload)
	if err != nil {
		fatalf("marshal request: %v", err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fatalf("request %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fatalf("request %s failed: %s", url, string(respBody))
	}
	var out map[string]any
	if err := json.Unmarshal(respBody, &out); err != nil {
		fatalf("decode response: %v", err)
	}
	return out
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
