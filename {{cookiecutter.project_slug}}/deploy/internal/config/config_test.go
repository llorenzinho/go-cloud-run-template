package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{ cookiecutter.module_path }}/deploy/internal/config"
)

{%- if cookiecutter.use_postgres == 'y' %}
const dbPassword = "s3cr3t-pw"
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}

const redisURL = "rediss://default:s3cr3t-pw@redis.example.com:6379/0"
{%- endif %}

// writeStack copies the committed defaults and writes a stack overlay.
func writeStack(t *testing.T, overlay string) string {
	t.Helper()
	dir := t.TempDir()
	defaults, err := os.ReadFile("../../config/default.yaml")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "default.yaml"), defaults, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "test"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test", "config.yaml"), []byte(overlay), 0o600))
	return dir
}

const base = `
stack: test
project: prj
region: europe-west1
`

{%- if cookiecutter.use_postgres == 'y' %}

const dbApp = "app: {db: {host: db.internal, user: app, name: app}}\n"
{%- else %}

const dbApp = ""
{%- endif %}

const access = "access: {callers: [gateway@prj.iam.gserviceaccount.com]}\n"

const overlay = base + dbApp + access

func setSecrets(t *testing.T) {
	t.Helper()
{%- if cookiecutter.use_postgres == 'y' %}
	t.Setenv("DEPLOY_SECRET_DB_PASSWORD", dbPassword)
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	t.Setenv("DEPLOY_SECRET_REDIS_URL", redisURL)
{%- endif %}
}

func TestCommittedStacks(t *testing.T) {
	setSecrets(t)
	for _, stack := range []string{"dev", "prod"} {
		t.Run(stack, func(t *testing.T) {
			cfg, sec, err := config.Load("../../config", stack)
			require.NoError(t, err)
			assert.Equal(t, stack, cfg.Stack)
			assert.Zero(t, cfg.Service.MinInstances, "scale to zero")
{%- if cookiecutter.use_postgres == 'y' %}
			assert.Equal(t, dbPassword, sec.DBPassword)
			assert.NotEmpty(t, cfg.App.DB.Host)
{%- else %}
			assert.NotNil(t, sec)
{%- endif %}
		})
	}
}

func TestEnvOverride(t *testing.T) {
	setSecrets(t)
	t.Setenv("DEPLOY_SERVICE_MAX_INSTANCES", "9")
	t.Setenv("DEPLOY_ACCESS_CALLERS", "a@prj.iam.gserviceaccount.com,b@prj.iam.gserviceaccount.com")

	cfg, _, err := config.Load(writeStack(t, overlay), "test")
	require.NoError(t, err)
	assert.Equal(t, 9, cfg.Service.MaxInstances)
	assert.Equal(t, []string{"a@prj.iam.gserviceaccount.com", "b@prj.iam.gserviceaccount.com"}, cfg.Access.Callers)
}

func TestRejects(t *testing.T) {
	tests := []struct {
		name, overlay, want string
	}{
		{"public caller", base + dbApp + "access: {callers: [allUsers]}\n", "access.callers[0]"},
		{"no caller", base + dbApp + "access: {callers: []}\n", "access.callers"},
		{"stack mismatch", "stack: other\nproject: p\nregion: r\n" + dbApp + access, "selected stack"},
		{"unknown key", overlay + "\nnope: 1\n", "nope"},
		{"managed env", overlay + "\nservice: {env: {{ '{' }}{{ cookiecutter.env_prefix }}_LOG_LEVEL: x}}\n", "cannot be overridden"},
{%- if cookiecutter.use_postgres == 'y' %}
		{"pool min above max", base + access + "app: {db: {host: h, user: u, name: n, pool: {max_conns: 2, min_conns: 3}}}\n", "app.db.pool.min_conns"},
		{"bad sslmode", base + access + "app: {db: {host: h, user: u, name: n, sslmode: maybe}}\n", "app.db.sslmode"},
{%- endif %}
		{"memory for cpu always allocated", overlay + "\nservice: {cpu_idle: false, memory: 256Mi}\n", "must be at least 512Mi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setSecrets(t)
			_, _, err := config.Load(writeStack(t, tt.overlay), "test")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

{%- if cookiecutter.use_postgres == 'y' %}

func TestDBPasswordRequired(t *testing.T) {
	t.Setenv("DEPLOY_SECRET_DB_PASSWORD", "")
	_, _, err := config.Load(writeStack(t, overlay), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db_password")
}
{%- endif %}
{%- if cookiecutter.use_auth == 'y' %}

func TestAuth(t *testing.T) {
	setSecrets(t)
	cfg, _, err := config.Load(writeStack(t, overlay), "test")
	require.NoError(t, err)
	assert.Equal(t, "jwks", cfg.App.Auth.Mode)
	assert.False(t, cfg.Access.Public)

	t.Run("static needs the public key secret", func(t *testing.T) {
		setSecrets(t)
		t.Setenv("DEPLOY_APP_AUTH_MODE", "static")
		_, _, err := config.Load(writeStack(t, overlay), "test")
		require.ErrorContains(t, err, "auth_public_key")

		t.Setenv("DEPLOY_SECRET_AUTH_PUBLIC_KEY", "-----BEGIN PUBLIC KEY-----\nx\n-----END PUBLIC KEY-----\n")
		_, sec, err := config.Load(writeStack(t, overlay), "test")
		require.NoError(t, err)
		assert.NotEmpty(t, sec.AuthPublicKey)
	})

	t.Run("jwks needs a url", func(t *testing.T) {
		setSecrets(t)
		t.Setenv("DEPLOY_APP_AUTH_JWKS_URL", "")
		_, _, err := config.Load(writeStack(t, overlay), "test")
		require.ErrorContains(t, err, "app.auth.jwks_url")
	})

	t.Run("public needs no caller", func(t *testing.T) {
		setSecrets(t)
		cfg, _, err := config.Load(writeStack(t, base+dbApp+"access: {callers: [], public: true}\n"), "test")
		require.NoError(t, err)
		assert.True(t, cfg.Access.Public)
		assert.NotEmpty(t, cfg.Warnings())
	})
}
{%- endif %}
