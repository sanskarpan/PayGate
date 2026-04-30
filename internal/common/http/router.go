package httpx

import (
	"net/http"

	"github.com/sanskarpan/PayGate/internal/common/middleware"
)

func NewRouter(base http.Handler) http.Handler {
	if base == nil {
		base = http.NewServeMux()
	}
	return middleware.RequestID(base)
}
