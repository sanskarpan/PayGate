package billing

import (
	"context"
	"time"

	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
)

type CreateManualInvoiceInput struct {
	MerchantID        string `json:"-"`
	CustomerID        string `json:"customer_id"`
	Amount            int64  `json:"amount"`
	Currency          string `json:"currency"`
	DueAt             int64  `json:"due_at"`
	Description       string `json:"description"`
	ExternalReference string `json:"external_reference"`
	CollectionSurface string `json:"collection_surface"`
}

func (s *Service) CreateManualInvoice(ctx context.Context, in CreateManualInvoiceInput) (Invoice, error) {
	if _, err := s.repo.GetCustomer(ctx, in.MerchantID, in.CustomerID); err != nil {
		return Invoice{}, err
	}
	dueAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	if in.DueAt > 0 {
		dueAt = time.Unix(in.DueAt, 0).UTC()
	}
	normalizedCurrency := in.Currency
	if normalizedCurrency == "" {
		normalizedCurrency = "INR"
	}
	reference := in.ExternalReference
	if reference == "" {
		reference = "manual"
	}
	invoice := Invoice{
		MerchantID:        in.MerchantID,
		CustomerID:        in.CustomerID,
		ExternalReference: in.ExternalReference,
		Description:       in.Description,
		Amount:            in.Amount,
		Currency:          normalizedCurrency,
		Status:            InvoiceOpen,
		BillingReason:     "manual_invoice",
		PeriodStart:       dueAt,
		PeriodEnd:         dueAt,
		DueAt:             dueAt,
	}
	switch in.CollectionSurface {
	case "payment_link":
		link, err := s.CreatePaymentLink(ctx, CreatePaymentLinkInput{
			MerchantID:        in.MerchantID,
			CustomerID:        in.CustomerID,
			ExternalReference: in.ExternalReference,
			Title:             in.Description,
			Description:       in.Description,
			Amount:            in.Amount,
			Currency:          normalizedCurrency,
			CallbackURL:       "/checkout/callback",
			ExpiresAt:         dueAt.Unix(),
			Notes:             map[string]any{"manual_invoice": true},
		})
		if err != nil {
			return Invoice{}, err
		}
		invoice.OrderID = link.OrderID
		invoice.PaymentLinkID = link.ID
	case "virtual_account":
		orderResult, err := s.orderSvc.Create(ctx, order.CreateInput{
			MerchantID: in.MerchantID,
			Amount:     in.Amount,
			Currency:   normalizedCurrency,
			Receipt:    "invoice-" + reference,
			Notes:      map[string]any{"manual_invoice": true},
		})
		if err != nil {
			return Invoice{}, err
		}
		account, err := s.CreateVirtualAccount(ctx, CreateVirtualAccountInput{
			MerchantID: in.MerchantID,
			CustomerID: in.CustomerID,
			OrderID:    orderResult.ID,
			Reference:  "invoice-" + in.ExternalReference,
		})
		if err != nil {
			return Invoice{}, err
		}
		invoice.OrderID = orderResult.ID
		invoice.VirtualAccountID = account.ID
	default:
		orderResult, err := s.orderSvc.Create(ctx, order.CreateInput{
			MerchantID: in.MerchantID,
			Amount:     in.Amount,
			Currency:   normalizedCurrency,
			Receipt:    "invoice-" + reference,
			Notes:      map[string]any{"manual_invoice": true},
		})
		if err != nil {
			return Invoice{}, err
		}
		invoice.OrderID = orderResult.ID
	}
	created, err := s.repo.CreateInvoice(ctx, invoice)
	if err != nil {
		return Invoice{}, err
	}
	return s.hydrateInvoice(ctx, created)
}

func (s *Service) SendInvoiceReminder(ctx context.Context, merchantID, invoiceID string) (Invoice, error) {
	invoice, err := s.repo.MarkInvoiceReminded(ctx, merchantID, invoiceID, time.Now().UTC())
	if err != nil {
		return Invoice{}, err
	}
	return s.hydrateInvoice(ctx, invoice)
}

func (s *Service) hydrateInvoice(ctx context.Context, invoice Invoice) (Invoice, error) {
	if invoice.Status == InvoicePaid || invoice.Status == InvoiceFailed || invoice.Status == InvoiceVoid {
		invoice.Overdue = invoice.Status == InvoiceOpen && time.Now().UTC().After(invoice.DueAt)
		return invoice, nil
	}
	if invoice.OrderID != "" && s.orderSvc != nil {
		orderResult, err := s.orderSvc.GetByID(ctx, invoice.MerchantID, invoice.OrderID)
		if err == nil && orderResult.Status == order.StatePaid {
			invoice.Status = InvoicePaid
		}
	}
	if invoice.PaymentID == "" && invoice.OrderID != "" && s.paymentSvc != nil {
		if list, err := s.paymentSvc.List(ctx, payment.ListFilter{MerchantID: invoice.MerchantID, OrderID: invoice.OrderID, Count: 10}); err == nil {
			for _, item := range list.Items {
				if item.Status == payment.StateCaptured {
					invoice.PaymentID = item.PaymentID
					invoice.Status = InvoicePaid
					break
				}
			}
		}
	}
	invoice.Overdue = invoice.Status == InvoiceOpen && time.Now().UTC().After(invoice.DueAt)
	return invoice, nil
}
