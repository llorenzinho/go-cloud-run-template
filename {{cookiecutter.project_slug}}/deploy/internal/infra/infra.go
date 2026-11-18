// Package infra declares the resources of one stack: a private Cloud Run
// service scaling to zero, its runtime service account and the secrets it reads.
package infra

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/cloudrunv2"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/projects"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/secretmanager"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/serviceaccount"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"{{ cookiecutter.module_path }}/deploy/internal/config"
)

// Port is the container port of the server (http.addr in the image).
const Port = 8080

// APIs are the Google APIs the stack uses.
var APIs = []string{
	"run.googleapis.com",
	"secretmanager.googleapis.com",
}

var ingress = map[string]string{
	"all":                               "INGRESS_TRAFFIC_ALL",
	"internal":                          "INGRESS_TRAFFIC_INTERNAL_ONLY",
	"internal-and-cloud-load-balancing": "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER",
}

// secretEnv is a server env var read from Secret Manager.
type secretEnv struct {
	env     string
	version *secretmanager.SecretVersion
}

// Deploy declares every resource of the stack and exports its outputs.
func Deploy(ctx *pulumi.Context, cfg *config.Config, sec *config.Secrets) error {
	labels := map[string]string{"app": "{{ cookiecutter.project_slug }}", "stack": cfg.Stack}
	maps.Copy(labels, cfg.Labels)
	provider, err := gcp.NewProvider(ctx, "gcp", &gcp.ProviderArgs{
		Project:       pulumi.String(cfg.Project),
		Region:        pulumi.String(cfg.Region),
		DefaultLabels: pulumi.ToStringMap(labels),
	})
	if err != nil {
		return err
	}
	opts := []pulumi.ResourceOption{pulumi.Provider(provider)}

	apis, err := enableAPIs(ctx, cfg, opts)
	if err != nil {
		return err
	}
	opts = append(opts, pulumi.DependsOn(apis))

	sa, err := serviceaccount.NewAccount(ctx, "runtime", &serviceaccount.AccountArgs{
		AccountId:   pulumi.String(cfg.Service.Name + "-run"),
		DisplayName: pulumi.String("{{ cookiecutter.project_name }} runtime (" + cfg.Stack + ")"),
	}, opts...)
	if err != nil {
		return err
	}

	values := []struct{ name, env, value string }{
{%- if cookiecutter.use_postgres == 'y' %}
		{"db-password", config.EnvPrefixApp + "_DB_PASSWORD", sec.DBPassword},
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
		{"redis-url", config.EnvPrefixApp + "_REDIS_URL", sec.RedisURL},
{%- endif %}
	}
{%- if cookiecutter.use_auth == 'y' %}
	if cfg.App.Auth.Mode == "static" {
		// Not confidential, but versioned and rotated like the other secrets.
		values = append(values, struct{ name, env, value string }{"auth-public-key", config.EnvPrefixApp + "_AUTH_STATIC_PUBLIC_KEY", sec.AuthPublicKey})
	}
{%- endif %}
	secrets := make([]secretEnv, 0, len(values))
	deps := make([]pulumi.Resource, 0, len(values))
	for _, v := range values {
		version, access, err := secret(ctx, cfg, v.name, v.value, sa, opts)
		if err != nil {
			return err
		}
		secrets = append(secrets, secretEnv{env: v.env, version: version})
		deps = append(deps, access)
	}

	svc, err := service(ctx, cfg, sa, secrets, append(opts, pulumi.DependsOn(deps))...)
	if err != nil {
		return err
	}
	if err := invokers(ctx, cfg, svc, opts); err != nil {
		return err
	}

	ctx.Export("url", svc.Uri)
	ctx.Export("service_account", sa.Email)
	return nil
}

func enableAPIs(ctx *pulumi.Context, cfg *config.Config, opts []pulumi.ResourceOption) ([]pulumi.Resource, error) {
	if !cfg.EnableAPIs {
		return nil, nil
	}
	res := make([]pulumi.Resource, 0, len(APIs))
	for _, api := range APIs {
		s, err := projects.NewService(ctx, "api-"+strings.TrimSuffix(api, ".googleapis.com"), &projects.ServiceArgs{
			Service:          pulumi.String(api),
			DisableOnDestroy: pulumi.Bool(false), // other workloads may use it
		}, opts...)
		if err != nil {
			return nil, err
		}
		res = append(res, s)
	}
	return res, nil
}

