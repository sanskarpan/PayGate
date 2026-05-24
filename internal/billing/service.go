package billing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/tokenization"
)

type Service struct {
	repo       Repository
	orderSvc   *order.Service
	paymentSvc *payment.Service
	tokenSvc   *tokenization.Service
}

func NewService(repo Repository, orderSvc *order.Service, paymentSvc *payment.Service, tokenSvc *tokenization.Service) *Service {
	return &Service{repo: repo, orderSvc: orderSvc, paymentSvc: paymentSvc, tokenSvc: tokenSvc}
}

type CreateCustomerInput struct {
	MerchantID            string         `json:"-"`
	Name                  string         `json:"name"`
	Email                 string         `json:"email"`
	Phone                 string         `json:"phone"`
	ExternalReference     string         `json:"external_reference"`
	DefaultPaymentTokenID string         `json:"default_payment_token_id"`
	Metadata              map[string]any `json:"metadata"`
}

type CreateVirtualAccountInput struct {
	MerchantID string         `json:"-"`
	CustomerID string         `json:"customer_id"`
	OrderID    string         `json:"order_id"`
	Reference  string         `json:"reference"`
	Metadata   map[string]any `json:"metadata"`
}

type RecordInboundCollectionInput struct {
	MerchantID       string `json:"-"`
	VirtualAccountID string `json:"virtual_account_id"`
	OrderID          string `json:"order_id"`
	Amount           int64  `json:"amount"`
	Currency         string `json:"currency"`
	RemitterName     string `json:"remitter_name"`
	RemitterAccount  string `json:"remitter_account"`
	RemitterIFSC     string `json:"remitter_ifsc"`
	RemitterVPA      string `json:"remitter_vpa"`
	UTR              string `json:"utr"`
}

type CreateConnectedAccountInput struct {
	MerchantID        string         `json:"-"`
	LinkedMerchantID  string         `json:"linked_merchant_id"`
	BeneficiaryID     string         `json:"beneficiary_id"`
	DisplayName       string         `json:"display_name"`
	ExternalReference string         `json:"external_reference"`
	Metadata          map[string]any `json:"metadata"`
}

type CreateSubscriptionInput struct {
	MerchantID           string         `json:"-"`
	CustomerID           string         `json:"customer_id"`
	PlanName             string         `json:"plan_name"`
	PaymentMethodTokenID string         `json:"payment_method_token_id"`
	Amount               int64          `json:"amount"`
	Currency             string         `json:"currency"`
	IntervalUnit         IntervalUnit   `json:"interval_unit"`
	IntervalCount        int            `json:"interval_count"`
	StartsAt             int64          `json:"starts_at"`
	MaxRetryCount        int            `json:"max_retry_count"`
	RetryIntervalHours   int            `json:"retry_interval_hours"`
	Metadata             map[string]any `json:"metadata"`
}

func (s *Service) CreateCustomer(ctx context.Context, in CreateCustomerInput) (Customer, error) {
	customer := Customer{
		MerchantID:            in.MerchantID,
		Name:                  strings.TrimSpace(in.Name),
		Email:                 strings.TrimSpace(in.Email),
		Phone:                 strings.TrimSpace(in.Phone),
		ExternalReference:     strings.TrimSpace(in.ExternalReference),
		DefaultPaymentTokenID: strings.TrimSpace(in.DefaultPaymentTokenID),
		Metadata:              in.Metadata,
	}
	if err := customer.Validate(); err != nil {
		return Customer{}, err
	}
	return s.repo.CreateCustomer(ctx, customer)
}

