// Package redis opens the go-redis client.
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Client wraps *goredis.Client with a samber/do lifecycle; Name and Check
// make it a health.Checker.
type Client struct {
	*goredis.Client
}

type options struct {
	poolSize     int
	dialTimeout  time.Duration
	readTimeout  time.Duration
	writeTimeout time.Duration
	pingTimeout  time.Duration
}

// Option configures New.
type Option func(*options)

// WithPoolSize caps the connection pool; 0 keeps the go-redis default (10 per CPU).
func WithPoolSize(n int) Option {
	return func(o *options) { o.poolSize = n }
}

// WithDialTimeout bounds new connections.
func WithDialTimeout(d time.Duration) Option {
	return func(o *options) { o.dialTimeout = d }
}

// WithReadTimeout bounds socket reads.
func WithReadTimeout(d time.Duration) Option {
	return func(o *options) { o.readTimeout = d }
}

// WithWriteTimeout bounds socket writes.
func WithWriteTimeout(d time.Duration) Option {
	return func(o *options) { o.writeTimeout = d }
}

// WithPingTimeout bounds the connectivity check done by New (default 5s).
func WithPingTimeout(d time.Duration) Option {
	return func(o *options) { o.pingTimeout = d }
}

// New connects to url (redis://[user:pass@]host:port/db, rediss:// for TLS) and pings the server.
func New(ctx context.Context, url string, opts ...Option) (*Client, error) {
	o := options{pingTimeout: 5 * time.Second}
	for _, opt := range opts {
		opt(&o)
	}

	ropts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	if o.poolSize > 0 {
		ropts.PoolSize = o.poolSize
	}
	if o.dialTimeout > 0 {
		ropts.DialTimeout = o.dialTimeout
	}
	if o.readTimeout > 0 {
		ropts.ReadTimeout = o.readTimeout
	}
	if o.writeTimeout > 0 {
		ropts.WriteTimeout = o.writeTimeout
	}

	c := goredis.NewClient(ropts)
	pctx, cancel := context.WithTimeout(ctx, o.pingTimeout)
	defer cancel()
	if err := c.Ping(pctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Client{Client: c}, nil
}

// Shutdown closes the client.
func (c *Client) Shutdown() error { return c.Close() }

// Name implements health.Checker.
func (c *Client) Name() string { return "redis" }

// Check implements health.Checker: the server answers PING.
func (c *Client) Check(ctx context.Context) error { return c.Ping(ctx).Err() }
