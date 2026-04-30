package httpx

import "context"

type ctxKey string

const principalCtxKey ctxKey = "principal"

type Principal struct {
	MerchantID string
	KeyID      string
	UserID     string
	Email      string
	Role       string
	Scope      string
	AuthType   string
}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	v := ctx.Value(principalCtxKey)
	if v == nil {
		return Principal{}, false
	}
	p, ok := v.(Principal)
	return p, ok
}
