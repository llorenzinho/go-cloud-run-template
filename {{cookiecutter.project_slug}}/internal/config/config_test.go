package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{ cookiecutter.module_path }}/internal/config"
)

const committed = "../../configs/config.yaml"

func TestLoadCommittedConfig(t *testing.T) {
	cfg, err := config.Load(committed)
	require.NoError(t, err)
	assert.Equal(t, "dev", cfg.Env)
	assert.Equal(t, ":8080", cfg.HTTP.Addr)
	assert.Equal(t, "{{ cookiecutter.base_path }}", cfg.HTTP.BasePath)
	assert.Equal(t, 15*time.Second, cfg.HTTP.ShutdownTimeout)
	assert.Equal(t, 30*time.Second, cfg.HTTP.RequestTimeout)
{%- if cookiecutter.use_postgres == 'y' %}
	assert.Equal(t, "{{ cookiecutter.db_schema }}", cfg.DB.Schema)
	assert.EqualValues(t, 5, cfg.DB.Pool.MaxConns)
	assert.True(t, cfg.DB.AutoMigrate)
{%- endif %}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("{{ cookiecutter.env_prefix }}_LOG_FORMAT", "json")
{%- if cookiecutter.use_postgres == 'y' %}
	t.Setenv("{{ cookiecutter.env_prefix }}_DB_POOL_MAX_CONNS", "3")
	t.Setenv("{{ cookiecutter.env_prefix }}_DB_TIMEOUTS_QUERY", "2s")
{%- endif %}
	t.Setenv("{{ cookiecutter.env_prefix }}_HTTP_TRUSTED_PROXIES", "10.0.0.0/8,192.168.1.1")

	cfg, err := config.Load(committed)
	require.NoError(t, err)
	assert.Equal(t, "json", cfg.Log.Format)
{%- if cookiecutter.use_postgres == 'y' %}
	assert.EqualValues(t, 3, cfg.DB.Pool.MaxConns)
	assert.Equal(t, 2*time.Second, cfg.DB.Timeouts.Query)
{%- endif %}
	assert.Equal(t, []string{"10.0.0.0/8", "192.168.1.1"}, cfg.HTTP.TrustedProxies)
}

func TestConfigEnvSelectsFile(t *testing.T) {
	data, err := os.ReadFile(committed)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "other.yaml")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	t.Setenv("{{ cookiecutter.env_prefix }}_CONFIG", path)
	t.Setenv("{{ cookiecutter.env_prefix }}_ENV", "prod")

	cfg, err := config.Load("")
	require.NoError(t, err)
	assert.Equal(t, "prod", cfg.Env)
}

func TestValidation(t *testing.T) {
	t.Setenv("{{ cookiecutter.env_prefix }}_LOG_LEVEL", "verbose")
{%- if cookiecutter.use_postgres == 'y' %}
	t.Setenv("{{ cookiecutter.env_prefix }}_DB_HOST", "")
	t.Setenv("{{ cookiecutter.env_prefix }}_DB_POOL_MIN_CONNS", "9")
{%- endif %}

	_, err := config.Load(committed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log.level: must be one of")
{%- if cookiecutter.use_postgres == 'y' %}
	assert.Contains(t, err.Error(), "db.host: is required")
	assert.Contains(t, err.Error(), "db.pool.min_conns: must be less than or equal to max_conns")
{%- endif %}
}
{%- if cookiecutter.use_postgres == 'y' %}

func TestSchemaIsFixed(t *testing.T) {
	t.Setenv("{{ cookiecutter.env_prefix }}_DB_SCHEMA", "other")
	_, err := config.Load(committed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.schema")
}
{%- endif %}
