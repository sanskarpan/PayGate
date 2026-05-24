package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/sanskarpan/PayGate/sdk/go/paygate"
)

func main() {
	baseURL := envOr("PAYGATE_BASE_URL", "http://127.0.0.1:8090")
	keyID := os.Getenv("PAYGATE_KEY_ID")
	keySecret := os.Getenv("PAYGATE_KEY_SECRET")
	if keyID == "" || keySecret == "" {
		log.Fatal("set PAYGATE_KEY_ID and PAYGATE_KEY_SECRET")
	}

	client := paygate.New(baseURL, keyID, keySecret)
	order, err := client.CreateOrder(context.Background(), map[string]any{
		"amount":   4200,
		"currency": "INR",
		"receipt":  "example-server-only-go",
	}, "example-server-only-go-order")
	if err != nil {
		log.Fatal(err)
	}

	payment, err := client.CreatePayment(context.Background(), map[string]any{
		"order_id":                order.ID,
		"method":                  "card",
		"payment_method_token_id": "tok_sandbox_single_use",
		"auto_capture":            true,
	}, "example-server-only-go-payment")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("order_id=%s payment_id=%s status=%s\n", order.ID, payment.ID, payment.Status)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
