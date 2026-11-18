package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
{%- if cookiecutter.__examples == 'y' %}
	"uuid"
{%- endif %}

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

{%- if cookiecutter.__examples == 'y' %}
	"{{ cookiecutter.module_path }}/internal/domain"
{%- endif %}
	"{{ cookiecutter.module_path }}/internal/health"
	ihttp "{{ cookiecutter.module_path }}/internal/http"
	"{{ cookiecutter.module_path }}/internal/http/handler"
{%- if cookiecutter.__examples == 'y' %}
	"{{ cookiecutter.module_path }}/internal/service"
{%- endif %}
)

// checker is a fake health.Checker.
type checker struct {
	name string
	err  error
}

func (c checker) Name() string                  { return c.name }
func (c checker) Check(context.Context) error { return c.err }

func newRouter(t *testing.T, checks ...health.Checker) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	opts := make([]health.Option, 0, len(checks))
	for _, c := range checks {
		opts = append(opts, health.WithChecker(c))
	}
	r, err := ihttp.NewRouter(ihttp.Deps{
		BasePath: "/api",
		Log:      log,
		Health:   handler.NewHealth(health.NewRegistry(opts...)),
{%- if cookiecutter.__examples == 'y' %}
		Items:    handler.NewItems(service.NewItemService(&memRepo{items: map[uuid.UUID]*domain.Item{}}, log)),
{%- endif %}
	})
	require.NoError(t, err)
	return r
}

func send(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	}
	return rec.Code, out
}

func TestHealthz(t *testing.T) {
	code, body := send(t, newRouter(t, checker{name: "postgres", err: errors.New("down")}), http.MethodGet, "/healthz", "")
	assert.Equal(t, http.StatusOK, code, "liveness ignores dependencies")
	assert.Equal(t, "up", body["status"])
}

func TestReadyz(t *testing.T) {
	code, body := send(t, newRouter(t), http.MethodGet, "/readyz", "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, []any{}, body["checks"])

	code, body = send(t, newRouter(t, checker{name: "postgres"}), http.MethodGet, "/readyz", "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "up", body["status"])

	code, body = send(t, newRouter(t, checker{name: "postgres"}, checker{name: "redis", err: errors.New("refused")}), http.MethodGet, "/readyz", "")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "down", body["status"])
	checks := body["checks"].([]any)
	require.Len(t, checks, 2)
	assert.Equal(t, "redis", checks[1].(map[string]any)["name"])
	assert.Equal(t, "refused", checks[1].(map[string]any)["error"])
}

func TestNoRoute(t *testing.T) {
	code, body := send(t, newRouter(t), http.MethodGet, "/nope", "")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, "not_found", body["error"].(map[string]any)["code"])
}
