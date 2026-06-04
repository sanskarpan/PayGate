package billing

import (
	"context"
	"strings"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/order"
)

type CreatePaymentLinkInput struct {
	MerchantID        string         `json:"-"`
	CustomerID        string         `json:"customer_id"`
	ExternalReference string         `json:"external_reference"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	Amount            int64          `json:"amount"`
	Currency          string         `json:"currency"`
	CallbackURL       string         `json:"callback_url"`
	ExpiresAt         int64          `json:"expires_at"`
	Notes             map[string]any `json:"notes"`
}

func (s *Service) CreatePaymentLink(ctx context.Context, in CreatePaymentLinkInput) (PaymentLink, error) {
	if in.CustomerID != "" {
		if _, err := s.repo.GetCustomer(ctx, in.MerchantID, in.CustomerID); err != nil {
			return PaymentLink{}, err
		}
	}
	expiry := time.Now().UTC().Add(7 * 24 * time.Hour)
	if in.ExpiresAt > 0 {
		expiry = time.Unix(in.ExpiresAt, 0).UTC()
	}
	receipt := "plink-" + idgen.New("rcpt")
	idemKey := strings.TrimSpace(in.ExternalReference)
	if idemKey == "" {
		idemKey = receipt
	}
	orderNotes := map[string]any{
		"payment_link_title": strings.TrimSpace(in.Title),
	}
	for key, value := range in.Notes {
		orderNotes[key] = value
	}
	orderResult, err := s.orderSvc.Create(ctx, order.CreateInput{
		MerchantID:     in.MerchantID,
		IdempotencyKey: "payment-link:" + idemKey,
		Amount:         in.Amount,
		Currency:       strings.ToUpper(strings.TrimSpace(in.Currency)),
		Receipt:        receipt,
		Notes:          orderNotes,
	})
	if err != nil {
		return PaymentLink{}, err
	}
	link := PaymentLink{
		MerchantID:        in.MerchantID,
		CustomerID:        strings.TrimSpace(in.CustomerID),
		OrderID:           orderResult.ID,
		ExternalReference: strings.TrimSpace(in.ExternalReference),
		Title:             strings.TrimSpace(in.Title),
		Description:       strings.TrimSpace(in.Description),
		Amount:            in.Amount,
		Currency:          strings.ToUpper(strings.TrimSpace(in.Currency)),
		Status:            PaymentLinkActive,
		CallbackURL:       strings.TrimSpace(in.CallbackURL),
		Notes:             in.Notes,
		ExpiresAt:         expiry,
	}
	if link.Currency == "" {
		link.Currency = "INR"
	}
	if err := link.Validate(); err != nil {
		return PaymentLink{}, err
	}
	return s.repo.CreatePaymentLink(ctx, link)
}

func (s *Service) GetPaymentLink(ctx context.Context, merchantID, linkID string) (PaymentLink, error) {
	link, err := s.repo.GetPaymentLink(ctx, merchantID, linkID)
	if err != nil {
		return PaymentLink{}, err
	}
	return s.refreshPaymentLinkStatus(ctx, link)
}

func (s *Service) ListPaymentLinks(ctx context.Context, merchantID string, limit int) ([]PaymentLink, error) {
	items, err := s.repo.ListPaymentLinks(ctx, merchantID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]PaymentLink, 0, len(items))
	for _, item := range items {
		refreshed, err := s.refreshPaymentLinkStatus(ctx, item)
		if err != nil {
			return nil, err
		}
		out = append(out, refreshed)
	}
	return out, nil
}

func (s *Service) DisablePaymentLink(ctx context.Context, merchantID, linkID string) (PaymentLink, error) {
	return s.repo.UpdatePaymentLinkStatus(ctx, merchantID, linkID, PaymentLinkDisabled)
}

func (s *Service) ExpirePaymentLink(ctx context.Context, merchantID, linkID string) (PaymentLink, error) {
	return s.repo.UpdatePaymentLinkStatus(ctx, merchantID, linkID, PaymentLinkExpired)
}

func (s *Service) ResolvePaymentLink(ctx context.Context, merchantID, linkID string) (PaymentLink, order.Order, error) {
	link, err := s.GetPaymentLink(ctx, merchantID, linkID)
	if err != nil {
		return PaymentLink{}, order.Order{}, err
	}
	if link.Status != PaymentLinkActive {
		return PaymentLink{}, order.Order{}, ErrInvalidPaymentLink
	}
	if time.Now().UTC().After(link.ExpiresAt) {
		link, _ = s.repo.UpdatePaymentLinkStatus(ctx, merchantID, linkID, PaymentLinkExpired)
		return PaymentLink{}, order.Order{}, ErrInvalidPaymentLink
	}
	updated, err := s.repo.MarkPaymentLinkVisited(ctx, merchantID, linkID, time.Now().UTC())
	if err == nil {
		link = updated
	}
	orderResult, err := s.orderSvc.GetByID(ctx, merchantID, link.OrderID)
	if err != nil {
		return PaymentLink{}, order.Order{}, err
	}
	return link, orderResult, nil
}

func (s *Service) refreshPaymentLinkStatus(ctx context.Context, link PaymentLink) (PaymentLink, error) {
	if link.Status == PaymentLinkDisabled || link.Status == PaymentLinkExpired {
		return link, nil
	}
	if time.Now().UTC().After(link.ExpiresAt) && link.Status == PaymentLinkActive {
		return s.repo.UpdatePaymentLinkStatus(ctx, link.MerchantID, link.ID, PaymentLinkExpired)
	}
	if s.orderSvc == nil || link.OrderID == "" {
		return link, nil
	}
	orderResult, err := s.orderSvc.GetByID(ctx, link.MerchantID, link.OrderID)
	if err != nil {
		return PaymentLink{}, err
	}
	if orderResult.Status == order.StatePaid && link.Status != PaymentLinkPaid {
		return s.repo.UpdatePaymentLinkStatus(ctx, link.MerchantID, link.ID, PaymentLinkPaid)
	}
	return link, nil
}
