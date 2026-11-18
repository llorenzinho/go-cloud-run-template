// Package config reads the stack configuration from Pulumi config
// (Pulumi.<stack>.yaml, `pulumi config set [--secret] <key> <value>`).
//
// Objects (image, service, app, access) are merged over Default(): a stack
// sets only what differs. Secrets are Pulumi secrets: encrypted in the stack
// file, never in plain text in the program.
package config

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// EnvPrefixApp is the env prefix of the server configuration.
const EnvPrefixApp = "{{ cookiecutter.env_prefix }}"

// Config is the deploy configuration of one stack.
type Config struct {
	// Stack is the Pulumi stack name; it becomes the server env.
	Stack   string `json:"-"`
	Project string `json:"-"`
	Region  string `json:"-"`
	// EnableAPIs enables the Google APIs the stack needs.
	EnableAPIs bool              `json:"enable_apis"`
	Labels     map[string]string `json:"labels"`
	Image      ImageConfig       `json:"image"`
	Service    ServiceConfig     `json:"service"`
	App        AppConfig         `json:"app"`
	Access     AccessConfig      `json:"access"`
}

// ImageConfig is the container image to run.
type ImageConfig struct {
	// Repository is the image without tag.
	Repository string `json:"repository"`
	// Tag is the release (bumped by release-please in Default) or a digest
	// (sha256:…). A mutable tag such as latest hides image changes from Pulumi.
	Tag string `json:"tag"`
}

// Ref returns the full image reference: <repository>:<tag> or <repository>@sha256:….
func (i ImageConfig) Ref() string {
	if strings.HasPrefix(i.Tag, "sha256:") {
		return i.Repository + "@" + i.Tag
	}
	return i.Repository + ":" + i.Tag
}

// ServiceConfig configures the Cloud Run service.
type ServiceConfig struct {
	// Name of the service; also prefix of the runtime service account
	// (<name>-run, max 30 chars) and of the secrets.
	Name         string `json:"name"`
	MinInstances int    `json:"min_instances"` // 0 = scale to zero
	MaxInstances int    `json:"max_instances"`
	CPU          string `json:"cpu"`
	Memory       string `json:"memory"`
	// CPUIdle allocates CPU only while serving requests (cheaper, allows <512Mi).
	CPUIdle         bool   `json:"cpu_idle"`
	StartupCPUBoost bool   `json:"startup_cpu_boost"`
	Concurrency     int    `json:"concurrency"`
	Timeout         string `json:"timeout"` // Go duration, e.g. 60s
	// Ingress: all | internal | internal-and-cloud-load-balancing.
	Ingress            string `json:"ingress"`
	DeletionProtection bool   `json:"deletion_protection"`
	// Env adds server env vars; the managed ones (ManagedEnv) are rejected.
	Env map[string]string `json:"env"`
}

// AppConfig is the server configuration, passed as {{ cookiecutter.env_prefix }}_* env vars.
type AppConfig struct {
	BasePath string `json:"base_path"`
	LogLevel string `json:"log_level"`
	// RequestTimeout is the deadline of each request; keep it below service.timeout.
	RequestTimeout string   `json:"request_timeout"`
	TrustedProxies []string `json:"trusted_proxies"`
{%- if cookiecutter.use_postgres == 'y' %}
	DB DBConfig `json:"db"`
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	RedisPoolSize int `json:"redis_pool_size"`
{%- endif %}
{%- if cookiecutter.use_auth == 'y' %}
	Auth AuthConfig `json:"auth"`
{%- endif %}
}
{%- if cookiecutter.use_postgres == 'y' %}

