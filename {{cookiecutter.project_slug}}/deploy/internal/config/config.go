// Package config loads the deploy configuration of a stack:
//
//	<dir>/default.yaml          shared defaults (committed)
//	<dir>/<stack>/config.yaml   per-environment overlay (committed)
//	<dir>/<stack>/secret.yaml   secrets (never committed)
//
// Every key can be overridden by an environment variable: DEPLOY_<KEY> for
// the config and DEPLOY_SECRET_<KEY> for secrets, with dots replaced by
// underscores (service.max_instances -> DEPLOY_SERVICE_MAX_INSTANCES).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// Env var prefixes.
const (
	EnvPrefix       = "DEPLOY"
	SecretEnvPrefix = "DEPLOY_SECRET"
	// EnvPrefixApp is the env prefix of the server configuration.
	EnvPrefixApp = "{{ cookiecutter.env_prefix }}"
)

// Config is the deploy configuration of one stack (environment).
type Config struct {
	// Stack must match the Pulumi stack: a guard against deploying the wrong overlay.
	Stack   string `mapstructure:"stack" validate:"required"`
	Project string `mapstructure:"project" validate:"required"`
	Region  string `mapstructure:"region" validate:"required"`
	// EnableAPIs enables the Google APIs the stack needs.
	EnableAPIs bool              `mapstructure:"enable_apis"`
	Labels     map[string]string `mapstructure:"labels" validate:"dive,keys,label,endkeys,max=63"`
	Image      ImageConfig       `mapstructure:"image"`
	Service    ServiceConfig     `mapstructure:"service"`
	App        AppConfig         `mapstructure:"app"`
	Access     AccessConfig      `mapstructure:"access"`
}

// ImageConfig is the container image to run.
type ImageConfig struct {
	// Repository is the image without tag, e.g. <region>-docker.pkg.dev/<project>/<repo>/{{ cookiecutter.project_slug }}.
	Repository string `mapstructure:"repository" validate:"required,imagerepo"`
	// Tag is the release (v0.0.1, bumped by release-please in default.yaml) or
	// a digest (sha256:…). A mutable tag such as latest hides image changes
	// from Pulumi.
	Tag string `mapstructure:"tag" validate:"required,excludes=@"`
}

// Ref returns the full image reference: <repository>:<tag> or <repository>@sha256:….
func (i ImageConfig) Ref() string {
	if strings.HasPrefix(i.Tag, "sha256:") {
		return i.Repository + "@" + i.Tag
	}
	return i.Repository + ":" + i.Tag
}

{%- if cookiecutter.use_auth == 'y' %}
// ServiceConfig configures the Cloud Run service. Invokers are the service
// accounts in access.callers, plus allUsers with access.public.
{%- else %}
// ServiceConfig configures the Cloud Run service. It is never public: there is
// no setting for it, invokers are the service accounts in access.callers.
{%- endif %}
type ServiceConfig struct {
	// Name of the service; also prefix of the runtime service account
	// (<name>-run, max 30 chars) and of the secrets.
	Name         string `mapstructure:"name" validate:"required,max=26,runname"`
	MinInstances int    `mapstructure:"min_instances" validate:"gte=0"` // 0 = scale to zero
	MaxInstances int    `mapstructure:"max_instances" validate:"gte=1,gtefield=MinInstances"`
	CPU          string `mapstructure:"cpu" validate:"oneof=1 2 4 8"`
	Memory       string `mapstructure:"memory" validate:"required,memory"`
	// CPUIdle allocates CPU only while serving requests (cheaper, allows <512Mi).
	CPUIdle            bool          `mapstructure:"cpu_idle"`
	StartupCPUBoost    bool          `mapstructure:"startup_cpu_boost"`
	Concurrency        int           `mapstructure:"concurrency" validate:"gte=1,lte=1000"`
	Timeout            time.Duration `mapstructure:"timeout" validate:"gte=1s,lte=60m"`
	Ingress            string        `mapstructure:"ingress" validate:"oneof=all internal internal-and-cloud-load-balancing"`
	DeletionProtection bool          `mapstructure:"deletion_protection"`
	// Env adds environment variables to the server (e.g. {{ cookiecutter.env_prefix }}_LOG_LEVEL is
	// in app; this is for anything else). Managed variables cannot be set here.
	Env map[string]string `mapstructure:"env" validate:"dive,keys,envname,endkeys"`
}

