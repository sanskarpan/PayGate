package httpx

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code        string         `json:"code"`
	Description string         `json:"description"`
	Field       string         `json:"field,omitempty"`
	Source      string         `json:"source,omitempty"`
	Step        string         `json:"step,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func WriteError(w http.ResponseWriter, status int, apiErr APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: apiErr})
}
