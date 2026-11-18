//go:build integration

// Package testutil starts throw-away dependencies for integration tests.
package testutil

import (
	"context"
{%- if cookiecutter.use_postgres == 'y' %}
	"io"
	"log/slog"
	"strconv"
{%- endif %}
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
{%- if cookiecutter.use_postgres == 'y' %}
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
{%- endif %}
{%- if cookiecutter.use_postgres == 'y' %}

	"{{ cookiecutter.module_path }}/db"
	"{{ cookiecutter.module_path }}/internal/postgres"
{%- endif %}
)

// Images used by integration tests; keep in sync with docker-compose.yml.
const (
{%- if cookiecutter.use_postgres == 'y' %}
	PostgresImage = "postgres:18.7-alpine"
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	RedisImage = "redis:8-alpine"
{%- endif %}
)
{%- if cookiecutter.use_postgres == 'y' %}

// Postgres starts Postgres, applies the migrations and returns the pool
// (service schema, default timeouts) and its connection settings.
func Postgres(t *testing.T, opts ...postgres.Option) (*postgres.DB, postgres.Conn) {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, PostgresImage,
		tcpostgres.WithDatabase("test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	host, err := ctr.Host(ctx)
	require.NoError(t, err)
	port, err := ctr.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	p, err := strconv.Atoi(port.Port())
	require.NoError(t, err)
	conn := postgres.Conn{Host: host, Port: p, User: "test", Password: "test", Database: "test", SSLMode: "disable"}

	database, err := postgres.New(ctx, conn, append([]postgres.Option{postgres.WithSchema(db.Schema)}, opts...)...)
	require.NoError(t, err)
	t.Cleanup(database.Shutdown)

	require.NoError(t, database.Migrate(ctx, "up", slog.New(slog.NewTextHandler(io.Discard, nil))))
	return database, conn
}
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}

// Redis starts Redis and returns its URL.
func Redis(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcredis.Run(ctx, RedisImage)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	url, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)
	return url
}
{%- endif %}
