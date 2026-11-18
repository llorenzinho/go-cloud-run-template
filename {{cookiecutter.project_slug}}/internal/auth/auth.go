// Package auth authenticates API callers with Bearer JWTs.
//
// A Verifier checks the signature with a key from a JWKS URL (Firebase,
// ZITADEL, any OIDC provider) or with a static PEM public key, then the
// claims: iss, aud, exp/nbf/iat, sub and an optional `typ` allowlist. The
// resulting Principal travels in the request context (FromContext).
//
// The package is independent of the HTTP framework: the gin middleware is in
// internal/http/middleware.
package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// Errors returned by Authenticate. Every failure wraps ErrUnauthenticated (401);
// the specific cause is for logs only, never for the client.
var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrMissingToken    = fmt.Errorf("%w: missing bearer token", ErrUnauthenticated)
	// ErrForbidden is returned by authorization checks (403).
	ErrForbidden = errors.New("forbidden")
)

// Principal is the authenticated caller.
type Principal struct {
	Subject string   `json:"sub"`
	Issuer  string   `json:"iss"`
	Email   string   `json:"email,omitempty"`
	Roles   []string `json:"roles"`
	// Claims are every claim of the token, for provider specific data
	// (e.g. Firebase "firebase.sign_in_provider").
	Claims map[string]any `json:"-"`
}

// HasRole reports whether the principal has role.
func (p *Principal) HasRole(role string) bool {
	return p != nil && slices.Contains(p.Roles, role)
}

// HasAnyRole reports whether the principal has at least one of roles.
func (p *Principal) HasAnyRole(roles ...string) bool {
	return slices.ContainsFunc(roles, p.HasRole)
}

// Authenticator turns the raw Bearer token of a request ("" when absent)
// into a Principal.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*Principal, error)
}

type ctxKey struct{}

// WithPrincipal returns a copy of ctx carrying p.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns the principal of the request, if authenticated.
func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(*Principal)
	return p, ok && p != nil
}

// Dev authenticates every request as a fixed principal, without looking at
// the token. Only for auth.enabled=false in local development.
type Dev struct {
	Principal Principal
}

// Authenticate returns a copy of the configured principal.
func (d Dev) Authenticate(context.Context, string) (*Principal, error) {
	p := d.Principal
	p.Roles = slices.Clone(p.Roles)
	return &p, nil
}