// AppConfig is the server configuration, passed as {{ cookiecutter.env_prefix }}_* env vars.
type AppConfig struct {
	BasePath string `mapstructure:"base_path" validate:"omitempty,startswith=/,endsnotwith=/"`
	LogLevel string `mapstructure:"log_level" validate:"oneof=debug info warn error"`
	// RequestTimeout is the deadline of each request context; keep it below service.timeout.
	RequestTimeout time.Duration `mapstructure:"request_timeout" validate:"gte=0"`
	TrustedProxies []string      `mapstructure:"trusted_proxies" validate:"dive,cidr|ip"`
{%- if cookiecutter.use_postgres == 'y' %}
	DB DBConfig `mapstructure:"db"`
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	RedisPoolSize int `mapstructure:"redis_pool_size" validate:"gte=0"`
{%- endif %}
{%- if cookiecutter.use_auth == 'y' %}
	Auth AuthConfig `mapstructure:"auth"`
{%- endif %}
}
{%- if cookiecutter.use_auth == 'y' %}

// AuthConfig is the Bearer JWT verification of the API. In static mode the
// public key is the secret auth_public_key (Secret Manager <service>-auth-public-key).
type AuthConfig struct {
	Mode       string   `mapstructure:"mode" validate:"oneof=jwks static"`
	Issuer     string   `mapstructure:"issuer" validate:"required"`
	Audiences  []string `mapstructure:"audiences" validate:"min=1,dive,required"`
	Algorithms []string `mapstructure:"algorithms" validate:"min=1,dive,oneof=RS256 RS384 RS512 PS256 PS384 PS512 ES256 ES384 ES512 EdDSA"`
	// JWKSURL is required with mode jwks.
	JWKSURL    string   `mapstructure:"jwks_url" validate:"required_if=Mode jwks,omitempty,url"`
	RolesClaim string   `mapstructure:"roles_claim"`
	TokenTypes []string `mapstructure:"token_types" validate:"dive,required"`
}
{%- endif %}
{%- if cookiecutter.use_postgres == 'y' %}

// DBConfig is the database of the stack. The password is a secret (db_password).
type DBConfig struct {
	Host    string `mapstructure:"host" validate:"required"`
	Port    int    `mapstructure:"port" validate:"gte=1,lte=65535"`
	User    string `mapstructure:"user" validate:"required"`
	Name    string `mapstructure:"name" validate:"required"`
	SSLMode string `mapstructure:"sslmode" validate:"oneof=disable allow prefer require verify-ca verify-full"`
	// AutoMigrate applies the migrations at startup (advisory lock); false: task migrate:up ENV=<stack>.
	AutoMigrate bool            `mapstructure:"auto_migrate"`
	Pool        DBPoolConfig    `mapstructure:"pool"`
	Timeouts    DBTimeoutConfig `mapstructure:"timeouts"`
}

// DBPoolConfig sizes the pgx pool of each instance: max_instances x max_conns
// is the worst case number of connections.
type DBPoolConfig struct {
	MaxConns        int           `mapstructure:"max_conns" validate:"gte=1"`
	MinConns        int           `mapstructure:"min_conns" validate:"gte=0,ltefield=MaxConns"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime" validate:"gte=0"`
	MaxConnIdleTime time.Duration `mapstructure:"max_conn_idle_time" validate:"gte=0"`
}

// DBTimeoutConfig bounds connections and queries (0 = none).
type DBTimeoutConfig struct {
	Connect time.Duration `mapstructure:"connect" validate:"gte=0"`
	Query   time.Duration `mapstructure:"query" validate:"gte=0"`
}
{%- endif %}

