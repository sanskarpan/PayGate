package billing

import (
	"context"
	"time"
)

type Repository interface {
	CreateCustomer(ctx context.Context, customer Customer) (Customer, error)
	GetCustomer(ctx context.Context, merchantID, customerID string) (Customer, error)
	ListCustomers(ctx context.Context, merchantID string, limit int) ([]Customer, error)
	UpdateCustomer(ctx context.Context, customer Customer) (Customer, error)

	CreateUPIMandate(ctx context.Context, mandate UPIMandate, actorType, actorID string) (UPIMandate, error)
	GetUPIMandate(ctx context.Context, merchantID, mandateID string) (UPIMandate, error)
	ListUPIMandates(ctx context.Context, merchantID string, limit int) ([]UPIMandate, error)
	UpdateUPIMandateStatus(ctx context.Context, merchantID, mandateID string, status UPIMandateStatus, actorType, actorID, reason string) (UPIMandate, error)
	ListUPIMandateEvents(ctx context.Context, merchantID, mandateID string, limit int) ([]UPIMandateEvent, error)
	RecordUPIMandateChargeResult(ctx context.Context, merchantID, mandateID string, eventType UPIMandateEventType, paymentID, reason string, metadata map[string]any) error

	CreateVirtualAccount(ctx context.Context, account VirtualAccount) (VirtualAccount, error)
	GetVirtualAccount(ctx context.Context, merchantID, virtualAccountID string) (VirtualAccount, error)
	ListVirtualAccounts(ctx context.Context, merchantID string, limit int) ([]VirtualAccount, error)
	CreateInboundCollection(ctx context.Context, collection InboundCollection) (InboundCollection, error)
	GetInboundCollection(ctx context.Context, merchantID, collectionID string) (InboundCollection, error)
	ListInboundCollections(ctx context.Context, merchantID string, limit int, reviewOnly bool) ([]InboundCollection, error)
	ReviewInboundCollection(ctx context.Context, merchantID, collectionID, orderID, customerID, notes string) (InboundCollection, error)

	CreateConnectedAccount(ctx context.Context, account ConnectedAccount) (ConnectedAccount, error)
	GetConnectedAccount(ctx context.Context, merchantID, accountID string) (ConnectedAccount, error)
	ListConnectedAccounts(ctx context.Context, merchantID string, limit int) ([]ConnectedAccount, error)

	CreateSubscription(ctx context.Context, subscription Subscription) (Subscription, error)
	GetSubscription(ctx context.Context, merchantID, subscriptionID string) (Subscription, error)
	ListSubscriptions(ctx context.Context, merchantID string, limit int) ([]Subscription, error)
	UpdateSubscriptionStatus(ctx context.Context, merchantID, subscriptionID string, status SubscriptionStatus, pauseReason string, cancelAtPeriodEnd bool, nextBillingAt *time.Time) (Subscription, error)
	LeaseDueSubscriptions(ctx context.Context, before time.Time, limit int) ([]Subscription, error)

	CreateInvoice(ctx context.Context, invoice Invoice) (Invoice, error)
	GetInvoice(ctx context.Context, merchantID, invoiceID string) (Invoice, error)
	ListInvoices(ctx context.Context, merchantID, subscriptionID string, limit int) ([]Invoice, error)
	MarkInvoiceReminded(ctx context.Context, merchantID, invoiceID string, remindedAt time.Time) (Invoice, error)
	CreatePaymentLink(ctx context.Context, link PaymentLink) (PaymentLink, error)
	GetPaymentLink(ctx context.Context, merchantID, linkID string) (PaymentLink, error)
	ListPaymentLinks(ctx context.Context, merchantID string, limit int) ([]PaymentLink, error)
	UpdatePaymentLinkStatus(ctx context.Context, merchantID, linkID string, status PaymentLinkStatus) (PaymentLink, error)
	MarkPaymentLinkVisited(ctx context.Context, merchantID, linkID string, visitedAt time.Time) (PaymentLink, error)
	CreateInvoiceAttempt(ctx context.Context, attempt InvoiceAttempt) (InvoiceAttempt, error)
	MarkInvoiceAttempt(ctx context.Context, merchantID, attemptID string, status InvoiceAttemptStatus, orderID, paymentID, failureCode, failureMessage string) error
	MarkInvoicePaid(ctx context.Context, merchantID, invoiceID, orderID, paymentID string, nextBillingAt time.Time) (Invoice, error)
	MarkInvoiceFailed(ctx context.Context, merchantID, invoiceID, failureCode, failureMessage string, nextBillingAt time.Time, retryCount int) (Invoice, error)
}
