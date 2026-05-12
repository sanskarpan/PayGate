package middleware

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type contextKey string

const requestIDContextKey contextKey = "request_id"

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

func RequestIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(requestIDContextKey)
	if v == nil {
		return "", false
	}
	requestID, ok := v.(string)
	return requestID, ok && requestID != ""
}

// ClientIPWithTrustedProxies returns the best-effort client IP for a request.
// X-Forwarded-For is only trusted when the direct peer is in trustedProxies.
func ClientIPWithTrustedProxies(r *http.Request, trustedProxies []netip.Prefix) string {
	remote := remoteAddrHost(r.RemoteAddr)
	remoteIP, err := netip.ParseAddr(remote)
	if err != nil {
		return remote
	}
	if len(trustedProxies) == 0 || !isTrustedProxy(remoteIP, trustedProxies) {
		return remoteIP.String()
	}

	forwarded := parseForwardedChain(r.Header.Get("X-Forwarded-For"))
	for i := len(forwarded) - 1; i >= 0; i-- {
		if !isTrustedProxy(forwarded[i], trustedProxies) {
			return forwarded[i].String()
		}
	}
	if len(forwarded) > 0 {
		return forwarded[0].String()
	}
	return remoteIP.String()
}

func remoteAddrHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return strings.TrimSpace(remoteAddr)
	}
	return strings.TrimSpace(host)
}

func parseForwardedChain(raw string) []netip.Addr {
	parts := strings.Split(raw, ",")
	addrs := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		addr, err := netip.ParseAddr(strings.TrimSpace(part))
		if err == nil {
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

func isTrustedProxy(ip netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}
