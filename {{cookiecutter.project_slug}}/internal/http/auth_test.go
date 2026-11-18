package http_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{ cookiecutter.module_path }}/internal/auth"
	"{{ cookiecutter.module_path }}/internal/auth/authtest"
)

func TestAPIRequiresToken(t *testing.T) {
	r := newRouter(t)

	code, body, hdr := sendAs(t, r, "", http.MethodGet, "/api/v1/me", "")
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, "unauthenticated", body["error"].(map[string]any)["code"])
	assert.Equal(t, "Bearer", hdr.Get("WWW-Authenticate"))

	expired := issuer.MustToken(authtest.Claims{Subject: "x", TTL: -time.Hour})
	code, body, hdr = sendAs(t, r, expired, http.MethodGet, "/api/v1/me", "")
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, `Bearer error="invalid_token"`, hdr.Get("WWW-Authenticate"))
	assert.Equal(t, "authentication required", body["error"].(map[string]any)["message"], "no detail for the client")

	code, _, _ = sendAs(t, r, "", http.MethodGet, "/healthz", "")
	assert.Equal(t, http.StatusOK, code, "probes stay public")
	code, _, _ = sendAs(t, r, "", http.MethodGet, "/api/v1/nope", "")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestMe(t *testing.T) {
	token := issuer.MustToken(authtest.Claims{Subject: "alice", Email: "alice@example.com", Roles: []string{"viewer"}})
	code, body, _ := sendAs(t, newRouter(t), token, http.MethodGet, "/api/v1/me", "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "alice", body["sub"])
	assert.Equal(t, issuer.Issuer, body["iss"])
	assert.Equal(t, "alice@example.com", body["email"])
	assert.Equal(t, []any{"viewer"}, body["roles"])
}

func TestDevAuthenticator(t *testing.T) {
	r := newRouterWith(t, auth.Dev{Principal: auth.Principal{Subject: "dev", Roles: []string{"admin"}}})
	code, body, _ := sendAs(t, r, "", http.MethodGet, "/api/v1/me", "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "dev", body["sub"])
}
{%- if cookiecutter.__examples == 'y' %}

func TestPublicRoute(t *testing.T) {
	code, body, _ := sendAs(t, newRouter(t), "", http.MethodGet, "/api/v1/public/ping", "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok", body["status"])
}

func TestItemWritesRequireAdmin(t *testing.T) {
	r := newRouter(t)
	viewer := issuer.MustToken(authtest.Claims{Subject: "bob", Roles: []string{"viewer"}})

	code, body, _ := sendAs(t, r, viewer, http.MethodPost, "/api/v1/items", `{"name":"x"}`)
	assert.Equal(t, http.StatusForbidden, code)
	assert.Equal(t, "forbidden", body["error"].(map[string]any)["code"])

	// 400: past authentication and role checks, stopped by validation (no database here).
	code, _, _ = sendAs(t, r, viewer, http.MethodGet, "/api/v1/items/nope", "")
	assert.Equal(t, http.StatusBadRequest, code, "reads need a token only")

	code, _, _ = sendAs(t, r, "", http.MethodGet, "/api/v1/items", "")
	assert.Equal(t, http.StatusUnauthorized, code)

	code, _, _ = sendAs(t, r, adminToken, http.MethodPost, "/api/v1/items", `{"name":" "}`)
	assert.Equal(t, http.StatusBadRequest, code)
}
{%- endif %}
