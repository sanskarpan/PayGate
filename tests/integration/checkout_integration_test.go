//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sanskarpan/PayGate/internal/gateway"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
)

func TestIntegrationCheckoutFlow(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	_, _ = db.Exec(ctx, `INSERT INTO paygate_merchants.merchants(id,name,email,business_type,status,settings) VALUES('merch_int_3','M3','m3@test.com','company','active','{}') ON CONFLICT (id) DO NOTHING`)
	orderSvc := order.NewService(order.NewPostgresRepository(db))
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	paySvc := payment.NewService(payment.NewPostgresRepository(db, ledgerSvc, orderSvc), gateway.NewSimulator())
	checkout := gateway.NewCheckoutHandler(paySvc, orderSvc)

	o, err := orderSvc.Create(ctx, order.CreateInput{MerchantID: "merch_int_3", Amount: 12345, Currency: "INR"})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	mux := http.NewServeMux()
	checkout.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	form := url.Values{}
	form.Set("order_id", o.ID)
	form.Set("merchant_id", "merch_int_3")
	form.Set("method", "card")
	form.Set("callback_url", server.URL+"/checkout/callback")

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/checkout/pay", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("checkout post: %v", err)
	}
	if res.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect, got %d", res.StatusCode)
	}
	location := res.Header.Get("Location")
	if !strings.Contains(location, "payment_id=") {
		t.Fatalf("expected payment_id in callback url, got %s", location)
	}
	if !strings.Contains(location, fmt.Sprintf("status=%s", payment.StateAuthorized)) {
		t.Fatalf("expected authorized status in callback url, got %s", location)
	}
}

func TestIntegrationCheckoutIgnoresTamperedAmount(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	_, _ = db.Exec(ctx, `INSERT INTO paygate_merchants.merchants(id,name,email,business_type,status,settings) VALUES('merch_int_4','M4','m4@test.com','company','active','{}') ON CONFLICT (id) DO NOTHING`)
	orderSvc := order.NewService(order.NewPostgresRepository(db))
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	paySvc := payment.NewService(payment.NewPostgresRepository(db, ledgerSvc, orderSvc), gateway.NewSimulator())
	checkout := gateway.NewCheckoutHandler(paySvc, orderSvc)

	o, err := orderSvc.Create(ctx, order.CreateInput{MerchantID: "merch_int_4", Amount: 12345, Currency: "INR"})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	mux := http.NewServeMux()
	checkout.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	form := url.Values{}
	form.Set("order_id", o.ID)
	form.Set("merchant_id", "merch_int_4")
	form.Set("amount", "1")
	form.Set("currency", "USD")
	form.Set("method", "card")
	form.Set("callback_url", server.URL+"/checkout/callback")

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/checkout/pay", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("checkout post: %v", err)
	}
	if res.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect, got %d", res.StatusCode)
	}

	var paymentAmount int64
	err = db.QueryRow(ctx, `SELECT amount FROM paygate_payments.payments WHERE order_id = $1`, o.ID).Scan(&paymentAmount)
	if err != nil {
		t.Fatalf("query payment amount: %v", err)
	}
	if paymentAmount != o.Amount {
		t.Fatalf("expected payment amount %d, got %d", o.Amount, paymentAmount)
	}
}
