package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/payment"
)

const (
	ProviderSimulatorPrimary  = "simulator_primary"
	ProviderSimulatorFailover = "simulator_failover"
	ProviderSimulatorSaver    = "simulator_cost_saver"
)

var ErrRoutingPolicyNotFound = errors.New("routing policy not found")

type ProviderDefinition struct {
	Name             string
	SupportedMethods map[PaymentMethod]bool
	BaseCostBPS      int
	SuccessScore     int
}

type RoutingPolicy struct {
	ID                string
	MerchantID        string
	Method            PaymentMethod
	PrimaryProvider   string
	FallbackProvider  string
	ForceProvider     string
	FailoverOnDecline bool
	FailoverOnError   bool
	CostWeight        int
	SuccessWeight     int
	CreatedAt         time.Time
}

type RoutingPolicyStore struct {
	db *pgxpool.Pool
}

func NewRoutingPolicyStore(db *pgxpool.Pool) *RoutingPolicyStore {
	return &RoutingPolicyStore{db: db}
}

func (s *RoutingPolicyStore) Upsert(ctx context.Context, policy RoutingPolicy) (RoutingPolicy, error) {
	if policy.ID == "" {
		policy.ID = idgen.New("grp")
	}
	if policy.CostWeight <= 0 {
		policy.CostWeight = 50
	}
	if policy.SuccessWeight <= 0 {
		policy.SuccessWeight = 50
	}
	_, err := s.db.Exec(ctx, `
INSERT INTO paygate_gateway.routing_policies
    (id, merchant_id, method, primary_provider, fallback_provider, force_provider, failover_on_decline, failover_on_error, cost_weight, success_weight)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (merchant_id, method) DO UPDATE
SET primary_provider = EXCLUDED.primary_provider,
    fallback_provider = EXCLUDED.fallback_provider,
    force_provider = EXCLUDED.force_provider,
    failover_on_decline = EXCLUDED.failover_on_decline,
    failover_on_error = EXCLUDED.failover_on_error,
    cost_weight = EXCLUDED.cost_weight,
    success_weight = EXCLUDED.success_weight,
    updated_at = NOW()
`, policy.ID, policy.MerchantID, policy.Method, policy.PrimaryProvider, policy.FallbackProvider, policy.ForceProvider, policy.FailoverOnDecline, policy.FailoverOnError, policy.CostWeight, policy.SuccessWeight)
	if err != nil {
		return RoutingPolicy{}, fmt.Errorf("upsert routing policy: %w", err)
	}
	return s.Get(ctx, policy.MerchantID, string(policy.Method))
}

func (s *RoutingPolicyStore) Get(ctx context.Context, merchantID, method string) (RoutingPolicy, error) {
	var policy RoutingPolicy
	err := s.db.QueryRow(ctx, `
SELECT id, merchant_id, method, primary_provider, fallback_provider, force_provider, failover_on_decline, failover_on_error, cost_weight, success_weight, created_at
FROM paygate_gateway.routing_policies
WHERE merchant_id = $1 AND method = $2
`, merchantID, method).Scan(
		&policy.ID, &policy.MerchantID, &policy.Method, &policy.PrimaryProvider, &policy.FallbackProvider, &policy.ForceProvider,
		&policy.FailoverOnDecline, &policy.FailoverOnError, &policy.CostWeight, &policy.SuccessWeight, &policy.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return RoutingPolicy{}, ErrRoutingPolicyNotFound
	}
	if err != nil {
		return RoutingPolicy{}, fmt.Errorf("get routing policy: %w", err)
	}
	return policy, nil
}

