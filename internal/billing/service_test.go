package billing

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	due []Subscription
}

func (f *fakeRepository) CreateCustomer(context.Context, Customer) (Customer, error) { return Customer{}, nil }
func (f *fakeRepository) GetCustomer(context.Context, string, string) (Customer, error) {
	return Customer{}, nil
}
func (f *fakeRepository) ListCustomers(context.Context, string, int) ([]Customer, error) { return nil, nil }
func (f *fakeRepository) UpdateCustomer(context.Context, Customer) (Customer, error) { return Customer{}, nil }
func (f *fakeRepository) CreateUPIMandate(context.Context, UPIMandate, string, string) (UPIMandate, error) {
	return UPIMandate{}, nil
}
func (f *fakeRepository) GetUPIMandate(context.Context, string, string) (UPIMandate, error) {
	return UPIMandate{}, nil
}
func (f *fakeRepository) ListUPIMandates(context.Context, string, int) ([]UPIMandate, error) { return nil, nil }
func (f *fakeRepository) UpdateUPIMandateStatus(context.Context, string, string, UPIMandateStatus, string, string, string) (UPIMandate, error) {
	return UPIMandate{}, nil
}
func (f *fakeRepository) ListUPIMandateEvents(context.Context, string, string, int) ([]UPIMandateEvent, error) {
	return nil, nil
}
func (f *fakeRepository) RecordUPIMandateChargeResult(context.Context, string, string, UPIMandateEventType, string, string, map[string]any) error {
	return nil
}
func (f *fakeRepository) CreateVirtualAccount(context.Context, VirtualAccount) (VirtualAccount, error) {
	return VirtualAccount{}, nil
}
func (f *fakeRepository) GetVirtualAccount(context.Context, string, string) (VirtualAccount, error) {
	return VirtualAccount{}, nil
}
func (f *fakeRepository) ListVirtualAccounts(context.Context, string, int) ([]VirtualAccount, error) {
	return nil, nil
}
func (f *fakeRepository) CreateInboundCollection(context.Context, InboundCollection) (InboundCollection, error) {
	return InboundCollection{}, nil
}
func (f *fakeRepository) GetInboundCollection(context.Context, string, string) (InboundCollection, error) {
	return InboundCollection{}, nil
}
func (f *fakeRepository) ListInboundCollections(context.Context, string, int, bool) ([]InboundCollection, error) {
	return nil, nil
}
func (f *fakeRepository) ReviewInboundCollection(context.Context, string, string, string, string, string) (InboundCollection, error) {
	return InboundCollection{}, nil
}
func (f *fakeRepository) CreateConnectedAccount(context.Context, ConnectedAccount) (ConnectedAccount, error) {
	return ConnectedAccount{}, nil
}
func (f *fakeRepository) GetConnectedAccount(context.Context, string, string) (ConnectedAccount, error) {
	return ConnectedAccount{}, nil
}
func (f *fakeRepository) ListConnectedAccounts(context.Context, string, int) ([]ConnectedAccount, error) {
	return nil, nil
}
func (f *fakeRepository) CreateSubscription(context.Context, Subscription) (Subscription, error) {
	return Subscription{}, nil
}
func (f *fakeRepository) GetSubscription(context.Context, string, string) (Subscription, error) {
	return Subscription{}, nil
}
func (f *fakeRepository) ListSubscriptions(context.Context, string, int) ([]Subscription, error) { return nil, nil }
func (f *fakeRepository) UpdateSubscriptionStatus(context.Context, string, string, SubscriptionStatus, string, bool, *time.Time) (Subscription, error) {
	return Subscription{}, nil
}
func (f *fakeRepository) LeaseDueSubscriptions(context.Context, time.Time, int) ([]Subscription, error) {
	return f.due, nil
}
func (f *fakeRepository) CreateInvoice(context.Context, Invoice) (Invoice, error)  { return Invoice{}, nil }
func (f *fakeRepository) GetInvoice(context.Context, string, string) (Invoice, error) {
	return Invoice{}, nil
}
func (f *fakeRepository) ListInvoices(context.Context, string, string, int) ([]Invoice, error) { return nil, nil }
func (f *fakeRepository) MarkInvoiceReminded(context.Context, string, string, time.Time) (Invoice, error) {
	return Invoice{}, nil
}
func (f *fakeRepository) CreatePaymentLink(context.Context, PaymentLink) (PaymentLink, error) {
	return PaymentLink{}, nil
}
func (f *fakeRepository) GetPaymentLink(context.Context, string, string) (PaymentLink, error) {
	return PaymentLink{}, nil
}
func (f *fakeRepository) ListPaymentLinks(context.Context, string, int) ([]PaymentLink, error) { return nil, nil }
func (f *fakeRepository) UpdatePaymentLinkStatus(context.Context, string, string, PaymentLinkStatus) (PaymentLink, error) {
	return PaymentLink{}, nil
}
func (f *fakeRepository) MarkPaymentLinkVisited(context.Context, string, string, time.Time) (PaymentLink, error) {
	return PaymentLink{}, nil
}
func (f *fakeRepository) CreateInvoiceAttempt(context.Context, InvoiceAttempt) (InvoiceAttempt, error) {
	return InvoiceAttempt{}, nil
}
func (f *fakeRepository) MarkInvoiceAttempt(context.Context, string, string, InvoiceAttemptStatus, string, string, string, string) error {
	return nil
}
func (f *fakeRepository) MarkInvoicePaid(context.Context, string, string, string, string, time.Time) (Invoice, error) {
	return Invoice{}, nil
}
func (f *fakeRepository) MarkInvoiceFailed(context.Context, string, string, string, string, time.Time, int) (Invoice, error) {
	return Invoice{}, nil
}

func TestRunDueSubscriptionsSurfacesFailures(t *testing.T) {
	repo := &fakeRepository{
		due: []Subscription{
			{ID: "sub_paused", MerchantID: "merch_1", Status: SubscriptionPaused},
			{ID: "sub_canceled", MerchantID: "merch_1", Status: SubscriptionCanceled},
		},
	}
	svc := NewService(repo, nil, nil, nil)

	invoices, err := svc.RunDueSubscriptions(context.Background(), 10)
	if len(invoices) != 0 {
		t.Fatalf("expected no invoices on failed run, got %d", len(invoices))
	}
	if err == nil {
		t.Fatal("expected joined error for failed due subscriptions")
	}
	for _, want := range []string{"sub_paused", "sub_canceled", ErrSubscriptionNotActive.Error()} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestRunDueSubscriptionsReturnsNilErrorWhenNothingFails(t *testing.T) {
	svc := NewService(&fakeRepository{}, nil, nil, nil)

	invoices, err := svc.RunDueSubscriptions(context.Background(), 10)
	if err != nil {
		t.Fatalf("expected nil error when no due subscriptions fail, got %v", err)
	}
	if len(invoices) != 0 {
		t.Fatalf("expected no invoices, got %d", len(invoices))
	}
}
