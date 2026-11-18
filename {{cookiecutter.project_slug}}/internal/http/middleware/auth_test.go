package middleware_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"{{ cookiecutter.module_path }}/internal/auth"
	"{{ cookiecutter.module_path }}/internal/http/middleware"
)

// fakeAuth accepts the token "good" and records what it received.
type fakeAuth struct{ got *string }

func (f fakeAuth) Authenticate(_ context.Context, token string) (*auth.Principal, error) {
	*f.got = token
	if token == "" {
		return nil, auth.ErrMissingToken
	}
	if token != "good" {
		return nil, auth.ErrUnauthenticated
	}
	return &auth.Principal{Subject: "u", Roles: []string{"viewer"}}, nil
}

func engine(got *string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Errors())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	v1 := r.Group("/api/v1", middleware.Authenticate(fakeAuth{got: got}, log, "/api/v1/public"))
	ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	v1.GET("/x", ok)
	v1.GET("/publicity", ok) // not under /public: protected
	v1.GET("/public/ping", ok)
	v1.GET("/admin", middleware.RequireRole("admin"), ok)
	return r
}

func call(r http.Handler, path, authz string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestAuthenticateHeader(t *testing.T) {
	var got string
	r := engine(&got)
	tests := []struct {
		authz, token string
		want         int
	}{
		{"Bearer good", "good", http.StatusNoContent},
		{"bearer   good ", "good", http.StatusNoContent},
		{"Bearer bad", "bad", http.StatusUnauthorized},
		{"Basic dXNlcjpwdw==", "", http.StatusUnauthorized},
		{"Bearer", "", http.StatusUnauthorized},
		{"", "", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.authz, func(t *testing.T) {
			got = "unset"
			rec := call(r, "/api/v1/x", tt.authz)
			assert.Equal(t, tt.want, rec.Code)
			assert.Equal(t, tt.token, got)
			if tt.want == http.StatusUnauthorized {
				assert.NotEmpty(t, rec.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

func TestPublicPrefix(t *testing.T) {
	var got string
	r := engine(&got)
	assert.Equal(t, http.StatusNoContent, call(r, "/api/v1/public/ping", "").Code)
	assert.Equal(t, http.StatusUnauthorized, call(r, "/api/v1/publicity", "").Code, "prefix match is per path segment")
}

func TestRequireRole(t *testing.T) {
	var got string
	r := engine(&got)
	assert.Equal(t, http.StatusForbidden, call(r, "/api/v1/admin", "Bearer good").Code)
}