func (s *RoutingPolicyStore) List(ctx context.Context) ([]RoutingPolicy, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, merchant_id, method, primary_provider, fallback_provider, force_provider, failover_on_decline, failover_on_error, cost_weight, success_weight, created_at
FROM paygate_gateway.routing_policies
ORDER BY created_at DESC
LIMIT 200
`)
	if err != nil {
		return nil, fmt.Errorf("list routing policies: %w", err)
	}
	defer rows.Close()

	var out []RoutingPolicy
	for rows.Next() {
		var policy RoutingPolicy
		if err := rows.Scan(
			&policy.ID, &policy.MerchantID, &policy.Method, &policy.PrimaryProvider, &policy.FallbackProvider, &policy.ForceProvider,
			&policy.FailoverOnDecline, &policy.FailoverOnError, &policy.CostWeight, &policy.SuccessWeight, &policy.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, policy)
	}
	return out, rows.Err()
}

type providerGateway interface {
	AuthorizeWithProvider(ctx context.Context, provider string, amount int64, currency, merchantID, method string) (payment.GatewayAuthResult, error)
}

type Router struct {
	store     *RoutingPolicyStore
	gateway   providerGateway
	providers map[string]ProviderDefinition
}

func NewRouter(store *RoutingPolicyStore, gateway providerGateway) *Router {
	return &Router{
		store:   store,
		gateway: gateway,
		providers: map[string]ProviderDefinition{
			ProviderSimulatorPrimary: {
				Name:             ProviderSimulatorPrimary,
				SupportedMethods: supportedMethodSet(),
				BaseCostBPS:      100,
				SuccessScore:     92,
			},
			ProviderSimulatorFailover: {
				Name:             ProviderSimulatorFailover,
				SupportedMethods: supportedMethodSet(),
				BaseCostBPS:      135,
				SuccessScore:     99,
			},
			ProviderSimulatorSaver: {
				Name:             ProviderSimulatorSaver,
				SupportedMethods: supportedMethodSet(),
				BaseCostBPS:      65,
				SuccessScore:     84,
			},
		},
	}
}

func supportedMethodSet() map[PaymentMethod]bool {
	return map[PaymentMethod]bool{
		MethodCard:       true,
		MethodUPI:        true,
		MethodNetbanking: true,
		MethodWallet:     true,
	}
}

func (r *Router) Name() string {
	return "gateway_router"
}

func (r *Router) CreateUPIIntent(ctx context.Context, in payment.GatewayUPIIntentCreateInput) (payment.GatewayUPIIntentResult, error) {
	upi, ok := r.gateway.(payment.UPIGateway)
	if !ok {
		return payment.GatewayUPIIntentResult{}, errors.New("upi intent gateway is not configured")
	}
	return upi.CreateUPIIntent(ctx, in)
}

func (r *Router) PollUPIIntent(ctx context.Context, in payment.GatewayUPIIntentPollInput) (payment.GatewayUPIIntentStatus, error) {
	upi, ok := r.gateway.(payment.UPIGateway)
	if !ok {
		return payment.GatewayUPIIntentStatus{}, errors.New("upi intent gateway is not configured")
	}
	return upi.PollUPIIntent(ctx, in)
}

func (r *Router) CreateRedirectSession(ctx context.Context, in payment.GatewayRedirectCreateInput) (payment.GatewayRedirectResult, error) {
	redirect, ok := r.gateway.(payment.RedirectGateway)
	if !ok {
		return payment.GatewayRedirectResult{}, errors.New("redirect gateway is not configured")
	}
	return redirect.CreateRedirectSession(ctx, in)
}

func (r *Router) PollRedirectSession(ctx context.Context, in payment.GatewayRedirectPollInput) (payment.GatewayRedirectStatus, error) {
	redirect, ok := r.gateway.(payment.RedirectGateway)
	if !ok {
		return payment.GatewayRedirectStatus{}, errors.New("redirect gateway is not configured")
	}
	return redirect.PollRedirectSession(ctx, in)
}

func (r *Router) Authorize(ctx context.Context, amount int64, currency, merchantID, method string) (payment.GatewayAuthResult, error) {
	result, _, err := r.AuthorizeWithRouting(ctx, amount, currency, merchantID, method)
	return result, err
}

func (r *Router) AuthorizeWithRouting(ctx context.Context, amount int64, currency, merchantID, method string) (payment.GatewayAuthResult, payment.GatewayRouteDecision, error) {
	policy, err := r.policyFor(ctx, merchantID, method)
	if err != nil {
		return payment.GatewayAuthResult{}, payment.GatewayRouteDecision{}, err
	}
	primary, reason := r.selectPrimary(policy, method)
	if primary == "" {
		primary = ProviderSimulatorPrimary
	}
	result, err := r.gateway.AuthorizeWithProvider(ctx, primary, amount, currency, merchantID, method)
	decision := payment.GatewayRouteDecision{
		Provider:           primary,
		PrimaryProvider:    primary,
		RoutingReason:      reason,
		AttemptedProviders: []string{primary},
	}
	if err == nil && (result.Success || result.RequiresAction || policy.FallbackProvider == "" || policy.FallbackProvider == primary) {
		return result, decision, err
	}
	if err != nil && !policy.FailoverOnError {
		return result, decision, err
	}
	if err == nil && !result.Success && !result.RequiresAction && !policy.FailoverOnDecline {
		return result, decision, err
	}
	fallback := strings.TrimSpace(policy.FallbackProvider)
	if fallback == "" || fallback == primary {
		return result, decision, err
	}
	fallbackResult, fallbackErr := r.gateway.AuthorizeWithProvider(ctx, fallback, amount, currency, merchantID, method)
	decision.FallbackUsed = true
	decision.Provider = fallback
	decision.RoutingReason = reason + "+failover"
	decision.AttemptedProviders = append(decision.AttemptedProviders, fallback)
	if fallbackErr != nil {
		if err != nil {
			return fallbackResult, decision, fallbackErr
		}
		return result, decision, nil
	}
	return fallbackResult, decision, nil
}

func (r *Router) policyFor(ctx context.Context, merchantID, method string) (RoutingPolicy, error) {
	if r.store == nil || merchantID == "" {
		return defaultRoutingPolicy(method), nil
	}
	policy, err := r.store.Get(ctx, merchantID, method)
	if err == nil {
		return policy, nil
	}
	if errors.Is(err, ErrRoutingPolicyNotFound) {
		return defaultRoutingPolicy(method), nil
	}
	return RoutingPolicy{}, err
}

func defaultRoutingPolicy(method string) RoutingPolicy {
	return RoutingPolicy{
		Method:           PaymentMethod(method),
		PrimaryProvider:  ProviderSimulatorPrimary,
		FallbackProvider: ProviderSimulatorFailover,
		// An issuer decline is a business decision — insufficient funds,
		// suspected fraud, a stolen card — not a provider outage. Re-presenting
		// it to a second acquirer to force an approval is contrary to card
		// scheme rules and reads as a duplicate authorization attempt, so it is
		// off unless a merchant opts in on their own routing policy.
		FailoverOnDecline: false,
		// A transport error or timeout genuinely is an outage, and failing over
		// is the reason a fallback provider exists.
		FailoverOnError: true,
		CostWeight:      40,
		SuccessWeight:   60,
	}
}

func (r *Router) selectPrimary(policy RoutingPolicy, method string) (string, string) {
	if force := strings.TrimSpace(policy.ForceProvider); force != "" {
		if r.supports(force, method) {
			return force, "forced_provider"
		}
	}
	primary := strings.TrimSpace(policy.PrimaryProvider)
	fallback := strings.TrimSpace(policy.FallbackProvider)
	if primary == "" {
		primary = ProviderSimulatorPrimary
	}
	if !r.supports(primary, method) {
		primary = ProviderSimulatorPrimary
	}
	if fallback == "" || !r.supports(fallback, method) {
		return primary, "policy_primary"
	}
	primaryScore := r.routeScore(primary, policy)
	fallbackScore := r.routeScore(fallback, policy)
	if fallbackScore > primaryScore {
		return fallback, "weighted_score"
	}
	return primary, "policy_primary"
}

func (r *Router) supports(provider, method string) bool {
	def, ok := r.providers[provider]
	if !ok {
		return false
	}
	return def.SupportedMethods[PaymentMethod(method)]
}

func (r *Router) routeScore(provider string, policy RoutingPolicy) int {
	def := r.providers[provider]
	return def.SuccessScore*policy.SuccessWeight - def.BaseCostBPS*policy.CostWeight
}
