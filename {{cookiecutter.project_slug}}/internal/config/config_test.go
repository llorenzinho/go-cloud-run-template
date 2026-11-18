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
{%- if cookiecutter.use_auth == 'y' %}
	assert.True(t, cfg.Auth.Enabled)
	assert.Equal(t, config.AuthModeStatic, cfg.Auth.Mode)
	assert.Equal(t, []string{"RS256"}, cfg.Auth.Algorithms)
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
{%- if cookiecutter.use_auth == 'y' %}

func TestAuthEnvOverrides(t *testing.T) {
	t.Setenv("{{ cookiecutter.env_prefix }}_AUTH_MODE", "jwks")
	t.Setenv("{{ cookiecutter.env_prefix }}_AUTH_JWKS_URL", "https://idp.example.com/keys")
	t.Setenv("{{ cookiecutter.env_prefix }}_AUTH_AUDIENCES", "a,b")
	cfg, err := config.Load(committed)
	require.NoError(t, err)
	assert.Equal(t, "jwks", cfg.Auth.Mode)
	assert.Equal(t, []string{"a", "b"}, cfg.Auth.Audiences)
}

func TestAuthValidation(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"jwks without url", map[string]string{"AUTH_MODE": "jwks"}, "auth.jwks.url: is required"},
		{"static without key", map[string]string{"AUTH_STATIC_PUBLIC_KEY_FILE": ""}, "auth.static: public_key or public_key_file"},
		{"no issuer", map[string]string{"AUTH_ISSUER": ""}, "auth.issuer: is required"},
		{"bad mode", map[string]string{"AUTH_MODE": "magic"}, "auth.mode"},
		{"symmetric alg", map[string]string{"AUTH_ALGORITHMS": "HS256"}, "auth.algorithms"},
		{"disabled outside dev", map[string]string{"AUTH_ENABLED": "false", "ENV": "prod"}, "auth.enabled: can be false only with env=dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv("{{ cookiecutter.env_prefix }}_"+k, v)
			}
			_, err := config.Load(committed)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}

	t.Run("disabled in dev", func(t *testing.T) {
		t.Setenv("{{ cookiecutter.env_prefix }}_AUTH_ENABLED", "false")
		cfg, err := config.Load(committed)
		require.NoError(t, err)
		assert.Equal(t, "dev", cfg.Auth.DevPrincipal.Subject)
	})
}
{%- endif %}
