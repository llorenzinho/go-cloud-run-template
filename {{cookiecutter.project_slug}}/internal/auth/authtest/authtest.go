// Package authtest signs JWTs for tests and local development (cmd/devtoken).
// The service binary never imports it: it holds private keys.
package authtest

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"

	"{{ cookiecutter.module_path }}/internal/auth"
)

// Issuer signs tokens like an identity provider.
type Issuer struct {
	Key crypto.Signer
	// Alg is the JWS algorithm: RS256, ES256 or EdDSA.
	Alg string
	// KID is the `kid` header and the key id in JWKS.
	KID      string
	Issuer   string
	Audience string
	// RolesClaim is the dotted path where Token writes the roles.
	RolesClaim string
}

// NewIssuer generates a key pair for alg (RS256, ES256 or EdDSA).
func NewIssuer(alg string) (*Issuer, error) {
	var (
		key crypto.Signer
		err error
	)
	switch alg {
	case "RS256":
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	case "ES256":
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "EdDSA":
		_, key, err = ed25519.GenerateKey(rand.Reader)
	default:
		return nil, fmt.Errorf("authtest: unsupported algorithm %q (RS256, ES256, EdDSA)", alg)
	}
	if err != nil {
		return nil, err
	}
	return &Issuer{
		Key: key, Alg: alg, KID: "test-key",
		Issuer: "https://issuer.test", Audience: "test-api", RolesClaim: "roles",
	}, nil
}

// LoadIssuer reads a PKCS#8 "PRIVATE KEY" PEM written by PrivateKeyPEM.
func LoadIssuer(privatePEM []byte) (*Issuer, error) {
	block, _ := pem.Decode(privatePEM)
	if block == nil {
		return nil, errors.New("authtest: no PEM block found")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("authtest: %w", err)
	}
	i := &Issuer{KID: "dev-key", Issuer: "https://issuer.test", Audience: "test-api", RolesClaim: "roles"}
	switch k := k.(type) {
	case *rsa.PrivateKey:
		i.Key, i.Alg = k, "RS256"
	case *ecdsa.PrivateKey:
		i.Key, i.Alg = k, "ES256"
	case ed25519.PrivateKey:
		i.Key, i.Alg = k, "EdDSA"
	default:
		return nil, fmt.Errorf("authtest: unsupported key %T", k)
	}
	return i, nil
}

// PrivateKeyPEM returns the private key as PKCS#8 PEM.
func (i *Issuer) PrivateKeyPEM() ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(i.Key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// PublicKeyPEM returns the public key as PKIX PEM (auth.static.public_key).
func (i *Issuer) PublicKeyPEM() ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(i.Key.Public())
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

// JWKS returns the public key as a JWK Set (serve it to test jwks mode).
func (i *Issuer) JWKS() ([]byte, error) {
	jwk, err := jwkset.NewJWKFromKey(i.Key.Public(), jwkset.JWKOptions{
		Metadata: jwkset.JWKMetadataOptions{KID: i.KID, ALG: jwkset.ALG(i.Alg), USE: jwkset.UseSig},
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(jwkset.JWKSMarshal{Keys: []jwkset.JWKMarshal{jwk.Marshal()}})
}

// Claims describe a token. Zero values get defaults: TTL 1h.
type Claims struct {
	Subject string
	Email   string
	Roles   []string
	TTL     time.Duration
	// Extra claims, overriding the standard ones (e.g. "aud", "iss", "exp").
	Extra map[string]any
	// Header entries, e.g. "typ": "at+jwt".
	Header map[string]any
}

// Token signs a JWT with the issuer's key.
func (i *Issuer) Token(c Claims) (string, error) {
	now := time.Now()
	ttl := c.TTL
	if ttl == 0 {
		ttl = time.Hour
	}
	claims := jwt.MapClaims{
		"iss": i.Issuer,
		"aud": i.Audience,
		"sub": c.Subject,
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	}
	if c.Email != "" {
		claims["email"] = c.Email
	}
	if i.RolesClaim != "" && c.Roles != nil {
		setPath(claims, i.RolesClaim, c.Roles)
	}
	maps.Copy(claims, c.Extra)

	method := jwt.GetSigningMethod(i.Alg)
	if method == nil {
		return "", fmt.Errorf("authtest: unknown algorithm %q", i.Alg)
	}
	t := jwt.NewWithClaims(method, claims)
	t.Header["kid"] = i.KID
	maps.Copy(t.Header, c.Header)
	return t.SignedString(i.Key)
}

// MustToken is Token for tests: it panics on error.
func (i *Issuer) MustToken(c Claims) string {
	s, err := i.Token(c)
	if err != nil {
		panic(err)
	}
	return s
}

// Verifier returns a static-mode verifier trusting this issuer.
func (i *Issuer) Verifier(ctx context.Context, opts ...auth.Option) (*auth.Verifier, error) {
	return auth.New(ctx, append([]auth.Option{
		auth.WithIssuer(i.Issuer),
		auth.WithAudiences(i.Audience),
		auth.WithAlgorithms(i.Alg),
		auth.WithRolesClaim(i.RolesClaim),
		auth.WithPublicKey(i.Key.Public()),
	}, opts...)...)
}

// setPath writes v at the dotted path of m, creating nested objects.
func setPath(m map[string]any, path string, v any) {
	parts := strings.Split(path, ".")
	for _, p := range parts[:len(parts)-1] {
		next, ok := m[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[parts[len(parts)-1]] = v
}
