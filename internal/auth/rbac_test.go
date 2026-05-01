package auth

import (
	"testing"

	"github.com/sanskarpan/PayGate/internal/merchant"
)

func TestRoleAllows(t *testing.T) {
	tests := []struct {
		role    merchant.MerchantUserRole
		action  Action
		allowed bool
	}{
		// admin — all actions
		{merchant.MerchantUserRoleAdmin, ActionOrderCreate, true},
		{merchant.MerchantUserRoleAdmin, ActionPaymentCapture, true},
		{merchant.MerchantUserRoleAdmin, ActionRefundCreate, true},
		{merchant.MerchantUserRoleAdmin, ActionWebhookDelete, true},
		{merchant.MerchantUserRoleAdmin, ActionTeamInvite, true},
		{merchant.MerchantUserRoleAdmin, ActionAuditRead, true},
		{merchant.MerchantUserRoleAdmin, ActionRiskReview, true},
		{merchant.MerchantUserRoleAdmin, ActionAPIKeyRevoke, true},

		// developer — no team management, no risk review
		{merchant.MerchantUserRoleDeveloper, ActionOrderCreate, true},
		{merchant.MerchantUserRoleDeveloper, ActionPaymentCapture, true},
		{merchant.MerchantUserRoleDeveloper, ActionRefundCreate, true},
		{merchant.MerchantUserRoleDeveloper, ActionWebhookWrite, true},
		{merchant.MerchantUserRoleDeveloper, ActionAPIKeyCreate, true},
		{merchant.MerchantUserRoleDeveloper, ActionTeamInvite, false},
		{merchant.MerchantUserRoleDeveloper, ActionTeamManage, false},
		{merchant.MerchantUserRoleDeveloper, ActionRiskReview, false},

		// ops — can read everything + replay webhooks + risk review, cannot create orders/payments/refunds
		{merchant.MerchantUserRoleOps, ActionOrderRead, true},
		{merchant.MerchantUserRoleOps, ActionPaymentRead, true},
		{merchant.MerchantUserRoleOps, ActionRefundRead, true},
		{merchant.MerchantUserRoleOps, ActionWebhookReplay, true},
		{merchant.MerchantUserRoleOps, ActionRiskReview, true},
		{merchant.MerchantUserRoleOps, ActionAuditRead, true},
		{merchant.MerchantUserRoleOps, ActionOrderCreate, false},
		{merchant.MerchantUserRoleOps, ActionPaymentCapture, false},
		{merchant.MerchantUserRoleOps, ActionRefundCreate, false},
		{merchant.MerchantUserRoleOps, ActionWebhookWrite, false},
		{merchant.MerchantUserRoleOps, ActionAPIKeyCreate, false},
		{merchant.MerchantUserRoleOps, ActionTeamInvite, false},

		// readonly — read only, nothing mutating
		{merchant.MerchantUserRoleReadonly, ActionOrderRead, true},
		{merchant.MerchantUserRoleReadonly, ActionPaymentRead, true},
		{merchant.MerchantUserRoleReadonly, ActionRefundRead, true},
		{merchant.MerchantUserRoleReadonly, ActionWebhookRead, true},
		{merchant.MerchantUserRoleReadonly, ActionSettlementRead, true},
		{merchant.MerchantUserRoleReadonly, ActionReconRead, true},
		{merchant.MerchantUserRoleReadonly, ActionAuditRead, true},
		{merchant.MerchantUserRoleReadonly, ActionRiskRead, true},
		{merchant.MerchantUserRoleReadonly, ActionOrderCreate, false},
		{merchant.MerchantUserRoleReadonly, ActionPaymentCapture, false},
		{merchant.MerchantUserRoleReadonly, ActionRefundCreate, false},
		{merchant.MerchantUserRoleReadonly, ActionWebhookWrite, false},
		{merchant.MerchantUserRoleReadonly, ActionWebhookDelete, false},
		{merchant.MerchantUserRoleReadonly, ActionTeamInvite, false},
		{merchant.MerchantUserRoleReadonly, ActionRiskReview, false},
		{merchant.MerchantUserRoleReadonly, ActionAPIKeyRevoke, false},
	}

	for _, tc := range tests {
		got := RoleAllows(tc.role, tc.action)
		if got != tc.allowed {
			t.Errorf("RoleAllows(%q, %q) = %v, want %v", tc.role, tc.action, got, tc.allowed)
		}
	}
}
