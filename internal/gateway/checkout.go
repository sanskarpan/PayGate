package gateway

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/tokenization"
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

const checkoutPageHTML = `<html><body><h1>PayGate Checkout</h1>
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

const upiIntentHTML = `<html><body><h1>Complete UPI Payment</h1>
<p>Order: {{.OrderID}}</p>
<p>Payment ID: {{.PaymentID}}</p>
<p>Status: {{.Status}}</p>
<p><a href="{{.DeepLink}}">Open UPI App</a></p>
<p>Return after payment to: <a href="{{.CallbackURL}}">{{.CallbackURL}}</a></p>
</body></html>`

type CheckoutHandler struct {
	paymentSvc    *payment.Service
	orderSvc      *order.Service
	cardTokenizer CardTokenizer
	checkoutTmpl  *template.Template
	upiTmpl       *template.Template
	sandboxVPA    string
	sandboxCard   string
}

type CardTokenizer interface {
	CreateCardToken(ctx context.Context, in tokenization.CreateCardTokenInput) (tokenization.CardToken, error)
}

func NewCheckoutHandler(paymentSvc *payment.Service, orderSvc *order.Service, opts ...func(*CheckoutHandler)) *CheckoutHandler {
	h := &CheckoutHandler{
		paymentSvc:   paymentSvc,
		orderSvc:     orderSvc,
		sandboxVPA:   "customer@upi",
		sandboxCard:  "4111111111111111",
	}
	h.checkoutTmpl = template.Must(template.New("checkout").Parse(checkoutPageHTML))
	h.upiTmpl = template.Must(template.New("upi-intent").Parse(upiIntentHTML))
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func WithCardTokenizer(tokenizer CardTokenizer) func(*CheckoutHandler) {
	return func(h *CheckoutHandler) {
		h.cardTokenizer = tokenizer
	}
}

func WithSandboxVPA(vpa string) func(*CheckoutHandler) {
	return func(h *CheckoutHandler) {
		if vpa != "" {
			h.sandboxVPA = vpa
		}
	}
}

func WithSandboxCard(card string) func(*CheckoutHandler) {
	return func(h *CheckoutHandler) {
		if card != "" {
			h.sandboxCard = card
		}
	}
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

	_ = h.checkoutTmpl.Execute(w, map[string]string{
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

	if method == string(MethodUPI) {
		intent, upiErr := h.paymentSvc.CreateUPIIntent(r.Context(), payment.CreateUPIIntentInput{
			MerchantID:       merchantID,
			OrderID:          orderID,
			Amount:           o.AmountDue,
			Currency:         o.Currency,
			VPA:              h.sandboxVPA,
			ExpiresInSeconds: 300,
		})
		if upiErr != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: upiErr.Error()})
			return
		}
		_ = h.upiTmpl.Execute(w, map[string]string{
			"OrderID":     orderID,
			"PaymentID":   intent.PaymentID,
			"Status":      string(intent.Status),
			"DeepLink":    intent.DeepLink,
			"CallbackURL": callbackURL,
		})
		return
	}
	authInput := payment.AuthorizeInput{
		MerchantID:  merchantID,
		OrderID:     orderID,
		Amount:      o.AmountDue,
		Currency:    o.Currency,
		Method:      method,
		AutoCapture: true,
	}
	if method == string(MethodCard) && h.cardTokenizer != nil {
		expires := time.Now().UTC().AddDate(3, 0, 0)
		token, tokenErr := h.cardTokenizer.CreateCardToken(r.Context(), tokenization.CreateCardTokenInput{
			MerchantID: merchantID,
			CardNumber: h.sandboxCard,
			ExpMonth:   int(expires.Month()),
			ExpYear:    expires.Year(),
		})
		if tokenErr != nil {
			httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "sandbox card tokenization failed"})
			return
		}
		authInput.PaymentMethodTokenID = token.ID
	}
	out, err := h.paymentSvc.Authorize(r.Context(), authInput)
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
