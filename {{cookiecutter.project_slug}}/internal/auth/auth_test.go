package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{ cookiecutter.module_path }}/internal/auth"
	"{{ cookiecutter.module_path }}/internal/auth/authtest"
)

func newIssuer(t *testing.T, alg string) *authtest.Issuer {
	t.Helper()
	iss, err := authtest.NewIssuer(alg)
	require.NoError(t, err)
	return iss
}

func verifier(t *testing.T, iss *authtest.Issuer, opts ...auth.Option) *auth.Verifier {
	t.Helper()
	v, err := iss.Verifier(t.Context(), opts...)
	require.NoError(t, err)
	t.Cleanup(v.Shutdown)
	return v
}

func TestStaticKeyAlgorithms(t *testing.T) {
	for _, alg := range []string{"RS256", "ES256", "EdDSA"} {
		t.Run(alg, func(t *testing.T) {
			iss := newIssuer(t, alg)
			v := verifier(t, iss)
			p, err := v.Authenticate(t.Context(), iss.MustToken(authtest.Claims{
				Subject: "alice", Email: "alice@example.com", Roles: []string{"admin"},
			}))
			require.NoError(t, err)
			assert.Equal(t, "alice", p.Subject)
			assert.Equal(t, iss.Issuer, p.Issuer)
			assert.Equal(t, "alice@example.com", p.Email)
			assert.True(t, p.HasRole("admin"))
			assert.Equal(t, "alice", p.Claims["sub"])
		})
	}
}

func TestRejectedTokens(t *testing.T) {
	iss := newIssuer(t, "RS256")
	other := newIssuer(t, "RS256")
	v := verifier(t, iss, auth.WithClockSkew(0))

	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": iss.Issuer, "aud": iss.Audience, "sub": "x", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	})
	hsToken, err := hs.SignedString([]byte("secret"))
	require.NoError(t, err)
	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"iss": iss.Issuer, "aud": iss.Audience, "sub": "x", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	})
	noneToken, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	tests := map[string]string{
		"missing":       "",
		"garbage":       "not.a.jwt",
		"other key":     other.MustToken(authtest.Claims{Subject: "x"}),
		"wrong issuer":  iss.MustToken(authtest.Claims{Subject: "x", Extra: map[string]any{"iss": "https://evil"}}),
		"wrong aud":     iss.MustToken(authtest.Claims{Subject: "x", Extra: map[string]any{"aud": "other-api"}}),
		"expired":       iss.MustToken(authtest.Claims{Subject: "x", TTL: -time.Minute}),
		"no exp":        iss.MustToken(authtest.Claims{Subject: "x", Extra: map[string]any{"exp": nil}}),
		"not yet valid": iss.MustToken(authtest.Claims{Subject: "x", Extra: map[string]any{"nbf": time.Now().Add(time.Hour).Unix()}}),
		"no sub":        iss.MustToken(authtest.Claims{}),
		"HS256":         hsToken,
		"alg none":      noneToken,
	}
	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := v.Authenticate(t.Context(), token)
			require.ErrorIs(t, err, auth.ErrUnauthenticated)
		})
	}
	_, err = v.Authenticate(t.Context(), "")
	require.ErrorIs(t, err, auth.ErrMissingToken)
}

func TestAudienceList(t *testing.T) {
	iss := newIssuer(t, "RS256")
	v := verifier(t, iss, auth.WithAudiences("other-api", iss.Audience))
	_, err := v.Authenticate(t.Context(), iss.MustToken(authtest.Claims{
		Subject: "x", Extra: map[string]any{"aud": []string{"zitadel-client", iss.Audience}},
	}))
	require.NoError(t, err)
}

func TestAlgorithmAllowlist(t *testing.T) {
	iss := newIssuer(t, "ES256")
	v := verifier(t, iss, auth.WithAlgorithms("RS256"))
	_, err := v.Authenticate(t.Context(), iss.MustToken(authtest.Claims{Subject: "x"}))
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
}

