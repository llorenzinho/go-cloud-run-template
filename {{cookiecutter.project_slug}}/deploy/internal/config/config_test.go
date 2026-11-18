package config_test

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{ cookiecutter.module_path }}/deploy/internal/config"
	"{{ cookiecutter.module_path }}/deploy/internal/stacktest"
)

// secrets are the required Pulumi secrets of a stack.
var secrets = map[string]any{
{%- if cookiecutter.use_postgres == 'y' %}
	"db_password": "s3cr3t-pw",
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	"redis_url": "rediss://default:s3cr3t-pw@redis.example.com:6379/0",
{%- endif %}
}

// load runs config.Load for stack with set applied over its committed file.
func load(t *testing.T, stack string, set map[string]any) (*config.Config, error) {
	t.Helper()
	var cfg *config.Config
	_, err := stacktest.Run(t, stack, set, secrets, func(ctx *pulumi.Context) error {
		var err error
		cfg, _, err = config.Load(ctx)
		return err
	})
	return cfg, err
}

func TestCommittedStacks(t *testing.T) {
	for _, stack := range []string{"dev", "prod"} {
		t.Run(stack, func(t *testing.T) {
			cfg, err := load(t, stack, nil)
			require.NoError(t, err)
			assert.Equal(t, stack, cfg.Stack)
			assert.Zero(t, cfg.Service.MinInstances, "scale to zero")
			assert.Equal(t, "60s", cfg.Service.Timeout, "default kept when the stack sets other service keys")
			assert.NotEmpty(t, cfg.Access.Callers)
{%- if cookiecutter.use_postgres == 'y' %}
			assert.NotEmpty(t, cfg.App.DB.Host)
			assert.Equal(t, 5432, cfg.App.DB.Port, "nested defaults are merged")
{%- endif %}
		})
	}
}

func TestRejects(t *testing.T) {
	const sa = "gateway@prj.iam.gserviceaccount.com"
	tests := []struct {
		name string
		set  map[string]any
		want string
	}{
		{"public caller", map[string]any{"access": map[string]any{"callers": []string{"allUsers"}}}, "access.callers"},
		{"no caller", map[string]any{"access": map[string]any{"callers": []string{}}}, "access.callers"},
		{"managed env", map[string]any{"service": map[string]any{"env": map[string]string{"{{ cookiecutter.env_prefix }}_LOG_LEVEL": "x"}}}, "set by the deploy"},
		{"bad timeout", map[string]any{"service": map[string]any{"timeout": "soon"}}, "service.timeout"},
		{"no project", map[string]any{"project": nil}, "project and region"},
{%- if cookiecutter.use_auth == 'y' %}
		{"jwks without url", map[string]any{"app": map[string]any{"auth": map[string]any{"issuer": "i", "audiences": []string{"a"}}}}, "app.auth.jwks_url"},
		{"static without key", map[string]any{"app": map[string]any{"auth": map[string]any{"mode": "static", "issuer": "i", "audiences": []string{"a"}}}}, "auth_public_key"},
{%- endif %}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(t, "prod", tt.set)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
	_, err := load(t, "prod", map[string]any{"access": map[string]any{"callers": []string{sa}}})
	require.NoError(t, err)
}
{%- if cookiecutter.use_postgres == 'y' %}

func TestDBPasswordRequired(t *testing.T) {
	_, err := stacktest.Run(t, "prod", nil, nil, func(ctx *pulumi.Context) error {
		_, _, err := config.Load(ctx)
		return err
	})
	require.ErrorContains(t, err, "db_password")
}
{%- endif %}
{%- if cookiecutter.use_auth == 'y' %}

func TestPublicAccess(t *testing.T) {
	cfg, err := load(t, "prod", map[string]any{"access": map[string]any{"callers": []string{}, "public": true}})
	require.NoError(t, err)
	assert.True(t, cfg.Access.Public)
	assert.NotEmpty(t, cfg.Warnings())
}
{%- endif %}
