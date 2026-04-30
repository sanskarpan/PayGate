package httpx

import "net/http"

type healthResponse struct {
	Status string `json:"status"`
}

func Healthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func Readyz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, healthResponse{Status: "ready"})
}