func TestTokenTypes(t *testing.T) {
	iss := newIssuer(t, "RS256")
	v := verifier(t, iss, auth.WithTokenTypes("at+jwt"))
	_, err := v.Authenticate(t.Context(), iss.MustToken(authtest.Claims{Subject: "x"}))
	require.ErrorIs(t, err, auth.ErrUnauthenticated, "typ JWT is not at+jwt")
	_, err = v.Authenticate(t.Context(), iss.MustToken(authtest.Claims{
		Subject: "x", Header: map[string]any{"typ": "application/AT+JWT"},
	}))
	require.NoError(t, err)
}

func TestRolesClaimShapes(t *testing.T) {
	iss := newIssuer(t, "RS256")
	tests := []struct {
		name, path string
		extra      map[string]any
		want       []string
	}{
		{"firebase array", "roles", map[string]any{"roles": []string{"admin", "viewer"}}, []string{"admin", "viewer"}},
		{"zitadel object", "urn:zitadel:iam:org:project:roles", map[string]any{
			"urn:zitadel:iam:org:project:roles": map[string]any{"viewer": map[string]any{"1": "org"}, "admin": map[string]any{}},
		}, []string{"admin", "viewer"}},
		{"nested path", "app.roles", map[string]any{"app": map[string]any{"roles": []string{"ops"}}}, []string{"ops"}},
		{"single string", "role", map[string]any{"role": "admin"}, []string{"admin"}},
		{"missing", "roles", nil, []string{}},
		{"no claim configured", "", map[string]any{"roles": []string{"admin"}}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := verifier(t, iss, auth.WithRolesClaim(tt.path))
			p, err := v.Authenticate(t.Context(), iss.MustToken(authtest.Claims{Subject: "x", Extra: tt.extra}))
			require.NoError(t, err)
			assert.Equal(t, tt.want, p.Roles)
		})
	}

	v := verifier(t, iss, auth.WithRolesClaim("roles"))
	_, err := v.Authenticate(t.Context(), iss.MustToken(authtest.Claims{Subject: "x", Extra: map[string]any{"roles": 42}}))
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
}

func TestNewValidates(t *testing.T) {
	iss := newIssuer(t, "RS256")
	_, err := auth.New(t.Context(), auth.WithPublicKey(iss.Key.Public()))
	require.ErrorContains(t, err, "issuer is required")
	require.ErrorContains(t, err, "audience")

	_, err = auth.New(t.Context(), auth.WithIssuer("i"), auth.WithAudiences("a"))
	require.ErrorContains(t, err, "exactly one of")

	_, err = auth.New(t.Context(), auth.WithIssuer("i"), auth.WithAudiences("a"),
		auth.WithPublicKey(iss.Key.Public()), auth.WithAlgorithms("HS256"))
	require.ErrorContains(t, err, `"HS256" is not supported`)
}

func TestParsePublicKeyPEM(t *testing.T) {
	iss := newIssuer(t, "RS256")
	pub, err := iss.PublicKeyPEM()
	require.NoError(t, err)
	key, err := auth.ParsePublicKeyPEM(pub)
	require.NoError(t, err)
	assert.IsType(t, &rsa.PublicKey{}, key)

	// X.509 certificate (e.g. a Firebase / Google x509 key).
	rsaKey := iss.Key.(*rsa.PrivateKey)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"}, NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &rsaKey.PublicKey, rsaKey)
	require.NoError(t, err)
	key, err = auth.ParsePublicKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	require.NoError(t, err)
	assert.Equal(t, &rsaKey.PublicKey, key)

	priv, err := iss.PrivateKeyPEM()
	require.NoError(t, err)
	_, err = auth.ParsePublicKeyPEM(priv)
	require.ErrorContains(t, err, "private key")

	_, err = auth.ParsePublicKeyPEM([]byte("nope"))
	require.ErrorContains(t, err, "no PEM block")
}