func (s *Service) CreateVirtualAccount(ctx context.Context, in CreateVirtualAccountInput) (VirtualAccount, error) {
	account := VirtualAccount{
		MerchantID: in.MerchantID,
		CustomerID: strings.TrimSpace(in.CustomerID),
		OrderID:    strings.TrimSpace(in.OrderID),
		Reference:  strings.TrimSpace(in.Reference),
		Provider:   "simulated",
		Status:     VirtualAccountActive,
		Metadata:   in.Metadata,
	}
	if account.Reference == "" {
		account.Reference = "va-" + idgen.New("ref")
	}
	if account.CustomerID != "" {
		if _, err := s.repo.GetCustomer(ctx, in.MerchantID, account.CustomerID); err != nil {
			return VirtualAccount{}, err
		}
	}
	if account.OrderID != "" && s.orderSvc != nil {
		if _, err := s.orderSvc.GetByID(ctx, in.MerchantID, account.OrderID); err != nil {
			return VirtualAccount{}, err
		}
	}
	if err := account.Validate(); err != nil {
		return VirtualAccount{}, err
	}
	return s.repo.CreateVirtualAccount(ctx, account)
}

func (s *Service) GetVirtualAccount(ctx context.Context, merchantID, virtualAccountID string) (VirtualAccount, error) {
	return s.repo.GetVirtualAccount(ctx, merchantID, virtualAccountID)
}

func (s *Service) ListVirtualAccounts(ctx context.Context, merchantID string, limit int) ([]VirtualAccount, error) {
	return s.repo.ListVirtualAccounts(ctx, merchantID, limit)
}

func (s *Service) RecordInboundCollection(ctx context.Context, in RecordInboundCollectionInput) (InboundCollection, error) {
	account, err := s.repo.GetVirtualAccount(ctx, in.MerchantID, in.VirtualAccountID)
	if err != nil {
		return InboundCollection{}, err
	}
	collection := InboundCollection{
		MerchantID:       in.MerchantID,
		VirtualAccountID: account.ID,
		CustomerID:       account.CustomerID,
		OrderID:          account.OrderID,
		Amount:           in.Amount,
		Currency:         strings.ToUpper(strings.TrimSpace(in.Currency)),
		RemitterName:     strings.TrimSpace(in.RemitterName),
		RemitterAccount:  strings.TrimSpace(in.RemitterAccount),
		RemitterIFSC:     strings.TrimSpace(strings.ToUpper(in.RemitterIFSC)),
		RemitterVPA:      strings.TrimSpace(strings.ToLower(in.RemitterVPA)),
		UTR:              strings.TrimSpace(in.UTR),
		Status:           CollectionReviewRequired,
	}
	if collection.Currency == "" {
		collection.Currency = "INR"
	}
	if overrideOrderID := strings.TrimSpace(in.OrderID); overrideOrderID != "" {
		collection.OrderID = overrideOrderID
	}
	if collection.OrderID != "" {
		if s.orderSvc != nil {
			if _, err := s.orderSvc.GetByID(ctx, in.MerchantID, collection.OrderID); err != nil {
				return InboundCollection{}, err
			}
		}
		collection.Status = CollectionMatched
	}
	if err := collection.Validate(); err != nil {
		return InboundCollection{}, err
	}
	return s.repo.CreateInboundCollection(ctx, collection)
}

func (s *Service) ListInboundCollections(ctx context.Context, merchantID string, limit int, reviewOnly bool) ([]InboundCollection, error) {
	return s.repo.ListInboundCollections(ctx, merchantID, limit, reviewOnly)
}

func (s *Service) ReviewInboundCollection(ctx context.Context, merchantID, collectionID, orderID, customerID, notes string) (InboundCollection, error) {
	if orderID != "" && s.orderSvc != nil {
		if _, err := s.orderSvc.GetByID(ctx, merchantID, orderID); err != nil {
			return InboundCollection{}, err
		}
	}
	if customerID != "" {
		if _, err := s.repo.GetCustomer(ctx, merchantID, customerID); err != nil {
			return InboundCollection{}, err
		}
	}
	return s.repo.ReviewInboundCollection(ctx, merchantID, collectionID, strings.TrimSpace(orderID), strings.TrimSpace(customerID), strings.TrimSpace(notes))
}

