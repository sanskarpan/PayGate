package gateway

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
)

// safeCallbackURL validates that the provided URL is a safe relative path to
// prevent open redirect attacks. Only paths starting with "/" are accepted;
// absolute URLs with a host are rejected. Returns the default fallback if the
// input is empty or unsafe.
func safeCallbackURL(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host != "" || u.Scheme != "" {
		return fallback
	}
	if !strings.HasPrefix(u.Path, "/") {
		return fallback
	}
	return raw
}

type CheckoutHandler struct {
	paymentSvc *payment.Service
	orderSvc   *order.Service
}

func NewCheckoutHandler(paymentSvc *payment.Service, orderSvc *order.Service) *CheckoutHandler {
	return &CheckoutHandler{paymentSvc: paymentSvc, orderSvc: orderSvc}
}

func (h *CheckoutHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /checkout", h.checkoutPage)
	mux.HandleFunc("POST /checkout/pay", h.pay)
	mux.HandleFunc("GET /checkout/callback", h.callback)
}

func (h *CheckoutHandler) checkoutPage(w http.ResponseWriter, r *http.Request) {
	orderID := r.URL.Query().Get("order_id")
	merchantID := r.URL.Query().Get("merchant_id")
	callbackURL := safeCallbackURL(r.URL.Query().Get("callback_url"), "/checkout/callback")

	o, err := h.orderSvc.GetByID(r.Context(), merchantID, orderID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: "order not found"})
		return
	}
	if o.Status == order.StateExpired || time.Now().UTC().After(o.ExpiresAt) {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "order is expired"})
		return
	}

	page := `<html><body><h1>PayGate Checkout</h1>
<form method="POST" action="/checkout/pay">
<input type="hidden" name="order_id" value="{{.OrderID}}" />
<input type="hidden" name="merchant_id" value="{{.MerchantID}}" />
<input type="hidden" name="callback_url" value="{{.CallbackURL}}" />
<input type="hidden" name="amount" value="{{.Amount}}" />
<input type="hidden" name="currency" value="{{.Currency}}" />
<p>Amount: {{.AmountDisplay}}</p>
<p>Currency: {{.Currency}}</p>
<label>Method</label><select name="method"><option>card</option><option>upi</option></select>
<button type="submit">Pay</button>
</form></body></html>`
	t := template.Must(template.New("checkout").Parse(page))
	_ = t.Execute(w, map[string]string{
		"OrderID":       orderID,
		"MerchantID":    merchantID,
		"CallbackURL":   callbackURL,
		"Amount":        fmt.Sprintf("%d", o.AmountDue),
		"AmountDisplay": fmt.Sprintf("%d", o.AmountDue),
		"Currency":      o.Currency,
	})
}

func (h *CheckoutHandler) pay(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid form"})
		return
	}
	orderID := r.FormValue("order_id")
	merchantID := r.FormValue("merchant_id")
	callbackURL := safeCallbackURL(r.FormValue("callback_url"), "/checkout/callback")
	method := r.FormValue("method")

	if orderID == "" || merchantID == "" {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "order_id and merchant_id are required"})
		return
	}

	o, err := h.orderSvc.GetByID(r.Context(), merchantID, orderID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: "order not found"})
		return
	}
	if o.Status == order.StateExpired || time.Now().UTC().After(o.ExpiresAt) {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "order is expired"})
		return
	}
	if o.Status == order.StatePaid {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "order is already paid"})
		return
	}

	out, err := h.paymentSvc.Authorize(r.Context(), payment.AuthorizeInput{
		MerchantID:  merchantID,
		OrderID:     orderID,
		Amount:      o.AmountDue,
		Currency:    o.Currency,
		Method:      method,
		AutoCapture: true,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error()})
		return
	}

	redirectTo := fmt.Sprintf("%s?payment_id=%s&status=%s&ts=%d", callbackURL, out.PaymentID, out.Status, time.Now().Unix())
	http.Redirect(w, r, redirectTo, http.StatusFound)
}

func (h *CheckoutHandler) callback(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"payment_id": r.URL.Query().Get("payment_id"),
		"status":     r.URL.Query().Get("status"),
	})
}