// DBConfig is the database of the stack. The password is the secret db_password.
type DBConfig struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	Name    string `json:"name"`
	SSLMode string `json:"sslmode"`
	// AutoMigrate applies the migrations at startup; false: task migrate:up ENV=<stack>.
	AutoMigrate bool `json:"auto_migrate"`
	Pool        struct {
		// max_instances x max_conns is the worst case number of connections.
		MaxConns        int    `json:"max_conns"`
		MinConns        int    `json:"min_conns"`
		MaxConnLifetime string `json:"max_conn_lifetime"`
		MaxConnIdleTime string `json:"max_conn_idle_time"`
	} `json:"pool"`
	Timeouts struct {
		Connect string `json:"connect"`
		Query   string `json:"query"`
	} `json:"timeouts"`
}
{%- endif %}
{%- if cookiecutter.use_auth == 'y' %}

// AuthConfig is the Bearer JWT verification of the API. In static mode the
// public key is the secret auth_public_key.
type AuthConfig struct {
	Mode       string   `json:"mode"` // jwks | static
	Issuer     string   `json:"issuer"`
	Audiences  []string `json:"audiences"`
	Algorithms []string `json:"algorithms"`
	JWKSURL    string   `json:"jwks_url"`
	RolesClaim string   `json:"roles_claim"`
	TokenTypes []string `json:"token_types"`
}
{%- endif %}

// AccessConfig lists who may call the service.
type AccessConfig struct {
	// Callers are service accounts (gateway, other services): they get
	// roles/run.invoker. Cloud Run authenticates every request against them.
	Callers []string `json:"callers"`
{%- if cookiecutter.use_auth == 'y' %}
	// Public grants roles/run.invoker to allUsers: the Bearer JWT is the only gate.
	Public bool `json:"public"`
{%- endif %}
}

// Secrets are Pulumi secrets of the stack.
type Secrets struct {
{%- if cookiecutter.use_postgres == 'y' %}
	DBPassword pulumi.StringOutput
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	RedisURL pulumi.StringOutput
{%- endif %}
{%- if cookiecutter.use_auth == 'y' %}
	// AuthPublicKey is set only with app.auth.mode static.
	AuthPublicKey pulumi.StringOutput
{%- endif %}
}

// ManagedEnv are the server env vars set by the deploy; service.env cannot override them.
var ManagedEnv = []string{
	"{{ cookiecutter.env_prefix }}_CONFIG",
	"{{ cookiecutter.env_prefix }}_ENV",
	"{{ cookiecutter.env_prefix }}_HTTP_ADDR",
	"{{ cookiecutter.env_prefix }}_HTTP_BASE_PATH",
	"{{ cookiecutter.env_prefix }}_HTTP_TRUSTED_PROXIES",
	"{{ cookiecutter.env_prefix }}_LOG_LEVEL",
	"{{ cookiecutter.env_prefix }}_LOG_FORMAT",
	"{{ cookiecutter.env_prefix }}_HTTP_REQUEST_TIMEOUT",
{%- if cookiecutter.use_postgres == 'y' %}
	"{{ cookiecutter.env_prefix }}_DB_HOST",
	"{{ cookiecutter.env_prefix }}_DB_PORT",
	"{{ cookiecutter.env_prefix }}_DB_USER",
	"{{ cookiecutter.env_prefix }}_DB_PASSWORD",
	"{{ cookiecutter.env_prefix }}_DB_NAME",
	"{{ cookiecutter.env_prefix }}_DB_SSLMODE",
	"{{ cookiecutter.env_prefix }}_DB_AUTO_MIGRATE",
	"{{ cookiecutter.env_prefix }}_DB_POOL_MAX_CONNS",
	"{{ cookiecutter.env_prefix }}_DB_POOL_MIN_CONNS",
	"{{ cookiecutter.env_prefix }}_DB_POOL_MAX_CONN_LIFETIME",
	"{{ cookiecutter.env_prefix }}_DB_POOL_MAX_CONN_IDLE_TIME",
	"{{ cookiecutter.env_prefix }}_DB_TIMEOUTS_CONNECT",
	"{{ cookiecutter.env_prefix }}_DB_TIMEOUTS_QUERY",
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	"{{ cookiecutter.env_prefix }}_REDIS_URL",
	"{{ cookiecutter.env_prefix }}_REDIS_POOL_SIZE",
{%- endif %}
{%- if cookiecutter.use_auth == 'y' %}
	"{{ cookiecutter.env_prefix }}_AUTH_ENABLED",
	"{{ cookiecutter.env_prefix }}_AUTH_MODE",
	"{{ cookiecutter.env_prefix }}_AUTH_ISSUER",
	"{{ cookiecutter.env_prefix }}_AUTH_AUDIENCES",
	"{{ cookiecutter.env_prefix }}_AUTH_ALGORITHMS",
	"{{ cookiecutter.env_prefix }}_AUTH_ROLES_CLAIM",
	"{{ cookiecutter.env_prefix }}_AUTH_TOKEN_TYPES",
	"{{ cookiecutter.env_prefix }}_AUTH_JWKS_URL",
	"{{ cookiecutter.env_prefix }}_AUTH_STATIC_PUBLIC_KEY",
{%- endif %}
}