func (s *Service) CreateConnectedAccount(ctx context.Context, in CreateConnectedAccountInput) (ConnectedAccount, error) {
	account := ConnectedAccount{
		MerchantID:        in.MerchantID,
		LinkedMerchantID:  strings.TrimSpace(in.LinkedMerchantID),
		BeneficiaryID:     strings.TrimSpace(in.BeneficiaryID),
		DisplayName:       strings.TrimSpace(in.DisplayName),
		ExternalReference: strings.TrimSpace(in.ExternalReference),
		Status:            ConnectedAccountActive,
		Metadata:          in.Metadata,
	}
	if err := account.Validate(); err != nil {
		return ConnectedAccount{}, err
	}
	return s.repo.CreateConnectedAccount(ctx, account)
}

func (s *Service) GetConnectedAccount(ctx context.Context, merchantID, accountID string) (ConnectedAccount, error) {
	return s.repo.GetConnectedAccount(ctx, merchantID, accountID)
}

func (s *Service) ListConnectedAccounts(ctx context.Context, merchantID string, limit int) ([]ConnectedAccount, error) {
	return s.repo.ListConnectedAccounts(ctx, merchantID, limit)
}

func (s *Service) GetCustomer(ctx context.Context, merchantID, customerID string) (Customer, error) {
	return s.repo.GetCustomer(ctx, merchantID, customerID)
}

func (s *Service) ListCustomers(ctx context.Context, merchantID string, limit int) ([]Customer, error) {
	return s.repo.ListCustomers(ctx, merchantID, limit)
}

func (s *Service) UpdateCustomer(ctx context.Context, customer Customer) (Customer, error) {
	if err := customer.Validate(); err != nil {
		return Customer{}, err
	}
	return s.repo.UpdateCustomer(ctx, customer)
}

func (s *Service) CreateSubscription(ctx context.Context, in CreateSubscriptionInput) (Subscription, error) {
	customer, err := s.repo.GetCustomer(ctx, in.MerchantID, in.CustomerID)
	if err != nil {
		return Subscription{}, err
	}
	token, err := s.tokenSvc.GetCardToken(ctx, in.MerchantID, in.PaymentMethodTokenID)
	if err != nil {
		return Subscription{}, err
	}
	if token.TokenClass != tokenization.CardTokenClassReusable {
		return Subscription{}, ErrCardTokenNotReusable
	}
	if token.CustomerRef != "" && token.CustomerRef != customer.ID {
		return Subscription{}, ErrCustomerTokenMismatch
	}
	startAt := time.Now().UTC()
	if in.StartsAt > 0 {
		startAt = time.Unix(in.StartsAt, 0).UTC()
	}
	if in.MaxRetryCount <= 0 {
		in.MaxRetryCount = 3
	}
	if in.RetryIntervalHours <= 0 {
		in.RetryIntervalHours = 24
	}
	subscription := Subscription{
		MerchantID:           in.MerchantID,
		CustomerID:           customer.ID,
		PlanName:             strings.TrimSpace(in.PlanName),
		PaymentMethodTokenID: in.PaymentMethodTokenID,
		Amount:               in.Amount,
		Currency:             strings.ToUpper(strings.TrimSpace(in.Currency)),
		IntervalUnit:         in.IntervalUnit,
		IntervalCount:        in.IntervalCount,
		Status:               SubscriptionActive,
		NextBillingAt:        startAt,
		MaxRetryCount:        in.MaxRetryCount,
		RetryIntervalHours:   in.RetryIntervalHours,
		Metadata:             in.Metadata,
	}
	if subscription.Currency == "" {
		subscription.Currency = "INR"
	}
	if err := subscription.Validate(); err != nil {
		return Subscription{}, err
	}
	return s.repo.CreateSubscription(ctx, subscription)
}

func (s *Service) GetSubscription(ctx context.Context, merchantID, subscriptionID string) (Subscription, error) {
	return s.repo.GetSubscription(ctx, merchantID, subscriptionID)
}

func (s *Service) ListSubscriptions(ctx context.Context, merchantID string, limit int) ([]Subscription, error) {
	return s.repo.ListSubscriptions(ctx, merchantID, limit)
}