{%- if cookiecutter.use_auth == 'y' %}
// AccessConfig lists who may call the service.
type AccessConfig struct {
	// Callers are service accounts (gateway, other services): they get
	// roles/run.invoker. Cloud Run authenticates every request against them.
	// At least one unless Public.
	Callers []string `mapstructure:"callers" validate:"unique,dive,serviceaccount"`
	// Public grants roles/run.invoker to allUsers: anyone reaches the
	// container and the Bearer JWT (app.auth) is the only gate.
	Public bool `mapstructure:"public"`
}
{%- else %}
// AccessConfig lists who may call the service.
type AccessConfig struct {
	// Callers are service accounts (gateway, other services): they get
	// roles/run.invoker. Cloud Run authenticates every request against them.
	Callers []string `mapstructure:"callers" validate:"min=1,unique,dive,serviceaccount"`
}
{%- endif %}

// Secrets are the values read from secret.yaml / DEPLOY_SECRET_*.
type Secrets struct {
{%- if cookiecutter.use_postgres == 'y' %}
	DBPassword string `mapstructure:"db_password" validate:"required"`
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	RedisURL string `mapstructure:"redis_url" validate:"required,redisurl"`
{%- endif %}
{%- if cookiecutter.use_auth == 'y' %}
	// AuthPublicKey is the PEM public key of app.auth mode static (required then).
	AuthPublicKey string `mapstructure:"auth_public_key"`
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
	"{{ cookiecutter.env_prefix }}_AUTH_STATIC_PUBLIC_KEY_FILE",
{%- endif %}
}

// DefaultDir is the configuration directory, relative to the Pulumi project.
const DefaultDir = "config"

// Load reads and validates the configuration and secrets of stack from dir.
func Load(dir, stack string) (*Config, *Secrets, error) {
	cfg, err := LoadConfig(dir, stack)
	if err != nil {
		return nil, nil, err
	}
	sec, err := LoadSecrets(dir, stack)
	if err != nil {
		return nil, nil, err
	}
{%- if cookiecutter.use_auth == 'y' %}
	if cfg.App.Auth.Mode == "static" && sec.AuthPublicKey == "" {
		return nil, nil, errors.New("secrets: auth_public_key: is required with app.auth.mode static")
	}
{%- endif %}
	return cfg, sec, nil
}