// Default returns the values shared by every stack.
func Default() Config {
	c := Config{
		EnableAPIs: true,
		Image: ImageConfig{
			Repository: "{{ cookiecutter.gcp_region }}-docker.pkg.dev/{{ cookiecutter.gcp_project }}/{{ cookiecutter.ar_repository }}/{{ cookiecutter.project_slug }}",
			Tag:        "v0.0.0", // x-release-please-version
		},
		Service: ServiceConfig{
			Name:               "{{ cookiecutter.project_slug }}",
			MaxInstances:       5,
			CPU:                "1",
			Memory:             "256Mi",
			CPUIdle:            true,
			StartupCPUBoost:    true,
			Concurrency:        80,
			Timeout:            "60s",
			Ingress:            "all",
			DeletionProtection: true,
		},
		App: AppConfig{
			BasePath:       "{{ cookiecutter.base_path }}",
			LogLevel:       "info",
			RequestTimeout: "30s",
{%- if cookiecutter.use_auth == 'y' %}
			Auth: AuthConfig{
				Mode:       "jwks",
				Algorithms: []string{"RS256"},
				RolesClaim: "roles",
			},
{%- endif %}
		},
	}
{%- if cookiecutter.use_postgres == 'y' %}
	c.App.DB.Port = 5432
	c.App.DB.SSLMode = "require"
	c.App.DB.AutoMigrate = true
	c.App.DB.Pool.MaxConns = 5
	c.App.DB.Pool.MaxConnLifetime = "30m"
	c.App.DB.Pool.MaxConnIdleTime = "5m"
	c.App.DB.Timeouts.Connect = "5s"
	c.App.DB.Timeouts.Query = "10s"
{%- endif %}
	return c
}

// Load reads the stack configuration over Default and validates it.
func Load(ctx *pulumi.Context) (*Config, *Secrets, error) {
	pc := pconfig.New(ctx, "")
	cfg := Default()
	cfg.Stack = ctx.Stack()
	cfg.Project = pc.Get("project")
	cfg.Region = pc.Get("region")
	for key, out := range map[string]any{
		"enable_apis": &cfg.EnableAPIs, "labels": &cfg.Labels, "image": &cfg.Image,
		"service": &cfg.Service, "app": &cfg.App, "access": &cfg.Access,
	} {
		if err := pc.GetObject(key, out); err != nil {
			return nil, nil, fmt.Errorf("config %s: %w", key, err)
		}
	}

	var errs []error
{%- if cookiecutter.use_postgres == 'y' or cookiecutter.use_redis == 'y' or cookiecutter.use_auth == 'y' %}
	secret := func(key string, required bool) pulumi.StringOutput {
		if required && pc.Get(key) == "" {
			errs = append(errs, fmt.Errorf("%s: is required: pulumi config set --secret %s <value>", key, key))
		}
		return pc.GetSecret(key)
	}
{%- endif %}
	sec := &Secrets{
{%- if cookiecutter.use_postgres == 'y' %}
		DBPassword: secret("db_password", true),
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
		RedisURL: secret("redis_url", true),
{%- endif %}
{%- if cookiecutter.use_auth == 'y' %}
		AuthPublicKey: secret("auth_public_key", cfg.App.Auth.Mode == "static"),
{%- endif %}
	}
	if err := errors.Join(append(errs, cfg.validate())...); err != nil {
		return nil, nil, fmt.Errorf("stack %s: %w", cfg.Stack, err)
	}
	return &cfg, sec, nil
}