func (s *Service) PauseSubscription(ctx context.Context, merchantID, subscriptionID, reason string) (Subscription, error) {
	return s.repo.UpdateSubscriptionStatus(ctx, merchantID, subscriptionID, SubscriptionPaused, reason, false, nil)
}

func (s *Service) ResumeSubscription(ctx context.Context, merchantID, subscriptionID string) (Subscription, error) {
	nextAt := time.Now().UTC()
	return s.repo.UpdateSubscriptionStatus(ctx, merchantID, subscriptionID, SubscriptionActive, "", false, &nextAt)
}

func (s *Service) CancelSubscription(ctx context.Context, merchantID, subscriptionID string, atPeriodEnd bool) (Subscription, error) {
	status := SubscriptionCanceled
	if atPeriodEnd {
		status = SubscriptionActive
	}
	return s.repo.UpdateSubscriptionStatus(ctx, merchantID, subscriptionID, status, "", atPeriodEnd, nil)
}

func (s *Service) GetInvoice(ctx context.Context, merchantID, invoiceID string) (Invoice, error) {
	return s.repo.GetInvoice(ctx, merchantID, invoiceID)
}

func (s *Service) ListInvoices(ctx context.Context, merchantID, subscriptionID string, limit int) ([]Invoice, error) {
	return s.repo.ListInvoices(ctx, merchantID, subscriptionID, limit)
}

func (s *Service) RunSubscription(ctx context.Context, merchantID, subscriptionID string) (Invoice, payment.CaptureResult, error) {
	subscription, err := s.repo.GetSubscription(ctx, merchantID, subscriptionID)
	if err != nil {
		return Invoice{}, payment.CaptureResult{}, err
	}
	return s.runSubscription(ctx, subscription)
}

func (s *Service) RunDueSubscriptions(ctx context.Context, limit int) ([]Invoice, error) {
	due, err := s.repo.LeaseDueSubscriptions(ctx, time.Now().UTC(), limit)
	if err != nil {
		return nil, err
	}
	var invoices []Invoice
	for _, subscription := range due {
		invoice, _, err := s.runSubscription(ctx, subscription)
		if err == nil {
			invoices = append(invoices, invoice)
		}
	}
	return invoices, nil
}

