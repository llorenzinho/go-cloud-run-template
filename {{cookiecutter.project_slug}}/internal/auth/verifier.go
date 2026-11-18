package auth

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Defaults of New.
const (
	DefaultClockSkew          = 30 * time.Second
	DefaultJWKSFetchTimeout   = 5 * time.Second
	DefaultJWKSRefresh        = time.Hour
	DefaultUnknownKIDInterval = 5 * time.Minute
)

// SupportedAlgorithms are the accepted JWS algorithms: asymmetric only. `none`
// and HS* (a secret shared with the issuer) are always rejected.
var SupportedAlgorithms = []string{
	"RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA",
}

// Verifier validates Bearer JWTs. It is safe for concurrent use.
type Verifier struct {
	issuer     string
	audiences  []string
	algorithms []string
	clockSkew  time.Duration
	rolesClaim string
	tokenTypes []string

	publicKey crypto.PublicKey

	jwksURL            string
	jwksFetchTimeout   time.Duration
	jwksRefresh        time.Duration
	unknownKIDInterval time.Duration
	httpClient         *http.Client
	log                *slog.Logger

	jwks    *jwksCache
	keyfunc jwt.Keyfunc
	parser  *jwt.Parser
	cancel  context.CancelFunc
}

var _ Authenticator = (*Verifier)(nil)

// Option configures New.
type Option func(*Verifier)

// WithIssuer sets the required `iss` (exact match).
func WithIssuer(iss string) Option { return func(v *Verifier) { v.issuer = iss } }

// WithAudiences sets the accepted audiences: `aud` must contain one of them.
func WithAudiences(aud ...string) Option {
	return func(v *Verifier) { v.audiences = slices.Clone(aud) }
}

// WithAlgorithms sets the allowlist of JWS algorithms (default RS256).
func WithAlgorithms(algs ...string) Option {
	return func(v *Verifier) { v.algorithms = slices.Clone(algs) }
}

// WithClockSkew sets the tolerance on exp, nbf and iat (default 30s).
func WithClockSkew(d time.Duration) Option { return func(v *Verifier) { v.clockSkew = d } }

// WithRolesClaim sets the dotted path of the roles claim: an array of strings
// (Firebase custom claim "roles") or an object whose keys are the roles
// (ZITADEL "urn:zitadel:iam:org:project:roles"). Empty = no roles.
func WithRolesClaim(path string) Option { return func(v *Verifier) { v.rolesClaim = path } }

// WithTokenTypes restricts the `typ` header (e.g. "at+jwt"), compared without
// case and "application/" prefix. Empty = not checked.
func WithTokenTypes(types ...string) Option {
	return func(v *Verifier) { v.tokenTypes = slices.Clone(types) }
}

// WithPublicKey verifies every token with key (static mode).
func WithPublicKey(key crypto.PublicKey) Option { return func(v *Verifier) { v.publicKey = key } }

// WithJWKS verifies tokens with the key of their `kid` from the JWKS at url (jwks mode).
func WithJWKS(url string) Option { return func(v *Verifier) { v.jwksURL = url } }

// WithJWKSFetchTimeout bounds each JWKS request (default 5s).
func WithJWKSFetchTimeout(d time.Duration) Option {
	return func(v *Verifier) { v.jwksFetchTimeout = d }
}

// WithJWKSRefresh sets the background refresh interval of the JWKS (default 1h).
func WithJWKSRefresh(d time.Duration) Option { return func(v *Verifier) { v.jwksRefresh = d } }

// WithUnknownKIDInterval is the minimum interval between JWKS refreshes
// triggered by an unknown `kid` (default 5m).
func WithUnknownKIDInterval(d time.Duration) Option {
	return func(v *Verifier) { v.unknownKIDInterval = d }
}

// WithHTTPClient sets the client of the JWKS requests.
func WithHTTPClient(c *http.Client) Option { return func(v *Verifier) { v.httpClient = c } }

// WithLogger sets the logger of the JWKS refresh errors (default slog.Default()).
func WithLogger(l *slog.Logger) Option { return func(v *Verifier) { v.log = l } }

