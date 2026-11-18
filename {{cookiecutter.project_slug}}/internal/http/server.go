// Package http wires the gin engine and the HTTP server.
package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"{{ cookiecutter.module_path }}/internal/http/handler"
	"{{ cookiecutter.module_path }}/internal/http/middleware"
	"{{ cookiecutter.module_path }}/internal/validation"
)

// Deps are the router dependencies.
type Deps struct {
	// BasePath prefixes the API (see config http.base_path).
	BasePath string
	// TrustedProxies are IPs/CIDRs allowed to set X-Forwarded-For (see config http.trusted_proxies).
	TrustedProxies []string
	// RequestTimeout is the deadline of each request context (see config http.request_timeout).
	RequestTimeout time.Duration
	Log            *slog.Logger
	Health         *handler.Health
{%- if cookiecutter.__examples == 'y' %}
	Items *handler.Items
{%- endif %}
}

// APIPath returns the root of the versioned API under basePath: "/orders" -> "/orders/v1".
func APIPath(basePath string) string {
	return basePath + "/v1"
}

// NewRouter builds the gin engine with middlewares and routes. Callers are
// authenticated by Cloud Run (roles/run.invoker): requests reaching the
// container already carry a verified Google identity.
func NewRouter(d Deps) (*gin.Engine, error) {
	binding.Validator = validation.Gin{}
	r := gin.New()
	r.ContextWithFallback = true
	// Forwarded headers are honoured only from these peers; none by default.
	if err := r.SetTrustedProxies(d.TrustedProxies); err != nil {
		return nil, fmt.Errorf("trusted proxies: %w", err)
	}
	r.Use(
		middleware.RequestID(),
		middleware.Logger(d.Log, handler.HealthzPath, handler.ReadyzPath),
		middleware.Recovery(d.Log),
		middleware.Errors(),
		middleware.Timeout(d.RequestTimeout),
	)

	d.Health.Register(r)

	v1 := r.Group(APIPath(d.BasePath))
{%- if cookiecutter.__examples == 'y' %}
	d.Items.Register(v1)
{%- else %}
	_ = v1 // mount your handlers here: h.Register(v1)
{%- endif %}

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, middleware.ErrorBody{
			Error:     middleware.ErrorDetail{Code: "not_found", Message: "route not found"},
			RequestID: middleware.GetRequestID(c),
		})
	})
	return r, nil
}

// Server wraps http.Server with a lifecycle usable by the DI container.
type Server struct {
	srv             *http.Server
	addr            string
	shutdownTimeout time.Duration
	log             *slog.Logger
}

// Option configures NewServer.
type Option func(*Server)

// WithAddr sets the listen address (default ":8080").
func WithAddr(addr string) Option {
	return func(s *Server) { s.addr = addr }
}

// WithReadHeaderTimeout bounds the time to read request headers (default 10s).
func WithReadHeaderTimeout(d time.Duration) Option {
	return func(s *Server) { s.srv.ReadHeaderTimeout = d }
}

// WithShutdownTimeout bounds graceful shutdown (default 15s). Cloud Run sends
// SIGTERM and kills the instance 10s later: keep in-flight requests short.
func WithShutdownTimeout(d time.Duration) Option {
	return func(s *Server) { s.shutdownTimeout = d }
}

// WithLogger sets the logger (default slog.Default()).
func WithLogger(log *slog.Logger) Option {
	return func(s *Server) { s.log = log }
}

// NewServer returns a server for handler h.
func NewServer(h http.Handler, opts ...Option) *Server {
	s := &Server{
		srv:             &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second},
		addr:            ":8080",
		shutdownTimeout: 15 * time.Second,
		log:             slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.srv.Addr = s.addr
	return s
}

// Run listens and serves until Shutdown is called.
func (s *Server) Run(ctx context.Context) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	s.log.Info("http server listening", "addr", ln.Addr().String())
	if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server (implements do.ShutdownerWithContextAndError).
func (s *Server) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
	defer cancel()
	return s.srv.Shutdown(ctx)
}
