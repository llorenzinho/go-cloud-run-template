package infra_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{ cookiecutter.module_path }}/deploy/internal/config"
	"{{ cookiecutter.module_path }}/deploy/internal/infra"
	"{{ cookiecutter.module_path }}/deploy/internal/stacktest"
)

// deploy runs the program on the prod stack with set applied over its file.
func deploy(t *testing.T, set, extraSecrets map[string]any) *stacktest.Mocks {
	t.Helper()
	secrets := map[string]any{
{%- if cookiecutter.use_postgres == 'y' %}
		"db_password": "s3cr3t-pw",
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
		"redis_url": "redis://h:6379/0",
{%- endif %}
	}
	for k, v := range extraSecrets {
		secrets[k] = v
	}
	m, err := stacktest.Run(t, "prod", set, secrets, func(ctx *pulumi.Context) error {
		cfg, sec, err := config.Load(ctx)
		if err != nil {
			return err
		}
		return infra.Deploy(ctx, cfg, sec)
	})
	require.NoError(t, err)
	return m
}

func serviceJSON(t *testing.T, m *stacktest.Mocks) string {
	t.Helper()
	svc, ok := m.Resources["gcp:cloudrunv2/service:Service::service"]
	require.True(t, ok)
	raw, err := json.Marshal(svc.Mappable())
	require.NoError(t, err)
	return string(raw)
}

func TestNothingIsPublic(t *testing.T) {
	m := deploy(t, nil, nil)
	for key, in := range m.Resources {
		raw, err := json.Marshal(in.Mappable())
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "allUsers", key)
		assert.NotContains(t, string(raw), "allAuthenticatedUsers", key)
	}
	invokers := 0
	for key, in := range m.Resources {
		if strings.HasPrefix(key, "gcp:cloudrunv2/serviceIamMember:ServiceIamMember::") {
			invokers++
			assert.Equal(t, "roles/run.invoker", in["role"].StringValue())
			assert.True(t, strings.HasPrefix(in["member"].StringValue(), "serviceAccount:"))
		}
	}
	assert.Positive(t, invokers)
}

func TestServiceScalesToZero(t *testing.T) {
	m := deploy(t, nil, nil)
	svc := m.Resources["gcp:cloudrunv2/service:Service::service"]
	tpl := svc["template"].ObjectValue()
	assert.InDelta(t, 0, tpl["scaling"].ObjectValue()["minInstanceCount"].NumberValue(), 0)
	assert.Equal(t, "60s", tpl["timeout"].StringValue())
	assert.False(t, svc["invokerIamDisabled"].BoolValue())

	raw := serviceJSON(t, m)
	assert.NotContains(t, raw, "s3cr3t-pw", "secrets are referenced, not inlined")
	assert.Contains(t, raw, "{{ cookiecutter.env_prefix }}_HTTP_REQUEST_TIMEOUT")
{%- if cookiecutter.use_postgres == 'y' %}
	assert.Contains(t, raw, "{{ cookiecutter.env_prefix }}_DB_PASSWORD")
	assert.Contains(t, raw, "{{ cookiecutter.env_prefix }}_DB_HOST")
{%- endif %}
}
{%- if cookiecutter.use_auth == 'y' %}

func TestAuthEnv(t *testing.T) {
	m := deploy(t, nil, nil)
	raw := serviceJSON(t, m)
	assert.Contains(t, raw, "{{ cookiecutter.env_prefix }}_AUTH_JWKS_URL")
	assert.Contains(t, raw, "{{ cookiecutter.env_prefix }}_AUTH_ISSUER")
	_, ok := m.Resources["gcp:secretmanager/secret:Secret::auth-public-key"]
	assert.False(t, ok, "jwks mode has no key secret")
}

func TestAuthStaticKeySecret(t *testing.T) {
	m := deploy(t,
		map[string]any{"app": map[string]any{"auth": map[string]any{"mode": "static", "issuer": "i", "audiences": []string{"a"}}}},
		map[string]any{"auth_public_key": "-----BEGIN PUBLIC KEY-----"})
	_, ok := m.Resources["gcp:secretmanager/secret:Secret::auth-public-key"]
	assert.True(t, ok)
	raw := serviceJSON(t, m)
	assert.Contains(t, raw, "{{ cookiecutter.env_prefix }}_AUTH_STATIC_PUBLIC_KEY")
	assert.NotContains(t, raw, "BEGIN PUBLIC KEY", "the key is referenced, not inlined")
}

func TestPublicAccess(t *testing.T) {
	m := deploy(t, map[string]any{"access": map[string]any{"callers": []string{}, "public": true}}, nil)
	in, ok := m.Resources["gcp:cloudrunv2/serviceIamMember:ServiceIamMember::invoker-public"]
	require.True(t, ok)
	assert.Equal(t, "allUsers", in["member"].StringValue())
	assert.Equal(t, "roles/run.invoker", in["role"].StringValue())
}
{%- endif %}
