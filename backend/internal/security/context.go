// Package security owns authentication primitives: password hashing, JWT
// issuing/validation and the permission evaluator. It knows nothing about HTTP.
package security

import "context"

type ctxKey int

const (
	principalKey ctxKey = iota
	requestIDKey
	ipAddressKey
)

// Principal is the authenticated caller. It deliberately carries no "current
// company": §9 requires the company to travel per request, so a principal is
// valid across every company the user is authorised for.
type Principal struct {
	UserID   int64
	Username string
	TokenID  string
}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFrom returns the authenticated caller, if the request passed
// through AuthMiddleware.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

// UserIDFrom is the accessor the audit interceptor uses to stamp
// created_by / changed_by from the server side (§D4).
func UserIDFrom(ctx context.Context) (int64, bool) {
	p, ok := PrincipalFrom(ctx)
	if !ok || p.UserID == 0 {
		return 0, false
	}
	return p.UserID, true
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func WithIPAddress(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ipAddressKey, ip)
}

func IPAddressFrom(ctx context.Context) string {
	ip, _ := ctx.Value(ipAddressKey).(string)
	return ip
}
