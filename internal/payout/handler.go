package payout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/settlement"
)

// Handler exposes the payout HTTP endpoints.
type Handler struct {
	svc        *Service
	settlement *settlement.Service
	ledger     *ledger.Service
	caps       interface {
		CheckCapability(ctx context.Context, merchantID string, capability merchant.CapabilityCode) error
	}
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, settlementSvc *settlement.Service, ledgerSvc *ledger.Service, caps interface {
	CheckCapability(ctx context.Context, merchantID string, capability merchant.CapabilityCode) error
}) *Handler {
	return &Handler{svc: svc, settlement: settlementSvc, ledger: ledgerSvc, caps: caps}
}

func (h *Handler) RegisterPublicRoutes(mux *http.ServeMux) {
	mux.Handle("POST /v1/payouts/rail/callbacks", http.HandlerFunc(h.railCallback))
}

// RegisterRoutesWithAuth wires payout endpoints into mux under auth.
func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(scope merchant.APIKeyScope, next http.Handler) http.Handler) {
	mux.Handle("POST /v1/settlements/{settlementID}/payout", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.initiate)))
	mux.Handle("GET /v1/beneficiaries", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listBeneficiaries)))
	mux.Handle("POST /v1/beneficiaries", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.createBeneficiary)))
	mux.Handle("GET /v1/beneficiaries/{beneficiaryID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getBeneficiary)))
	mux.Handle("POST /v1/beneficiaries/{beneficiaryID}/verify", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.verifyBeneficiary)))
	mux.Handle("POST /v1/beneficiaries/{beneficiaryID}/approve", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.approveBeneficiary)))
	mux.Handle("GET /v1/settlements/{settlementID}/payout-simulator", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getScenario)))
	mux.Handle("PUT /v1/settlements/{settlementID}/payout-simulator", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.putScenario)))
	mux.Handle("GET /v1/payouts", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.list)))
	mux.Handle("GET /v1/payouts/{payoutID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.get)))
	mux.Handle("POST /v1/payouts/{payoutID}/approve", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.approvePayout)))
	mux.Handle("POST /v1/payouts/{payoutID}/reject", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.rejectPayout)))
	mux.Handle("GET /v1/payouts/{payoutID}/approvals", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listApprovals)))
	mux.Handle("POST /v1/payout-batches", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.createBatch)))
	mux.Handle("POST /v1/payouts/{payoutID}/cancel", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.cancel)))
	mux.Handle("GET /v1/payouts/{payoutID}/events", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.events)))
}

