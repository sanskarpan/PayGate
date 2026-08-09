package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// bs is a single backslash. Escape sequences are assembled from it so this
// file never contains a literal NUL or a pre-formed unicode escape.
const bs = `\`

func nullByteHandler() (http.Handler, *bool) {
	reached := false
	h := RejectNullBytes(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	return h, &reached
}

func postBody(t *testing.T, body string) (int, bool) {
	t.Helper()
	h, reached := nullByteHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/orders", strings.NewReader(body)))
	return rec.Code, *reached
}

func TestRejectNullBytesBlocksRawNullInBody(t *testing.T) {
	code, reached := postBody(t, "{\"receipt\":\"a\x00b\"}")
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", code)
	}
	if reached {
		t.Fatal("handler must not run for a body containing a raw NUL")
	}
}

// An odd-length backslash run means the escape is live and decodes to NUL.
func TestRejectNullBytesBlocksLiveUnicodeEscape(t *testing.T) {
	for _, body := range []string{
		`{"receipt":"a` + bs + `u0000b"}`,
		`{"receipt":"a` + bs + `U0000b"}`,
		`{"receipt":"` + bs + bs + bs + `u0000"}`,
	} {
		code, reached := postBody(t, body)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", body, code)
		}
		if reached {
			t.Fatalf("handler must not run for %s", body)
		}
	}
}

// An even-length run means the backslashes escape each other, leaving the
// literal text "u0000". That is valid input and must be accepted.
func TestRejectNullBytesAllowsEscapedBackslashText(t *testing.T) {
	for _, body := range []string{
		`{"receipt":"a` + bs + bs + `u0000b"}`,
		`{"receipt":"` + bs + bs + bs + bs + `u0000"}`,
		`{"receipt":"plain u0000"}`,
		`{"receipt":"café ✓ 日本"}`,
	} {
		code, reached := postBody(t, body)
		if code != http.StatusOK || !reached {
			t.Fatalf("expected %s to be accepted, got %d (reached=%v)", body, code, reached)
		}
	}
}

// RawQuery keeps percent-encoding, so %00 has to be caught before net/url
// decodes it into a NUL.
func TestRejectNullBytesBlocksPercentEncodedNullInQuery(t *testing.T) {
	for _, target := range []string{
		"/v1/orders?receipt=%00",
		"/v1/orders?a=b&c=%00d",
		"/v1/orders?receipt=%00",
	} {
		h, reached := nullByteHandler()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", target, rec.Code)
		}
		if *reached {
			t.Fatalf("handler must not run for %s", target)
		}
	}
}

func TestRejectNullBytesBlocksNullInPath(t *testing.T) {
	h, reached := nullByteHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/orders/abc%00def", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if *reached {
		t.Fatal("handler must not run for a NUL in the path")
	}
}

func TestRejectNullBytesPassesCleanRequestsThrough(t *testing.T) {
	h, reached := nullByteHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/orders?count=25", strings.NewReader(`{"amount":1000,"currency":"INR"}`)))
	if rec.Code != http.StatusOK || !*reached {
		t.Fatalf("expected a clean request to pass, got %d (reached=%v)", rec.Code, *reached)
	}
}

func TestRejectNullBytesReturnsAStructuredError(t *testing.T) {
	h, _ := nullByteHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/orders?a=%00", nil))

	var body struct {
		Error struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error.Code != "BAD_REQUEST_ERROR" || body.Error.Description == "" {
		t.Fatalf("unexpected error envelope: %+v", body.Error)
	}
}