// secret stores value in Secret Manager as <service>-<name>, readable by the runtime SA only.
func secret(ctx *pulumi.Context, cfg *config.Config, name, value string, sa *serviceaccount.Account,
	opts []pulumi.ResourceOption,
) (*secretmanager.SecretVersion, pulumi.Resource, error) {
	s, err := secretmanager.NewSecret(ctx, name, &secretmanager.SecretArgs{
		SecretId:    pulumi.String(cfg.Service.Name + "-" + name),
		Replication: &secretmanager.SecretReplicationArgs{Auto: &secretmanager.SecretReplicationAutoArgs{}},
	}, opts...)
	if err != nil {
		return nil, nil, err
	}
	// A new value replaces the version (create, repoint Cloud Run, then delete the
	// old one). DELETE destroys the old version: disabled versions are still billed.
	version, err := secretmanager.NewSecretVersion(ctx, name, &secretmanager.SecretVersionArgs{
		Secret:         s.ID(),
		SecretData:     pulumi.ToSecret(pulumi.String(value)).(pulumi.StringOutput),
		DeletionPolicy: pulumi.String("DELETE"),
	}, opts...)
	if err != nil {
		return nil, nil, err
	}
	access, err := secretmanager.NewSecretIamMember(ctx, name+"-runtime", &secretmanager.SecretIamMemberArgs{
		SecretId: s.SecretId,
		Role:     pulumi.String("roles/secretmanager.secretAccessor"),
		Member:   sa.Member,
	}, opts...)
	if err != nil {
		return nil, nil, err
	}
	return version, access, nil
}

func service(ctx *pulumi.Context, cfg *config.Config, sa *serviceaccount.Account, secrets []secretEnv,
	opts ...pulumi.ResourceOption,
) (*cloudrunv2.Service, error) {
	s := cfg.Service
	envs := cloudrunv2.ServiceTemplateContainerEnvArray{}
	for _, se := range secrets {
		envs = append(envs, &cloudrunv2.ServiceTemplateContainerEnvArgs{
			Name: pulumi.String(se.env),
			ValueSource: &cloudrunv2.ServiceTemplateContainerEnvValueSourceArgs{
				SecretKeyRef: &cloudrunv2.ServiceTemplateContainerEnvValueSourceSecretKeyRefArgs{
					Secret: se.version.Secret,
					// Pinned version: a new value rolls out a new revision.
					Version: se.version.Version,
				},
			},
		})
	}
	server := ServerEnv(cfg)
	for _, k := range slices.Sorted(maps.Keys(server)) {
		envs = append(envs, &cloudrunv2.ServiceTemplateContainerEnvArgs{Name: pulumi.String(k), Value: pulumi.String(server[k])})
	}

	return cloudrunv2.NewService(ctx, "service", &cloudrunv2.ServiceArgs{
		Name:               pulumi.String(s.Name),
		Location:           pulumi.String(cfg.Region),
		Ingress:            pulumi.String(ingress[s.Ingress]),
		DeletionProtection: pulumi.Bool(s.DeletionProtection),
{%- if cookiecutter.use_auth == 'y' %}
		// Cloud Run checks the caller's Google ID token and roles/run.invoker
		// (allUsers with access.public) before the request reaches the
		// container; the app then verifies the end-user Bearer JWT.
{%- else %}
		// IAM is the only authentication: Cloud Run checks the caller's Google
		// ID token and roles/run.invoker before the request reaches the container.
{%- endif %}
		InvokerIamDisabled: pulumi.Bool(false),
		Template: &cloudrunv2.ServiceTemplateArgs{
			ServiceAccount:                sa.Email,
			MaxInstanceRequestConcurrency: pulumi.Int(s.Concurrency),
			Timeout:                       pulumi.String(strconv.Itoa(int(s.Timeout.Seconds())) + "s"),
			Scaling: &cloudrunv2.ServiceTemplateScalingArgs{
				MinInstanceCount: pulumi.Int(s.MinInstances),
				MaxInstanceCount: pulumi.Int(s.MaxInstances),
			},
			Containers: cloudrunv2.ServiceTemplateContainerArray{
				&cloudrunv2.ServiceTemplateContainerArgs{
					Image: pulumi.String(cfg.Image.Ref()),
					Ports: &cloudrunv2.ServiceTemplateContainerPortsArgs{ContainerPort: pulumi.Int(Port)},
					Resources: &cloudrunv2.ServiceTemplateContainerResourcesArgs{
						Limits:          pulumi.StringMap{"cpu": pulumi.String(s.CPU), "memory": pulumi.String(s.Memory)},
						CpuIdle:         pulumi.Bool(s.CPUIdle),
						StartupCpuBoost: pulumi.Bool(s.StartupCPUBoost),
					},
					Envs: envs,
					StartupProbe: &cloudrunv2.ServiceTemplateContainerStartupProbeArgs{
						HttpGet:          &cloudrunv2.ServiceTemplateContainerStartupProbeHttpGetArgs{Path: pulumi.String("/readyz")},
						PeriodSeconds:    pulumi.Int(3),
						FailureThreshold: pulumi.Int(10),
					},
					LivenessProbe: &cloudrunv2.ServiceTemplateContainerLivenessProbeArgs{
						HttpGet:       &cloudrunv2.ServiceTemplateContainerLivenessProbeHttpGetArgs{Path: pulumi.String("/healthz")},
						PeriodSeconds: pulumi.Int(30),
					},
				},
			},
		},
	}, opts...)
}