func (h *Handler) initiate(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	settlementID := r.PathValue("settlementID")
	if h.caps != nil {
		if err := h.caps.CheckCapability(r.Context(), p.MerchantID, merchant.CapabilityPayouts); err != nil {
			handleError(w, err)
			return
		}
	}

	var req struct {
		BeneficiaryID string `json:"beneficiary_id"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
			return
		}
	}

	// Look up the settlement to get the net amount and currency.
	sttl, err := h.settlement.Get(r.Context(), p.MerchantID, settlementID)
	if err != nil {
		handleError(w, err)
		return
	}
	if sttl.OnHold {
		handleError(w, ErrSettlementOnHold)
		return
	}
	if sttl.RollbackMarkedAt != nil {
		handleError(w, settlement.ErrSettlementRollbackMarked)
		return
	}
	if h.ledger != nil {
		if err := h.ledger.CanReserveForPayout(r.Context(), p.MerchantID, sttl.Currency, sttl.NetAmount); err != nil {
			handleError(w, err)
			return
		}
	}
	prefs, err := h.settlement.GetPreferences(r.Context(), p.MerchantID)
	if err != nil {
		handleError(w, err)
		return
	}
	if prefs.PayoutMinimum > 0 && sttl.NetAmount < prefs.PayoutMinimum {
		handleError(w, ErrSettlementNotProcessed)
		return
	}

	pout, err := h.svc.InitiatePayoutForSettlement(r.Context(), p.MerchantID, settlementID, strings.TrimSpace(req.BeneficiaryID), sttl.NetAmount, sttl.Currency, prefs.ApprovalThresholdAmount, "")
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, present(pout))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	payouts, err := h.svc.List(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(payouts))
	for _, po := range payouts {
		items = append(items, present(po))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	pout, err := h.svc.Get(r.Context(), p.MerchantID, r.PathValue("payoutID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(pout))
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
			return
		}
	}
	pout, err := h.svc.Cancel(r.Context(), p.MerchantID, r.PathValue("payoutID"), req.Reason)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(pout))
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := h.svc.ListEvents(r.Context(), p.MerchantID, r.PathValue("payoutID"), limit)
	if err != nil {
		handleError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentEvent(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) railCallback(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid callback body"})
		return
	}
	timestamp := r.Header.Get(railTimestampHeader)
	signature := r.Header.Get(railSignatureHeader)
	if !VerifyRailPayload(h.svc.railSecret, timestamp, body, signature) {
		handleError(w, ErrInvalidRailSignature)
		return
	}
	var req struct {
		EventID       string `json:"event_id"`
		PayoutID      string `json:"payout_id"`
		MerchantID    string `json:"merchant_id"`
		Status        string `json:"status"`
		RailReference string `json:"rail_reference"`
		Reason        string `json:"reason"`
		OccurredAt    string `json:"occurred_at"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid callback payload"})
		return
	}
	var occurredAt time.Time
	if strings.TrimSpace(req.OccurredAt) != "" {
		occurredAt, err = time.Parse(time.RFC3339, req.OccurredAt)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "occurred_at must be RFC3339"})
			return
		}
	}
	status := RailCallbackStatus(strings.TrimSpace(req.Status))
	if strings.TrimSpace(req.EventID) == "" || strings.TrimSpace(req.PayoutID) == "" || strings.TrimSpace(req.MerchantID) == "" {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "event_id, payout_id, and merchant_id are required"})
		return
	}
	switch status {
	case RailStatusProcessing, RailStatusCompleted, RailStatusFailed, RailStatusReturned, RailStatusReversed:
	default:
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "unsupported rail callback status"})
		return
	}
	payoutOut, processed, err := h.svc.ApplyRailCallback(r.Context(), RailCallback{
		EventID:       strings.TrimSpace(req.EventID),
		PayoutID:      strings.TrimSpace(req.PayoutID),
		MerchantID:    strings.TrimSpace(req.MerchantID),
		Status:        status,
		RailReference: strings.TrimSpace(req.RailReference),
		Reason:        strings.TrimSpace(req.Reason),
		OccurredAt:    occurredAt,
	}, signature)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"entity":    "payout_rail_callback",
		"processed": processed,
		"payout":    present(payoutOut),
	})
}

