// Package postgres opens the pgx connection pool and exposes it to sqlc
// through a DBTX that applies a default query timeout.
package postgres

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"{{ cookiecutter.module_path }}/internal/db/sqlc"
)

// Conn identifies the database to connect to.
type Conn struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	// SSLMode is a libpq sslmode: disable, require, verify-full, ...
	SSLMode string
}

// URL returns the connection string, with every part escaped.
func (c Conn) URL() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Path:   "/" + c.Database,
	}
	if c.SSLMode != "" {
		u.RawQuery = url.Values{"sslmode": {c.SSLMode}}.Encode()
	}
	return u.String()
}

// DB owns the Postgres pool. Shutdown makes it a samber/do Shutdowner;
// Name and Check make it a health.Checker.
type DB struct {
	Pool         *pgxpool.Pool
	schema       string
	queryTimeout time.Duration
}

type options struct {
	schema                string
	applicationName       string
	maxConns              int32
	minConns              int32
	minIdleConns          int32
	maxConnLifetime       time.Duration
	maxConnLifetimeJitter time.Duration
	maxConnIdleTime       time.Duration
	healthCheckPeriod     time.Duration
	connectTimeout        time.Duration
	pingTimeout           time.Duration
	queryTimeout          time.Duration
}

// Option configures New.
type Option func(*options)

// WithSchema sets the search_path of every connection (default "public").
func WithSchema(schema string) Option {
	return func(o *options) { o.schema = schema }
}

// WithApplicationName sets application_name, shown in pg_stat_activity.
func WithApplicationName(name string) Option {
	return func(o *options) { o.applicationName = name }
}

// WithMaxConns caps the pool size; 0 keeps the pgx default (max(4, NumCPU)).
func WithMaxConns(n int32) Option {
	return func(o *options) { o.maxConns = n }
}

// WithMinConns keeps n connections open. Keep it 0 on Cloud Run with scale to
// zero: idle instances would otherwise hold connections.
func WithMinConns(n int32) Option {
	return func(o *options) { o.minConns = n }
}

// WithMinIdleConns keeps n idle connections ready.
func WithMinIdleConns(n int32) Option {
	return func(o *options) { o.minIdleConns = n }
}

// WithMaxConnLifetime recycles connections older than d.
func WithMaxConnLifetime(d time.Duration) Option {
	return func(o *options) { o.maxConnLifetime = d }
}

// WithMaxConnLifetimeJitter spreads recycling over d, so connections do not expire together.
func WithMaxConnLifetimeJitter(d time.Duration) Option {
	return func(o *options) { o.maxConnLifetimeJitter = d }
}

// WithMaxConnIdleTime closes connections idle for longer than d.
func WithMaxConnIdleTime(d time.Duration) Option {
	return func(o *options) { o.maxConnIdleTime = d }
}

// WithHealthCheckPeriod sets how often idle connections are checked.
func WithHealthCheckPeriod(d time.Duration) Option {
	return func(o *options) { o.healthCheckPeriod = d }
}

// WithConnectTimeout bounds dialing a new connection.
func WithConnectTimeout(d time.Duration) Option {
	return func(o *options) { o.connectTimeout = d }
}

// WithStartupPingTimeout bounds the connectivity check done by New (default 5s).
func WithStartupPingTimeout(d time.Duration) Option {
	return func(o *options) { o.pingTimeout = d }
}

// WithQueryTimeout sets the default deadline of queries run through DBTX and
// InTx whose context has no earlier deadline. 0 = none.
func WithQueryTimeout(d time.Duration) Option {
	return func(o *options) { o.queryTimeout = d }
}

// New connects to conn and pings the server.
func New(ctx context.Context, conn Conn, opts ...Option) (*DB, error) {
	o := options{schema: "public", pingTimeout: 5 * time.Second}
	for _, opt := range opts {
		opt(&o)
	}

	cfg, err := pgxpool.ParseConfig(conn.URL())
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = o.schema
	if o.applicationName != "" {
		cfg.ConnConfig.RuntimeParams["application_name"] = o.applicationName
	}
	if o.connectTimeout > 0 {
		cfg.ConnConfig.ConnectTimeout = o.connectTimeout
	}
	if o.maxConns > 0 {
		cfg.MaxConns = o.maxConns
	}
	cfg.MinConns = o.minConns
	cfg.MinIdleConns = o.minIdleConns
	if o.maxConnLifetime > 0 {
		cfg.MaxConnLifetime = o.maxConnLifetime
	}
	cfg.MaxConnLifetimeJitter = o.maxConnLifetimeJitter
	if o.maxConnIdleTime > 0 {
		cfg.MaxConnIdleTime = o.maxConnIdleTime
	}
	if o.healthCheckPeriod > 0 {
		cfg.HealthCheckPeriod = o.healthCheckPeriod
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	pctx, cancel := withTimeout(ctx, o.pingTimeout)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db %s:%d: %w", conn.Host, conn.Port, err)
	}
	return &DB{Pool: pool, schema: o.schema, queryTimeout: o.queryTimeout}, nil
}

// DBTX returns the pool as a sqlc.DBTX applying the default query timeout.
func (d *DB) DBTX() sqlc.DBTX {
	return timeoutDB{inner: d.Pool, timeout: d.queryTimeout}
}

// Shutdown closes the pool.
func (d *DB) Shutdown() { d.Pool.Close() }

// Name implements health.Checker.
func (d *DB) Name() string { return "postgres" }

// Check implements health.Checker: the server answers and the search_path
// resolves to the service schema (it exists once the migrations ran).
func (d *DB) Check(ctx context.Context) error {
	schema, err := sqlc.New().CurrentSchema(ctx, d.Pool)
	if err != nil {
		return err
	}
	if schema != d.schema {
		return fmt.Errorf("schema %q not found (migrations applied?)", d.schema)
	}
	return nil
}