func (s *Service) runSubscription(ctx context.Context, subscription Subscription) (Invoice, payment.CaptureResult, error) {
	if subscription.Status != SubscriptionActive {
		return Invoice{}, payment.CaptureResult{}, ErrSubscriptionNotActive
	}
	periodStart := subscription.NextBillingAt
	periodEnd := nextPeriodStart(periodStart, subscription.IntervalUnit, subscription.IntervalCount)
	invoice, err := s.repo.CreateInvoice(ctx, Invoice{
		MerchantID:     subscription.MerchantID,
		CustomerID:     subscription.CustomerID,
		SubscriptionID: subscription.ID,
		Amount:         subscription.Amount,
		Currency:       subscription.Currency,
		Status:         InvoiceOpen,
		BillingReason:  "subscription_cycle",
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		DueAt:          periodStart,
	})
	if err != nil {
		return Invoice{}, payment.CaptureResult{}, err
	}
	attempt, err := s.repo.CreateInvoiceAttempt(ctx, InvoiceAttempt{
		InvoiceID:      invoice.ID,
		MerchantID:     subscription.MerchantID,
		SubscriptionID: subscription.ID,
		AttemptNumber:  subscription.RetryCount + 1,
		Status:         InvoiceAttemptStarted,
	})
	if err != nil {
		return Invoice{}, payment.CaptureResult{}, err
	}
	orderResult, err := s.orderSvc.Create(ctx, order.CreateInput{
		MerchantID:     subscription.MerchantID,
		IdempotencyKey: "invoice:" + invoice.ID,
		Amount:         subscription.Amount,
		Currency:       subscription.Currency,
		Receipt:        fmt.Sprintf("sub-%s-%d", subscription.ID, time.Now().UTC().Unix()),
		Notes: map[string]any{
			"subscription_id": subscription.ID,
			"invoice_id":      invoice.ID,
			"customer_id":     subscription.CustomerID,
		},
	})
	if err != nil {
		nextAt, retryCount := subscriptionFailureSchedule(subscription)
		_ = s.repo.MarkInvoiceAttempt(ctx, subscription.MerchantID, attempt.ID, InvoiceAttemptFailed, "", "", "ORDER_CREATE_FAILED", err.Error())
		invoice, _ = s.repo.MarkInvoiceFailed(ctx, subscription.MerchantID, invoice.ID, "ORDER_CREATE_FAILED", err.Error(), nextAt, retryCount)
		return invoice, payment.CaptureResult{}, err
	}
	_ = s.repo.MarkInvoiceAttempt(ctx, subscription.MerchantID, attempt.ID, InvoiceAttemptStarted, orderResult.ID, "", "", "")
	authResult, err := s.paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID:           subscription.MerchantID,
		OrderID:              orderResult.ID,
		Amount:               subscription.Amount,
		Currency:             subscription.Currency,
		Method:               "card",
		PaymentMethodTokenID: subscription.PaymentMethodTokenID,
		IdempotencyKey:       "invoice-payment:" + invoice.ID,
		PaymentID:            idgen.New("pay"),
	})
	if err != nil {
		nextAt, retryCount := subscriptionFailureSchedule(subscription)
		_ = s.repo.MarkInvoiceAttempt(ctx, subscription.MerchantID, attempt.ID, InvoiceAttemptFailed, orderResult.ID, "", "AUTH_FAILED", err.Error())
		invoice, _ = s.repo.MarkInvoiceFailed(ctx, subscription.MerchantID, invoice.ID, "AUTH_FAILED", err.Error(), nextAt, retryCount)
		return invoice, payment.CaptureResult{}, err
	}
	_ = s.repo.MarkInvoiceAttempt(ctx, subscription.MerchantID, attempt.ID, InvoiceAttemptAuthorized, orderResult.ID, authResult.PaymentID, "", "")
	captureResult, err := s.paymentSvc.CaptureForMerchant(ctx, subscription.MerchantID, authResult.PaymentID, subscription.Amount)
	if err != nil {
		_, _ = s.paymentSvc.ReverseAuthorization(ctx, subscription.MerchantID, authResult.PaymentID, "subscription capture failed")
		nextAt, retryCount := subscriptionFailureSchedule(subscription)
		_ = s.repo.MarkInvoiceAttempt(ctx, subscription.MerchantID, attempt.ID, InvoiceAttemptFailed, orderResult.ID, authResult.PaymentID, "CAPTURE_FAILED", err.Error())
		invoice, _ = s.repo.MarkInvoiceFailed(ctx, subscription.MerchantID, invoice.ID, "CAPTURE_FAILED", err.Error(), nextAt, retryCount)
		return invoice, payment.CaptureResult{}, err
	}
	_ = s.repo.MarkInvoiceAttempt(ctx, subscription.MerchantID, attempt.ID, InvoiceAttemptCaptured, orderResult.ID, captureResult.PaymentID, "", "")
	invoice, err = s.repo.MarkInvoicePaid(ctx, subscription.MerchantID, invoice.ID, orderResult.ID, captureResult.PaymentID, periodEnd)
	if err != nil {
		return Invoice{}, payment.CaptureResult{}, err
	}
	return invoice, captureResult, nil
}

func subscriptionFailureSchedule(subscription Subscription) (time.Time, int) {
	retryCount := subscription.RetryCount + 1
	if retryCount <= subscription.MaxRetryCount {
		return time.Now().UTC().Add(time.Duration(subscription.RetryIntervalHours) * time.Hour), retryCount
	}
	return nextPeriodStart(subscription.NextBillingAt, subscription.IntervalUnit, subscription.IntervalCount), 0
}
