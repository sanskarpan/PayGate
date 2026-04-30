//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/sanskarpan/PayGate/internal/order"
)

func TestIntegrationCreateOrderVerifyDB(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	_, _ = db.Exec(context.Background(), `INSERT INTO paygate_merchants.merchants(id,name,email,business_type,status,settings) VALUES('merch_int_1','M','m1@test.com','company','active','{}') ON CONFLICT (id) DO NOTHING`)

	repo := order.NewPostgresRepository(db)
	svc := order.NewService(repo)
	created, err := svc.Create(context.Background(), order.CreateInput{MerchantID: "merch_int_1", Amount: 50000, Currency: "INR", Receipt: "it_order_1"})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	fetched, err := repo.GetByID(context.Background(), "merch_int_1", created.ID)
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("expected order id %s got %s", created.ID, fetched.ID)
	}
}