var (
	runNameRe = regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)
	// A Google service account: <id>@<project>.iam.gserviceaccount.com and the
	// Google-managed variants. allUsers / allAuthenticatedUsers never match.
	serviceAccountRe = regexp.MustCompile(`^[a-z0-9-]+@[a-z0-9-]+(\.iam)?\.gserviceaccount\.com$`)
)

// validate keeps the checks that protect something; the Cloud Run API
// rejects the other invalid values at preview.
func (c *Config) validate() error {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	if c.Project == "" || c.Region == "" {
		fail("project and region are required: pulumi config set project <id>; pulumi config set region <region>")
	}
	if len(c.Service.Name) > 26 || !runNameRe.MatchString(c.Service.Name) {
		fail("service.name %q: lowercase letters, digits and '-', max 26 chars", c.Service.Name)
	}
	for _, d := range []struct{ key, v string }{
		{"service.timeout", c.Service.Timeout}, {"app.request_timeout", c.App.RequestTimeout},
	} {
		if _, err := time.ParseDuration(d.v); err != nil {
			fail("%s: %w", d.key, err)
		}
	}
	for _, sa := range c.Access.Callers {
		if !serviceAccountRe.MatchString(sa) {
			fail("access.callers: %q is not a service account (public principals are not allowed)", sa)
		}
	}
{%- if cookiecutter.use_auth == 'y' %}
	if len(c.Access.Callers) == 0 && !c.Access.Public {
		fail("access.callers: at least one service account (or access.public: true)")
	}
	if a := c.App.Auth; a.Mode != "jwks" && a.Mode != "static" {
		fail("app.auth.mode: must be jwks or static")
	} else if a.Mode == "jwks" && a.JWKSURL == "" {
		fail("app.auth.jwks_url: is required with mode jwks")
	}
	if c.App.Auth.Issuer == "" || len(c.App.Auth.Audiences) == 0 {
		fail("app.auth.issuer and app.auth.audiences are required")
	}
{%- else %}
	if len(c.Access.Callers) == 0 {
		fail("access.callers: at least one service account")
	}
{%- endif %}
	for k := range c.Service.Env {
		if slices.Contains(ManagedEnv, k) {
			fail("service.env: %s is set by the deploy", k)
		}
	}
	return errors.Join(errs...)
}

// Warnings returns non-fatal configuration issues.
func (c *Config) Warnings() []string {
	var w []string
	if c.Image.Tag == "latest" {
		w = append(w, fmt.Sprintf("image %q uses a mutable tag: Pulumi cannot detect changes behind it", c.Image.Ref()))
	}
	req, _ := time.ParseDuration(c.App.RequestTimeout) // validated
	svc, _ := time.ParseDuration(c.Service.Timeout)
	if req == 0 || req >= svc {
		w = append(w, fmt.Sprintf("app.request_timeout (%s) should be below service.timeout (%s): Cloud Run cuts the request first", req, svc))
	}
{%- if cookiecutter.use_auth == 'y' %}
	if c.Access.Public {
		w = append(w, "access.public is true: anyone can reach the service, the Bearer JWT (app.auth) is the only gate")
	}
{%- endif %}
{%- if cookiecutter.use_postgres == 'y' %}
	if c.App.DB.SSLMode == "disable" {
		w = append(w, "app.db.sslmode is disable: the connection to the database is not encrypted")
	}
{%- endif %}
	return w
}
