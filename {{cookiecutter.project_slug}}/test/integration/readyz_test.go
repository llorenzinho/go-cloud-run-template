//go:build integration

// Package integration runs the HTTP API against real dependencies started
// with testcontainers.
package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{ cookiecutter.module_path }}/internal/health"
	ihttp "{{ cookiecutter.module_path }}/internal/http"
	"{{ cookiecutter.module_path }}/internal/http/handler"
{%- if cookiecutter.use_postgres == 'y' %}
	"{{ cookiecutter.module_path }}/internal/postgres"
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	"{{ cookiecutter.module_path }}/internal/redis"
{%- endif %}
	"{{ cookiecutter.module_path }}/internal/testutil"
)

// stack holds the dependencies started for a test.
type stack struct {
	Log *slog.Logger
{%- if cookiecutter.use_postgres == 'y' %}
	DB  *postgres.DB
{%- endif %}
}

// startServer boots the router against the real dependencies; mount adds
// handlers to the router dependencies (e.g. the example ones).
func startServer(t *testing.T, mount ...func(*ihttp.Deps, stack)) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	st := stack{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	checks := []health.Option{}
{%- if cookiecutter.use_postgres == 'y' %}

	st.DB, _ = testutil.Postgres(t)
	checks = append(checks, health.WithChecker(st.DB))
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}

	rdb, err := redis.New(context.Background(), testutil.Redis(t), redis.WithPoolSize(4))
	require.NoError(t, err)
	t.Cleanup(func() { _ = rdb.Shutdown() })
	checks = append(checks, health.WithChecker(rdb))
{%- endif %}

	d := ihttp.Deps{
		BasePath: "{{ cookiecutter.base_path }}",
		Log:      st.Log,
		Health:   handler.NewHealth(health.NewRegistry(checks...)),
	}
	for _, f := range mount {
		f(&d, st)
	}
	router, err := ihttp.NewRouter(d)
	require.NoError(t, err)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestReadyz(t *testing.T) {
	base := startServer(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/readyz", nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()

	var rep health.Report
	require.NoError(t, json.NewDecoder(res.Body).Decode(&rep))
	assert.Equal(t, http.StatusOK, res.StatusCode, rep)
	assert.Equal(t, health.StatusUp, rep.Status)
	names := make([]string, 0, len(rep.Checks))
	for _, c := range rep.Checks {
		names = append(names, c.Name)
	}
	assert.Equal(t, []string{ {%- if cookiecutter.use_postgres == 'y' %}"postgres"{% endif %}{% if cookiecutter.use_postgres == 'y' and cookiecutter.use_redis == 'y' %}, {% endif %}{% if cookiecutter.use_redis == 'y' %}"redis"{% endif %}}, names)
}
