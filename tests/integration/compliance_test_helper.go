//go:build integration

package integration

import (
	"net/http"
	"testing"
)

func approveMerchantOnboarding(t *testing.T, mux *http.ServeMux, authHeader string) {
	sendJSON(t, mux, http.MethodPut, "/v1/merchants/me/onboarding", authHeader, map[string]any{
		"legal_name":              "Compliant Merchant Private Limited",
		"business_classification": "private_limited",
		"registration_number":     "U12345KA2026PTC100001",
		"tax_identifier":          "29ABCDE1234F1Z9",
		"country_code":            "IN",
	}, http.StatusOK)
	sendJSON(t, mux, http.MethodPut, "/v1/merchants/me/onboarding/parties", authHeader, map[string]any{
		"items": []map[string]any{
			{
				"party_type":          "beneficial_owner",
				"full_name":           "Owner One",
				"ownership_bps":       6000,
				"verification_status": "verified",
			},
			{
				"party_type":          "controller",
				"full_name":           "Controller One",
				"verification_status": "verified",
			},
		},
	}, http.StatusOK)
	doc := sendJSON(t, mux, http.MethodPost, "/v1/merchants/me/onboarding/documents/request", authHeader, map[string]any{
		"document_type":  "certificate_of_incorporation",
		"request_reason": "required",
	}, http.StatusCreated)
	docID := mustString(t, doc, "id")
	sendJSON(t, mux, http.MethodPost, "/v1/merchants/me/onboarding/documents", authHeader, map[string]any{
		"document_id":   docID,
		"document_type": "certificate_of_incorporation",
		"file_name":     "coi.pdf",
		"content_type":  "application/pdf",
		"storage_key":   "merchant/coi.pdf",
	}, http.StatusCreated)
	sendJSON(t, mux, http.MethodPost, "/v1/merchants/me/onboarding/documents/"+docID+"/review", authHeader, map[string]any{
		"status":       "approved",
		"review_notes": "looks good",
	}, http.StatusOK)
	sendJSON(t, mux, http.MethodPost, "/v1/merchants/me/onboarding/screenings/run", authHeader, map[string]any{
		"screening_type": "kyb",
		"force_result":   "passed",
	}, http.StatusCreated)
	sendJSON(t, mux, http.MethodPost, "/v1/merchants/me/onboarding/submit", authHeader, map[string]any{}, http.StatusOK)
	sendJSON(t, mux, http.MethodPost, "/v1/merchants/me/onboarding/review", authHeader, map[string]any{
		"merchant_id":    merchantIDFromAuth(t, mux, authHeader),
		"state":          "approved",
		"reviewer_notes": "approved",
	}, http.StatusOK)
}

func createApprovedBeneficiary(t *testing.T, mux *http.ServeMux, authHeader string) string {
	resp := sendJSON(t, mux, http.MethodPost, "/v1/beneficiaries", authHeader, map[string]any{
		"destination_type":    "bank_account",
		"account_holder_name": "Settlement Beneficiary",
		"bank_account_number": "4111111111111234",
		"bank_ifsc":           "HDFC0001234",
	}, http.StatusCreated)
	beneficiaryID := mustString(t, resp, "id")
	sendJSON(t, mux, http.MethodPost, "/v1/beneficiaries/"+beneficiaryID+"/verify", authHeader, map[string]any{}, http.StatusOK)
	sendJSON(t, mux, http.MethodPost, "/v1/beneficiaries/"+beneficiaryID+"/approve", authHeader, map[string]any{
		"notes": "approved for payouts",
	}, http.StatusOK)
	return beneficiaryID
}

func merchantIDFromAuth(t *testing.T, mux *http.ServeMux, authHeader string) string {
	resp := sendJSON(t, mux, http.MethodGet, "/v1/merchants/me", authHeader, nil, http.StatusOK)
	return mustString(t, resp, "merchant_id")
}