// jwksServer serves iss's JWKS; down makes it answer 503.
func jwksServer(t *testing.T, iss *authtest.Issuer, down *atomic.Bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		body, err := iss.JWKS()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func jwksVerifier(t *testing.T, iss *authtest.Issuer, url string, opts ...auth.Option) *auth.Verifier {
	t.Helper()
	v, err := auth.New(t.Context(), append([]auth.Option{
		auth.WithIssuer(iss.Issuer),
		auth.WithAudiences(iss.Audience),
		auth.WithAlgorithms(iss.Alg),
		auth.WithJWKS(url),
	}, opts...)...)
	require.NoError(t, err)
	t.Cleanup(v.Shutdown)
	return v
}

func TestJWKS(t *testing.T) {
	iss := newIssuer(t, "RS256")
	var down atomic.Bool
	srv, _ := jwksServer(t, iss, &down)
	v := jwksVerifier(t, iss, srv.URL)

	require.NotNil(t, v.ReadinessCheck())
	assert.Equal(t, "auth_jwks", v.ReadinessCheck().Name())
	require.NoError(t, v.ReadinessCheck().Check(t.Context()))

	p, err := v.Authenticate(t.Context(), iss.MustToken(authtest.Claims{Subject: "alice"}))
	require.NoError(t, err)
	assert.Equal(t, "alice", p.Subject)

	static := verifier(t, iss)
	assert.Nil(t, static.ReadinessCheck(), "static mode has no readiness check")
}

func TestJWKSUnavailableAtStartup(t *testing.T) {
	iss := newIssuer(t, "RS256")
	var down atomic.Bool
	down.Store(true)
	srv, _ := jwksServer(t, iss, &down)
	v := jwksVerifier(t, iss, srv.URL, auth.WithUnknownKIDInterval(0))

	require.Error(t, v.ReadinessCheck().Check(t.Context()), "not ready before the first fetch")
	_, err := v.Authenticate(t.Context(), iss.MustToken(authtest.Claims{Subject: "x"}))
	require.ErrorIs(t, err, auth.ErrUnauthenticated)

	down.Store(false)
	require.NoError(t, v.ReadinessCheck().Check(t.Context()))
	_, err = v.Authenticate(t.Context(), iss.MustToken(authtest.Claims{Subject: "x"}))
	require.NoError(t, err)
}

func TestJWKSKeyRotation(t *testing.T) {
	iss := newIssuer(t, "RS256")
	var down atomic.Bool
	srv, hits := jwksServer(t, iss, &down)
	v := jwksVerifier(t, iss, srv.URL, auth.WithUnknownKIDInterval(time.Hour))
	before := hits.Load()

	// Same key, unknown kid: rate limited, no refresh within the interval.
	_, err := v.Authenticate(t.Context(), iss.MustToken(authtest.Claims{Subject: "x", Header: map[string]any{"kid": "rotated"}}))
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
	assert.Equal(t, before, hits.Load())

	// Without rate limit the unknown kid triggers a refresh.
	v2 := jwksVerifier(t, iss, srv.URL, auth.WithUnknownKIDInterval(0))
	before = hits.Load()
	_, _ = v2.Authenticate(t.Context(), iss.MustToken(authtest.Claims{Subject: "x", Header: map[string]any{"kid": "rotated"}}))
	assert.Greater(t, hits.Load(), before)
}

func TestDevAndContext(t *testing.T) {
	d := auth.Dev{Principal: auth.Principal{Subject: "dev", Roles: []string{"admin"}}}
	p, err := d.Authenticate(context.Background(), "")
	require.NoError(t, err)
	assert.True(t, p.HasAnyRole("viewer", "admin"))
	assert.False(t, p.HasAnyRole("viewer"))

	_, ok := auth.FromContext(t.Context())
	assert.False(t, ok)
	got, ok := auth.FromContext(auth.WithPrincipal(t.Context(), p))
	require.True(t, ok)
	assert.Equal(t, "dev", got.Subject)
}