func (h *Handler) getScenario(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	scenario, err := h.svc.GetSimulatorScenario(r.Context(), p.MerchantID, r.PathValue("settlementID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentScenario(scenario))
}

func (h *Handler) putScenario(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		TransientFailuresRemaining int                     `json:"transient_failures_remaining"`
		Notes                      string                  `json:"notes"`
		Steps                      []SimulatorScenarioStep `json:"steps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid payout simulator payload"})
		return
	}
	scenario, err := h.svc.UpsertSimulatorScenario(r.Context(), p.MerchantID, r.PathValue("settlementID"), SimulatorScenario{
		TransientFailuresRemaining: req.TransientFailuresRemaining,
		Notes:                      strings.TrimSpace(req.Notes),
		Steps:                      req.Steps,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentScenario(scenario))
}

func present(p Payout) map[string]any {
	var initiatedAt, completedAt any
	if p.InitiatedAt != nil {
		initiatedAt = p.InitiatedAt.Unix()
	}
	if p.CompletedAt != nil {
		completedAt = p.CompletedAt.Unix()
	}
	return map[string]any{
		"entity":          "payout",
		"id":              p.ID,
		"merchant_id":     p.MerchantID,
		"settlement_id":   p.SettlementID,
		"beneficiary_id":  p.BeneficiaryID,
		"saga_id":         p.SagaID,
		"status":          p.Status,
		"approval_status": p.ApprovalStatus,
		"amount":          p.Amount,
		"currency":        p.Currency,
		"bank_reference":  p.BankReference,
		"rail_reference":  p.RailReference,
		"failure_reason":  p.FailureReason,
		"return_reason":   p.ReturnReason,
		"initiated_at":    initiatedAt,
		"completed_at":    completedAt,
		"failed_at":       presentTime(p.FailedAt),
		"returned_at":     presentTime(p.ReturnedAt),
		"reversed_at":     presentTime(p.ReversedAt),
		"cancelled_at":    presentTime(p.CancelledAt),
		"cancel_reason":   p.CancelReason,
		"created_at":      p.CreatedAt.Unix(),
	}
}

func presentEvent(event TimelineEvent) map[string]any {
	return map[string]any{
		"entity":            "payout_event",
		"id":                event.ID,
		"payout_id":         event.PayoutID,
		"merchant_id":       event.MerchantID,
		"event_type":        event.EventType,
		"status_before":     event.StatusBefore,
		"status_after":      event.StatusAfter,
		"callback_event_id": event.CallbackEventID,
		"payload":           event.Payload,
		"created_at":        event.CreatedAt.Unix(),
	}
}

func presentTime(ts *time.Time) any {
	if ts == nil {
		return nil
	}
	return ts.Unix()
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrPayoutNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrPayoutAlreadyExists):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "PAYOUT_ALREADY_EXISTS", Description: err.Error()})
	case errors.Is(err, ErrInvalidTransition):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "INVALID_STATE", Description: err.Error()})
	case errors.Is(err, ErrSettlementNotProcessed):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "SETTLEMENT_NOT_PROCESSED", Description: err.Error()})
	case errors.Is(err, ErrSettlementOnHold):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "SETTLEMENT_ON_HOLD", Description: err.Error()})
	case errors.Is(err, ErrBeneficiaryNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "BENEFICIARY_NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrBeneficiaryInvalid), errors.Is(err, ErrInvalidSimulatorScenario):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: err.Error()})
	case errors.Is(err, ErrBeneficiaryNotApproved), errors.Is(err, ErrPayoutApprovalRequired), errors.Is(err, ErrPayoutApprovalRejected):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "PAYOUT_APPROVAL_REQUIRED", Description: err.Error()})
	case errors.Is(err, settlement.ErrSettlementRollbackMarked):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "SETTLEMENT_ROLLBACK_MARKED", Description: err.Error()})
	case errors.Is(err, ErrInvalidRailSignature):
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "INVALID_SIGNATURE", Description: err.Error()})
	case errors.Is(err, ledger.ErrHoldInsufficient):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "INSUFFICIENT_PAYOUTABLE_BALANCE", Description: err.Error()})
	case errors.Is(err, merchant.ErrCapabilityRestricted):
		httpx.WriteError(w, http.StatusForbidden, httpx.APIError{Code: "CAPABILITY_RESTRICTED", Description: err.Error()})
	default:
		// Check for settlement not found too.
		var errBody struct{ Code string }
		if b, merr := json.Marshal(err); merr == nil {
			_ = json.Unmarshal(b, &errBody)
		}
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
	}
}

func presentScenario(s SimulatorScenario) map[string]any {
	steps := make([]map[string]any, 0, len(s.Steps))
	for _, step := range s.Steps {
		steps = append(steps, map[string]any{
			"status":          step.Status,
			"delay_ms":        step.DelayMilliseconds,
			"rail_reference":  step.RailReference,
			"reason":          step.Reason,
			"duplicate_count": step.DuplicateCount,
		})
	}
	return map[string]any{
		"entity":                       "payout_simulator_scenario",
		"id":                           s.ID,
		"merchant_id":                  s.MerchantID,
		"settlement_id":                s.SettlementID,
		"transient_failures_remaining": s.TransientFailuresRemaining,
		"notes":                        s.Notes,
		"steps":                        steps,
		"created_at":                   presentZeroTime(s.CreatedAt),
		"updated_at":                   presentZeroTime(s.UpdatedAt),
	}
}

func presentZeroTime(ts time.Time) any {
	if ts.IsZero() {
		return nil
	}
	return ts.Unix()
}