// LoadConfig merges default.yaml and <stack>/config.yaml, applies DEPLOY_*
// env overrides and validates the result. Unknown keys are errors.
func LoadConfig(dir, stack string) (*Config, error) {
	v := newViper(EnvPrefix)
	v.SetConfigFile(filepath.Join(dir, "default.yaml"))
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read defaults: %w", err)
	}
	v.SetConfigFile(filepath.Join(dir, stack, "config.yaml"))
	if err := v.MergeInConfig(); err != nil {
		return nil, fmt.Errorf("read stack %q config: %w", stack, err)
	}
	if err := bindEnvs(v, reflect.TypeFor[Config](), ""); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.UnmarshalExact(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	// viper lowercases map keys: env var names are upper case.
	env := make(map[string]string, len(cfg.Service.Env))
	for k, val := range cfg.Service.Env {
		env[strings.ToUpper(k)] = val
	}
	cfg.Service.Env = env
	if err := check(&cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if cfg.Stack != stack {
		return nil, fmt.Errorf("config: stack is %q in the files, but the selected stack is %q", cfg.Stack, stack)
	}
	return &cfg, nil
}

// LoadSecrets reads <stack>/secret.yaml (optional when every secret comes
// from DEPLOY_SECRET_* env vars) and validates it. Errors never include values.
func LoadSecrets(dir, stack string) (*Secrets, error) {
	v := newViper(SecretEnvPrefix)
	v.SetConfigFile(filepath.Join(dir, stack, "secret.yaml"))
	if err := v.ReadInConfig(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read stack %q secrets: %w", stack, err)
	}
	if err := bindEnvs(v, reflect.TypeFor[Secrets](), ""); err != nil {
		return nil, err
	}

	var sec Secrets
	if err := v.UnmarshalExact(&sec); err != nil {
		// mapstructure errors may quote values: report the keys only.
		return nil, errors.New("decode secrets: invalid or unknown keys (expected: {{ ([] + (['db_password'] if cookiecutter.use_postgres == 'y' else []) + (['redis_url'] if cookiecutter.use_redis == 'y' else []) + (['auth_public_key'] if cookiecutter.use_auth == 'y' else [])) | join(', ') }})")
	}
	if err := check(&sec); err != nil {
		return nil, fmt.Errorf("secrets: %w", err)
	}
	return &sec, nil
}

// Warnings returns non-fatal configuration issues.
func (c *Config) Warnings() []string {
	var w []string
	if c.Image.Tag == "latest" {
		w = append(w, fmt.Sprintf("image %q uses a mutable tag: Pulumi cannot detect changes behind it", c.Image.Ref()))
	}
	if c.App.RequestTimeout == 0 || c.App.RequestTimeout >= c.Service.Timeout {
		w = append(w, fmt.Sprintf("app.request_timeout (%s) should be below service.timeout (%s): Cloud Run cuts the request first", c.App.RequestTimeout, c.Service.Timeout))
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

func newViper(prefix string) *viper.Viper {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetEnvPrefix(prefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AllowEmptyEnv(true)
	v.AutomaticEnv()
	return v
}

// bindEnvs binds the env var of every leaf key derived from mapstructure tags,
// so env vars work for keys missing from the files. Maps and lists of structs
// cannot be expressed as a single env var and are file-only.
func bindEnvs(v *viper.Viper, t reflect.Type, prefix string) error {
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("mapstructure"), ",")
		if name == "" || name == "-" {
			continue
		}
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		switch {
		case f.Type.Kind() == reflect.Struct:
			if err := bindEnvs(v, f.Type, key); err != nil {
				return err
			}
			continue
		case f.Type.Kind() == reflect.Map,
			f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.Struct:
			continue
		}
		if err := v.BindEnv(key); err != nil {
			return fmt.Errorf("bind env %s: %w", key, err)
		}
	}
	return nil
}

var (
	runNameRe    = regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)
	memoryRe     = regexp.MustCompile(`^([1-9][0-9]*)(Mi|Gi)$`)
	envNameRe    = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	labelRe      = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
	// A Google service account: <id>@<project>.iam.gserviceaccount.com and the
	// Google-managed variants (…@appspot.gserviceaccount.com, …@gcp-sa-….iam.…).
	serviceAccountRe = regexp.MustCompile(`^[a-z0-9-]+@[a-z0-9-]+(\.iam)?\.gserviceaccount\.com$`)
)

// validate returns the validator with the custom tags and rules of Config.
var validate = sync.OnceValue(func() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("mapstructure"), ",")
		return name
	})
	regex := func(re *regexp.Regexp) validator.Func {
		return func(fl validator.FieldLevel) bool { return re.MatchString(fl.Field().String()) }
	}
	must(v.RegisterValidation("runname", regex(runNameRe)))
	must(v.RegisterValidation("memory", regex(memoryRe)))
	must(v.RegisterValidation("envname", regex(envNameRe)))
	must(v.RegisterValidation("label", regex(labelRe)))
	must(v.RegisterValidation("serviceaccount", regex(serviceAccountRe)))
	// Repository without tag or digest (a registry host:port is allowed).
	must(v.RegisterValidation("imagerepo", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		return !strings.Contains(s, "@") && !strings.Contains(s[strings.LastIndex(s, "/")+1:], ":")
	}))
	must(v.RegisterValidation("redisurl", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		return strings.HasPrefix(s, "redis://") || strings.HasPrefix(s, "rediss://")
	}))
	v.RegisterStructValidation(validateService, ServiceConfig{})
{%- if cookiecutter.use_auth == 'y' %}
	v.RegisterStructValidation(validateAccess, AccessConfig{})
{%- endif %}
	return v
})

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// validateService enforces the Cloud Run limits between cpu, memory and CPU
// allocation, and keeps managed env vars out of service.env.
func validateService(sl validator.StructLevel) {
	s := sl.Current().Interface().(ServiceConfig)
	mib := memoryMiB(s.Memory)
	minMiB := map[string]int{"1": 128, "2": 128, "4": 2048, "8": 4096}[s.CPU]
	if !s.CPUIdle {
		minMiB = max(minMiB, 512) // CPU always allocated
	}
	if mib > 0 && mib < minMiB {
		sl.ReportError(s.Memory, "memory", "Memory", "min_memory", fmt.Sprintf("%dMi", minMiB))
	}
	for k := range s.Env {
		if slices.Contains(ManagedEnv, k) {
			sl.ReportError(s.Env, "env", "Env", "managed_env", k)
		}
	}
}

