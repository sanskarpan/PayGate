package auth

import (
	"net/http"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/merchant"
)

// Action represents a fine-grained permission that may be checked beyond
// the coarse read/write/admin API key scope already enforced by the middleware.
type Action string

const (
	// Order actions
	ActionOrderRead   Action = "order:read"
	ActionOrderCreate Action = "order:create"

	// Payment actions
	ActionPaymentRead    Action = "payment:read"
	ActionPaymentCapture Action = "payment:capture"

	// Refund actions
	ActionRefundRead   Action = "refund:read"
	ActionRefundCreate Action = "refund:create"

	// Webhook actions
	ActionWebhookRead   Action = "webhook:read"
	ActionWebhookWrite  Action = "webhook:write"
	ActionWebhookDelete Action = "webhook:delete"
	ActionWebhookReplay Action = "webhook:replay"

	// Settlement actions
	ActionSettlementRead Action = "settlement:read"

	// Reconciliation actions
	ActionReconRead Action = "recon:read"

	// API key actions
	ActionAPIKeyRead   Action = "apikey:read"
	ActionAPIKeyCreate Action = "apikey:create"
	ActionAPIKeyRevoke Action = "apikey:revoke"
	ActionAPIKeyRotate Action = "apikey:rotate"

	// Team management actions (admin only)
	ActionTeamInvite  Action = "team:invite"
	ActionTeamManage  Action = "team:manage"

	// Audit log access
	ActionAuditRead Action = "audit:read"

	// Risk / manual review
	ActionRiskRead   Action = "risk:read"
	ActionRiskReview Action = "risk:review"
)

// permissions is the role → allowed actions table.
// admin has all permissions; developer has read+write for payments/webhooks;
// readonly can only read; ops can read everything + approve risk reviews.
var permissions = map[merchant.MerchantUserRole]map[Action]bool{
	merchant.MerchantUserRoleAdmin: {
		ActionOrderRead:   true,
		ActionOrderCreate: true,

		ActionPaymentRead:    true,
		ActionPaymentCapture: true,

		ActionRefundRead:   true,
		ActionRefundCreate: true,

		ActionWebhookRead:   true,
		ActionWebhookWrite:  true,
		ActionWebhookDelete: true,
		ActionWebhookReplay: true,

		ActionSettlementRead: true,

		ActionReconRead: true,

		ActionAPIKeyRead:   true,
		ActionAPIKeyCreate: true,
		ActionAPIKeyRevoke: true,
		ActionAPIKeyRotate: true,

		ActionTeamInvite: true,
		ActionTeamManage: true,

		ActionAuditRead: true,

		ActionRiskRead:   true,
		ActionRiskReview: true,
	},
	merchant.MerchantUserRoleDeveloper: {
		ActionOrderRead:   true,
		ActionOrderCreate: true,

		ActionPaymentRead:    true,
		ActionPaymentCapture: true,

		ActionRefundRead:   true,
		ActionRefundCreate: true,

		ActionWebhookRead:   true,
		ActionWebhookWrite:  true,
		ActionWebhookDelete: true,
		ActionWebhookReplay: true,

		ActionSettlementRead: true,
		ActionReconRead:      true,

		ActionAPIKeyRead:   true,
		ActionAPIKeyCreate: true,
		ActionAPIKeyRevoke: true,
		ActionAPIKeyRotate: true,

		ActionAuditRead: true,
		ActionRiskRead:  true,
	},
	merchant.MerchantUserRoleOps: {
		ActionOrderRead:   true,
		ActionPaymentRead: true,
		ActionRefundRead:  true,

		ActionWebhookRead:   true,
		ActionWebhookReplay: true,

		ActionSettlementRead: true,
		ActionReconRead:      true,

		ActionAPIKeyRead: true,

		ActionAuditRead: true,

		ActionRiskRead:   true,
		ActionRiskReview: true,
	},
	merchant.MerchantUserRoleReadonly: {
		ActionOrderRead:      true,
		ActionPaymentRead:    true,
		ActionRefundRead:     true,
		ActionWebhookRead:    true,
		ActionSettlementRead: true,
		ActionReconRead:      true,
		ActionAPIKeyRead:     true,
		ActionAuditRead:      true,
		ActionRiskRead:       true,
	},
}

// RoleAllows returns true when the given role is allowed to perform action.
// API-key-only principals (no role) are not checked here — they rely solely on
// the coarse scope check in RequireScope.
func RoleAllows(role merchant.MerchantUserRole, action Action) bool {
	return permissions[role][action]
}

// RequireAction returns a middleware that checks the dashboard session role
// against a fine-grained action.  API key requests bypass this check because
// they are already gated by scope in RequireScope; adding an action layer on
// top would break existing integrations.
func (m *Middleware) RequireAction(action Action, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := httpx.PrincipalFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{
				Code:        "UNAUTHORIZED",
				Description: "missing authentication",
				Source:      "auth",
				Step:        "authorization",
				Reason:      "unauthenticated",
			})
			return
		}

		// API key principals skip the role-action check.
		if p.AuthType == "api_key" {
			next.ServeHTTP(w, r)
			return
		}

		role := merchant.MerchantUserRole(p.Role)
		if !RoleAllows(role, action) {
			httpx.WriteError(w, http.StatusForbidden, httpx.APIError{
				Code:        "FORBIDDEN",
				Description: "your role does not permit this action",
				Source:      "auth",
				Step:        "authorization",
				Reason:      "insufficient_role",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
