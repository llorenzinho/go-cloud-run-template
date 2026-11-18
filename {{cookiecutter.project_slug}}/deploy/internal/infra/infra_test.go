package infra_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{ cookiecutter.module_path }}/deploy/internal/config"
	"{{ cookiecutter.module_path }}/deploy/internal/infra"
)


// mocks records the inputs of every resource registered by the program.
type mocks struct {
	mu        sync.Mutex
	resources map[string]resource.PropertyMap // by type + "::" + name
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources[args.TypeToken+"::"+args.Name] = args.Inputs
	out := args.Inputs.Copy()
	switch args.TypeToken {
	case "gcp:serviceaccount/account:Account":
		email := args.Inputs["accountId"].StringValue() + "@prj.iam.gserviceaccount.com"
		out["email"] = resource.NewStringProperty(email)
		out["member"] = resource.NewStringProperty("serviceAccount:" + email)
	case "gcp:secretmanager/secretVersion:SecretVersion":
		out["version"] = resource.NewStringProperty("1")
	}
	return args.Name + "-id", out, nil
}

func (m *mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func deploy(t *testing.T) *mocks {
	t.Helper()
{%- if cookiecutter.use_postgres == 'y' %}
	t.Setenv("DEPLOY_SECRET_DB_PASSWORD", "s3cr3t-pw")
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	t.Setenv("DEPLOY_SECRET_REDIS_URL", "redis://h:6379/0")
{%- endif %}
	cfg, sec, err := config.Load("../../config", "prod")
	require.NoError(t, err)

	m := &mocks{resources: map[string]resource.PropertyMap{}}
	err = pulumi.RunErr(func(ctx *pulumi.Context) error {
		return infra.Deploy(ctx, cfg, sec)
	}, pulumi.WithMocks("{{ cookiecutter.project_slug }}", "prod", m))
	require.NoError(t, err)
	return m
}

func TestNothingIsPublic(t *testing.T) {
	m := deploy(t)
	for key, in := range m.resources {
		raw, err := json.Marshal(in.Mappable())
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "allUsers", key)
		assert.NotContains(t, string(raw), "allAuthenticatedUsers", key)
	}
	invokers := 0
	for key, in := range m.resources {
		if strings.HasPrefix(key, "gcp:cloudrunv2/serviceIamMember:ServiceIamMember::") {
			invokers++
			assert.Equal(t, "roles/run.invoker", in["role"].StringValue())
			assert.True(t, strings.HasPrefix(in["member"].StringValue(), "serviceAccount:"))
		}
	}
	assert.Positive(t, invokers)
}

func TestServiceScalesToZero(t *testing.T) {
	m := deploy(t)
	svc, ok := m.resources["gcp:cloudrunv2/service:Service::service"]
	require.True(t, ok)
	tpl := svc["template"].ObjectValue()
	assert.InDelta(t, 0, tpl["scaling"].ObjectValue()["minInstanceCount"].NumberValue(), 0)
	assert.False(t, svc["invokerIamDisabled"].BoolValue())

	raw, err := json.Marshal(svc.Mappable())
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "s3cr3t-pw", "secrets are referenced, not inlined")
	assert.Contains(t, string(raw), "{{ cookiecutter.env_prefix }}_HTTP_REQUEST_TIMEOUT")
{%- if cookiecutter.use_postgres == 'y' %}
	assert.Contains(t, string(raw), "{{ cookiecutter.env_prefix }}_DB_PASSWORD")
	assert.Contains(t, string(raw), "{{ cookiecutter.env_prefix }}_DB_HOST")
{%- endif %}
}