// New returns a verifier. In jwks mode it fetches the keys once (a failure is
// logged, not returned: a cold start must survive a blip of the IdP) and
// refreshes them in the background until ctx is done or Shutdown is called.
func New(ctx context.Context, opts ...Option) (*Verifier, error) {
	v := &Verifier{
		algorithms:         []string{"RS256"},
		clockSkew:          DefaultClockSkew,
		jwksFetchTimeout:   DefaultJWKSFetchTimeout,
		jwksRefresh:        DefaultJWKSRefresh,
		unknownKIDInterval: DefaultUnknownKIDInterval,
		httpClient:         http.DefaultClient,
		log:                slog.Default(),
	}
	for _, opt := range opts {
		opt(v)
	}
	if err := v.validate(); err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	bg, cancel := context.WithCancel(ctx)
	v.cancel = cancel
	if v.jwksURL != "" {
		c, err := newJWKSCache(bg, v)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("auth: %w", err)
		}
		v.jwks = c
		v.keyfunc = c.keyfunc
	} else {
		key := v.publicKey
		v.keyfunc = func(*jwt.Token) (any, error) { return key, nil }
	}

	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods(v.algorithms),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audiences...),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(v.clockSkew),
	}
	v.parser = jwt.NewParser(parserOpts...)
	return v, nil
}

func (v *Verifier) validate() error {
	var errs []error
	if v.issuer == "" {
		errs = append(errs, errors.New("issuer is required"))
	}
	if len(v.audiences) == 0 || slices.Contains(v.audiences, "") {
		errs = append(errs, errors.New("at least one non-empty audience is required"))
	}
	if len(v.algorithms) == 0 {
		errs = append(errs, errors.New("at least one algorithm is required"))
	}
	for _, a := range v.algorithms {
		if !slices.Contains(SupportedAlgorithms, a) {
			errs = append(errs, fmt.Errorf("algorithm %q is not supported (asymmetric only: %s)", a, strings.Join(SupportedAlgorithms, ", ")))
		}
	}
	if (v.publicKey == nil) == (v.jwksURL == "") {
		errs = append(errs, errors.New("set exactly one of a public key or a JWKS URL"))
	}
	if v.clockSkew < 0 {
		errs = append(errs, errors.New("clock skew must be >= 0"))
	}
	return errors.Join(errs...)
}

// Authenticate verifies token and returns its principal. Errors wrap
// ErrUnauthenticated and say why (for logs only).
func (v *Verifier) Authenticate(_ context.Context, token string) (*Principal, error) {
	if token == "" {
		return nil, ErrMissingToken
	}
	claims := jwt.MapClaims{}
	parsed, err := v.parser.ParseWithClaims(token, claims, v.keyfunc)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnauthenticated, err)
	}
	// Signature, alg, iss, aud, exp, nbf and iat are valid from here on.
	if err := v.checkType(parsed.Header); err != nil {
		return nil, err
	}

	p := &Principal{Claims: claims}
	p.Subject, _ = claims.GetSubject()
	if p.Subject == "" {
		return nil, fmt.Errorf("%w: missing sub claim", ErrUnauthenticated)
	}
	p.Issuer, _ = claims.GetIssuer()
	p.Email, _ = claims["email"].(string)
	if p.Roles, err = roles(claims, v.rolesClaim); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnauthenticated, err)
	}
	return p, nil
}

func (v *Verifier) checkType(header map[string]any) error {
	if len(v.tokenTypes) == 0 {
		return nil
	}
	norm := func(s string) string { return strings.TrimPrefix(strings.ToLower(s), "application/") }
	typ, _ := header["typ"].(string)
	if typ != "" && slices.ContainsFunc(v.tokenTypes, func(t string) bool { return norm(t) == norm(typ) }) {
		return nil
	}
	return fmt.Errorf("%w: typ header %q not accepted", ErrUnauthenticated, typ)
}

// ReadinessCheck returns the JWKS readiness check (name "auth_jwks"), nil in
// static mode. It is down until the keys have been fetched once.
func (v *Verifier) ReadinessCheck() interface {
	Name() string
	Check(ctx context.Context) error
} {
	if v.jwks == nil {
		return nil
	}
	return v.jwks
}

// Shutdown stops the background JWKS refresh (implements do.Shutdowner).
func (v *Verifier) Shutdown() { v.cancel() }