// ServerEnv returns the server env vars that depend only on the config,
// followed by service.env. Secrets are added by service.
func ServerEnv(cfg *config.Config) map[string]string {
	a := cfg.App
	p := config.EnvPrefixApp
	env := map[string]string{
		p + "_ENV":             cfg.Stack,
		p + "_HTTP_ADDR":       ":" + strconv.Itoa(Port),
		p + "_HTTP_BASE_PATH":  a.BasePath,
		p + "_LOG_LEVEL":       a.LogLevel,
		p + "_LOG_FORMAT":      "json",
		p + "_HTTP_REQUEST_TIMEOUT": a.RequestTimeout.String(),
{%- if cookiecutter.use_postgres == 'y' %}
		p + "_DB_HOST":                   a.DB.Host,
		p + "_DB_PORT":                   strconv.Itoa(a.DB.Port),
		p + "_DB_USER":                   a.DB.User,
		p + "_DB_NAME":                   a.DB.Name,
		p + "_DB_SSLMODE":                a.DB.SSLMode,
		p + "_DB_AUTO_MIGRATE":           strconv.FormatBool(a.DB.AutoMigrate),
		p + "_DB_POOL_MAX_CONNS":         strconv.Itoa(a.DB.Pool.MaxConns),
		p + "_DB_POOL_MIN_CONNS":         strconv.Itoa(a.DB.Pool.MinConns),
		p + "_DB_POOL_MAX_CONN_LIFETIME":  a.DB.Pool.MaxConnLifetime.String(),
		p + "_DB_POOL_MAX_CONN_IDLE_TIME": a.DB.Pool.MaxConnIdleTime.String(),
		p + "_DB_TIMEOUTS_CONNECT":       a.DB.Timeouts.Connect.String(),
		p + "_DB_TIMEOUTS_QUERY":         a.DB.Timeouts.Query.String(),
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
		p + "_REDIS_POOL_SIZE": strconv.Itoa(a.RedisPoolSize),
{%- endif %}
	}
	if len(a.TrustedProxies) > 0 {
		env[p+"_HTTP_TRUSTED_PROXIES"] = strings.Join(a.TrustedProxies, ",")
	}
{%- if cookiecutter.use_auth == 'y' %}
	maps.Copy(env, authEnv(a.Auth))
{%- endif %}
	maps.Copy(env, cfg.Service.Env) // validated: no managed keys
	return env
}

{%- if cookiecutter.use_auth == 'y' %}

// authEnv returns the auth.* env vars of the server. Verification is always
// on in a deploy (auth.enabled=false is refused outside env=dev anyway).
func authEnv(a config.AuthConfig) map[string]string {
	p := config.EnvPrefixApp
	env := map[string]string{
		p + "_AUTH_ENABLED":     "true",
		p + "_AUTH_MODE":        a.Mode,
		p + "_AUTH_ISSUER":      a.Issuer,
		p + "_AUTH_AUDIENCES":   strings.Join(a.Audiences, ","),
		p + "_AUTH_ALGORITHMS":  strings.Join(a.Algorithms, ","),
		p + "_AUTH_ROLES_CLAIM": a.RolesClaim,
		p + "_AUTH_JWKS_URL":    a.JWKSURL,
		// The image's config.yaml points to the local dev key: never use it.
		p + "_AUTH_STATIC_PUBLIC_KEY_FILE": "",
	}
	if len(a.TokenTypes) > 0 {
		env[p+"_AUTH_TOKEN_TYPES"] = strings.Join(a.TokenTypes, ",")
	}
	return env
}
{%- endif %}

{%- if cookiecutter.use_auth == 'y' %}

// invokers lets the callers invoke the service, and everyone with access.public.
{%- else %}

// invokers lets the callers (and only them) invoke the service. There is no
// public binding: allUsers / allAuthenticatedUsers are rejected by the config.
{%- endif %}
func invokers(ctx *pulumi.Context, cfg *config.Config, svc *cloudrunv2.Service, opts []pulumi.ResourceOption) error {
	for _, email := range cfg.Access.Callers {
		_, err := cloudrunv2.NewServiceIamMember(ctx, "invoker-"+email, &cloudrunv2.ServiceIamMemberArgs{
			Name:     svc.Name,
			Location: svc.Location,
			Role:     pulumi.String("roles/run.invoker"),
			Member:   pulumi.String("serviceAccount:" + email),
		}, opts...)
		if err != nil {
			return fmt.Errorf("invoker %s: %w", email, err)
		}
	}
{%- if cookiecutter.use_auth == 'y' %}
	if cfg.Access.Public {
		_, err := cloudrunv2.NewServiceIamMember(ctx, "invoker-public", &cloudrunv2.ServiceIamMemberArgs{
			Name:     svc.Name,
			Location: svc.Location,
			Role:     pulumi.String("roles/run.invoker"),
			Member:   pulumi.String("allUsers"),
		}, opts...)
		if err != nil {
			return fmt.Errorf("public invoker: %w", err)
		}
	}
{%- endif %}
	return nil
}
