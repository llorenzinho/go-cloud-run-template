// Package config loads the service configuration from a YAML file with
// environment variable overrides.
package config

import (
	"cmp"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/viper"

{%- if cookiecutter.use_postgres == 'y' %}

	"{{ cookiecutter.module_path }}/db"
{%- endif %}
	"{{ cookiecutter.module_path }}/internal/validation"
)

// EnvPrefix is the prefix of every environment variable read by the service.
const EnvPrefix = "{{ cookiecutter.env_prefix }}"

// Config is the root configuration.
type Config struct {
	Env    string       `mapstructure:"env" validate:"required"`
	HTTP   HTTPConfig   `mapstructure:"http"`
	Log    LogConfig    `mapstructure:"log"`
	Health HealthConfig `mapstructure:"health"`
{%- if cookiecutter.use_postgres == 'y' %}
	DB DBConfig `mapstructure:"db"`
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	Redis RedisConfig `mapstructure:"redis"`
{%- endif %}
}

// HTTPConfig configures the HTTP server.
type HTTPConfig struct {
	Addr string `mapstructure:"addr" validate:"required"`
	// BasePath prefixes the API routes (<base_path>/v1/...) so that a gateway can
	// route by path. Empty = root. Health probes always stay at the root.
	BasePath          string        `mapstructure:"base_path" validate:"omitempty,basepath"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout" validate:"gt=0"`
	ShutdownTimeout   time.Duration `mapstructure:"shutdown_timeout" validate:"gt=0"`
	// RequestTimeout is the deadline of every request context, propagated to
	// queries. Keep it below the Cloud Run request timeout. 0 = none.
	RequestTimeout time.Duration `mapstructure:"request_timeout" validate:"gte=0"`
	// TrustedProxies may set X-Forwarded-For / X-Real-IP (IPs or CIDRs).
	TrustedProxies []string `mapstructure:"trusted_proxies" validate:"dive,cidr|ip"`
}

// LogConfig configures the logger.
type LogConfig struct {
	Level  string `mapstructure:"level" validate:"oneof=debug info warn error"`
	Format string `mapstructure:"format" validate:"oneof=json text"`
}

// HealthConfig configures the readiness checks (/readyz).
type HealthConfig struct {
	// Timeout bounds each dependency check.
	Timeout time.Duration `mapstructure:"timeout" validate:"gt=0"`
}
{%- if cookiecutter.use_postgres == 'y' %}

// DBConfig configures the Postgres connection.
type DBConfig struct {
	Host     string `mapstructure:"host" validate:"required"`
	Port     int    `mapstructure:"port" validate:"gte=1,lte=65535"`
	User     string `mapstructure:"user" validate:"required"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name" validate:"required"`
	SSLMode  string `mapstructure:"sslmode" validate:"oneof=disable allow prefer require verify-ca verify-full"`
	// Schema holds the tables and the goose version table; it is the search_path.
	Schema          string `mapstructure:"schema" validate:"required"`
	ApplicationName string `mapstructure:"application_name"`
	// AutoMigrate applies pending migrations at startup (under an advisory lock).
	AutoMigrate bool           `mapstructure:"auto_migrate"`
	Pool        DBPoolConfig   `mapstructure:"pool"`
	Timeouts    DBTimeoutConfig `mapstructure:"timeouts"`
}

// DBPoolConfig configures the pgx connection pool.
type DBPoolConfig struct {
	MaxConns              int32         `mapstructure:"max_conns" validate:"gte=1"`
	MinConns              int32         `mapstructure:"min_conns" validate:"gte=0,ltefield=MaxConns"`
	MinIdleConns          int32         `mapstructure:"min_idle_conns" validate:"gte=0,ltefield=MaxConns"`
	MaxConnLifetime       time.Duration `mapstructure:"max_conn_lifetime" validate:"gte=0"`
	MaxConnLifetimeJitter time.Duration `mapstructure:"max_conn_lifetime_jitter" validate:"gte=0"`
	MaxConnIdleTime       time.Duration `mapstructure:"max_conn_idle_time" validate:"gte=0"`
	HealthCheckPeriod     time.Duration `mapstructure:"health_check_period" validate:"gte=0"`
}

// DBTimeoutConfig bounds connections and queries. 0 disables a timeout.
type DBTimeoutConfig struct {
	// Connect bounds dialing a new connection.
	Connect time.Duration `mapstructure:"connect" validate:"gte=0"`
	// StartupPing bounds the connectivity check at startup.
	StartupPing time.Duration `mapstructure:"startup_ping" validate:"gte=0"`
	// Query is the default deadline of a query whose context has none (or a later one).
	Query time.Duration `mapstructure:"query" validate:"gte=0"`
}
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}

// RedisConfig configures the Redis connection.
type RedisConfig struct {
	// URL is redis://[user:password@]host:port/db (rediss:// for TLS).
	URL      string `mapstructure:"url" validate:"required,url"`
	PoolSize int    `mapstructure:"pool_size" validate:"gte=0"`
}
{%- endif %}

// DefaultPath is the configuration file used when neither Load's argument nor
// {{ cookiecutter.env_prefix }}_CONFIG is set. It is relative to the working directory.
const DefaultPath = "configs/config.yaml"

// Load reads the YAML configuration file and applies environment overrides:
// every key maps to {{ cookiecutter.env_prefix }}_<KEY> with dots replaced by underscores
// (db.max_conns -> {{ cookiecutter.env_prefix }}_DB_MAX_CONNS).
// The file is taken from path, then {{ cookiecutter.env_prefix }}_CONFIG, then DefaultPath.
func Load(path string) (*Config, error) {
	if path == "" {
		path = cmp.Or(os.Getenv(EnvPrefix+"_CONFIG"), DefaultPath)
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AllowEmptyEnv(true) // {{ cookiecutter.env_prefix }}_X="" clears a value set in the file
	v.AutomaticEnv()
	// AutomaticEnv only applies to keys viper already knows: bind every key of
	// Config so env vars also work for keys missing from the file.
	if err := bindEnvs(v, reflect.TypeFor[Config](), ""); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// bindEnvs binds the env var of every leaf key derived from mapstructure tags.
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
		if f.Type.Kind() == reflect.Struct && f.Type != reflect.TypeFor[time.Time]() {
			if err := bindEnvs(v, f.Type, key); err != nil {
				return err
			}
			continue
		}
		if err := v.BindEnv(key); err != nil {
			return fmt.Errorf("bind env %s: %w", key, err)
		}
	}
	return nil
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if err := validation.Struct(c); err != nil {
		return fmt.Errorf("config: %w", err)
	}
{%- if cookiecutter.use_postgres == 'y' %}
	// The schema is written in the migrations: it cannot change at runtime.
	if c.DB.Schema != db.Schema {
		return fmt.Errorf("config: db.schema: must be %q (the schema of db/migrations)", db.Schema)
	}
{%- endif %}
	return nil
}