{%- if cookiecutter.use_auth == 'y' %}

// validateAccess requires a caller unless the service is public.
func validateAccess(sl validator.StructLevel) {
	a := sl.Current().Interface().(AccessConfig)
	if !a.Public && len(a.Callers) == 0 {
		sl.ReportError(a.Callers, "callers", "Callers", "min", "1")
	}
}
{%- endif %}

// memoryMiB parses "512Mi" / "1Gi"; 0 when invalid (reported by the memory tag).
func memoryMiB(s string) int {
	m := memoryRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	var n int
	_, _ = fmt.Sscan(m[1], &n)
	if m[2] == "Gi" {
		n *= 1024
	}
	return n
}

// check validates s and formats the errors as "field: rule" without values.
func check(s any) error {
	err := validate().Struct(s)
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return err
	}
	msgs := make([]string, 0, len(verrs))
	for _, fe := range verrs {
		msgs = append(msgs, fmt.Sprintf("%s: %s", fieldPath(fe.Namespace()), message(fe)))
	}
	return errors.New(strings.Join(msgs, "; "))
}

// fieldPath drops the root struct name: Config.service.memory -> service.memory.
func fieldPath(ns string) string {
	_, rest, ok := strings.Cut(ns, ".")
	if !ok {
		return ns
	}
	return rest
}

func message(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required", "required_if":
		return "is required"
	case "oneof":
		return "must be one of: " + fe.Param()
	case "min", "max", "gte", "lte", "gt":
		return fmt.Sprintf("must be %s %s", fe.Tag(), fe.Param())
	case "ltefield":
		return "must be <= " + fe.Param()
	case "gtefield":
		return "must be >= " + fe.Param()
	case "unique":
		return "must not contain duplicates"
	case "serviceaccount":
		return "must be a service account email (…@….iam.gserviceaccount.com); public principals are not allowed"
	case "runname":
		return "must be lowercase letters, digits and dashes, starting with a letter"
	case "memory":
		return `must be like "512Mi" or "1Gi"`
	case "min_memory":
		return "must be at least " + fe.Param() + " for this cpu / cpu_idle"
	case "envname":
		return "keys must be env var names (A-Z, 0-9, _)"
	case "managed_env":
		return fe.Param() + " is set by the deploy and cannot be overridden"
	case "label":
		return "keys must be GCP label keys (lowercase, start with a letter)"
	case "imagerepo":
		return "must be an image without tag or digest (set image.tag)"
	case "excludes":
		return "must not contain " + fe.Param()
	case "redisurl":
		return "must be a redis:// or rediss:// URL"
	case "cidr|ip":
		return "must be an IP or CIDR"
	case "url":
		return "must be a URL"
	default:
		return "failed rule " + fe.Tag()
	}
}

// Dir returns the config directory: DEPLOY_CONFIG_DIR or DefaultDir.
func Dir() string {
	if d := os.Getenv(EnvPrefix + "_CONFIG_DIR"); d != "" {
		return d
	}
	return DefaultDir
}
