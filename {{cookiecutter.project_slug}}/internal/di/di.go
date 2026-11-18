// Package di wires the service with samber/do. Providers translate the
// configuration into the functional options of each constructor.
package di

import (
	"context"
	"log/slog"

	"github.com/samber/do/v2"

	"{{ cookiecutter.module_path }}/internal/config"
	"{{ cookiecutter.module_path }}/internal/health"
	ihttp "{{ cookiecutter.module_path }}/internal/http"
	"{{ cookiecutter.module_path }}/internal/http/handler"
	"{{ cookiecutter.module_path }}/internal/logger"
{%- if cookiecutter.use_postgres == 'y' %}
	"{{ cookiecutter.module_path }}/internal/postgres"
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	"{{ cookiecutter.module_path }}/internal/redis"
{%- endif %}
{%- if cookiecutter.__examples == 'y' %}
	"{{ cookiecutter.module_path }}/internal/repository"
	"{{ cookiecutter.module_path }}/internal/service"
{%- endif %}
)

// AppContext is cancelled on shutdown; background workers use it.
type AppContext struct {
	context.Context
	cancel context.CancelFunc
}

// Shutdown cancels the context.
func (a *AppContext) Shutdown() { a.cancel() }

// Infra provides the infrastructure: logger{% if cookiecutter.use_postgres == 'y' %}, database{% endif %}{% if cookiecutter.use_redis == 'y' %}, redis{% endif %} and readiness checks.
var Infra = do.Package(
	do.Lazy(provideAppContext),
	do.Lazy(provideLogger),
{%- if cookiecutter.use_postgres == 'y' %}
	do.Lazy(provideDatabase),
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	do.Lazy(provideRedis),
{%- endif %}
	do.Lazy(provideHealth),
)

// App provides the business and HTTP layers.
var App = do.Package(
{%- if cookiecutter.__examples == 'y' %}
	// example:begin items
	do.Lazy(provideItemRepository),
	do.Lazy(provideItemService),
	// example:end
{%- endif %}
	do.Lazy(provideRouterAndServer),
)

// New returns an injector for cfg with every package registered.
func New(cfg *config.Config, version string) *do.RootScope {
	i := do.New(Infra, App)
	do.ProvideValue(i, cfg)
	do.ProvideNamedValue(i, "version", version)
	return i
}

func provideAppContext(do.Injector) (*AppContext, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &AppContext{Context: ctx, cancel: cancel}, nil
}

func provideLogger(i do.Injector) (*slog.Logger, error) {
	cfg := do.MustInvoke[*config.Config](i)
	version := do.MustInvokeNamed[string](i, "version")
	log := logger.New(
		logger.WithLevel(cfg.Log.Level),
		logger.WithFormat(cfg.Log.Format),
		logger.WithAttrs(slog.String("version", version)),
	)
	slog.SetDefault(log)
	return log, nil
}
{%- if cookiecutter.use_postgres == 'y' %}

func provideDatabase(i do.Injector) (*postgres.DB, error) {
	cfg := do.MustInvoke[*config.Config](i).DB
	ctx := do.MustInvoke[*AppContext](i)
	return postgres.New(ctx,
		postgres.Conn{
			Host:     cfg.Host,
			Port:     cfg.Port,
			User:     cfg.User,
			Password: cfg.Password,
			Database: cfg.Name,
			SSLMode:  cfg.SSLMode,
		},
		postgres.WithSchema(cfg.Schema),
		postgres.WithApplicationName(cfg.ApplicationName),
		postgres.WithMaxConns(cfg.Pool.MaxConns),
		postgres.WithMinConns(cfg.Pool.MinConns),
		postgres.WithMinIdleConns(cfg.Pool.MinIdleConns),
		postgres.WithMaxConnLifetime(cfg.Pool.MaxConnLifetime),
		postgres.WithMaxConnLifetimeJitter(cfg.Pool.MaxConnLifetimeJitter),
		postgres.WithMaxConnIdleTime(cfg.Pool.MaxConnIdleTime),
		postgres.WithHealthCheckPeriod(cfg.Pool.HealthCheckPeriod),
		postgres.WithConnectTimeout(cfg.Timeouts.Connect),
		postgres.WithStartupPingTimeout(cfg.Timeouts.StartupPing),
		postgres.WithQueryTimeout(cfg.Timeouts.Query),
	)
}
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}

func provideRedis(i do.Injector) (*redis.Client, error) {
	cfg := do.MustInvoke[*config.Config](i)
	ctx := do.MustInvoke[*AppContext](i)
	return redis.New(ctx, cfg.Redis.URL, redis.WithPoolSize(cfg.Redis.PoolSize))
}
{%- endif %}

// provideHealth registers the readiness check of every dependency.
func provideHealth(i do.Injector) (*health.Registry, error) {
	cfg := do.MustInvoke[*config.Config](i)
	opts := []health.Option{health.WithTimeout(cfg.Health.Timeout)}
{%- if cookiecutter.use_postgres == 'y' %}
	db, err := do.Invoke[*postgres.DB](i)
	if err != nil {
		return nil, err
	}
	opts = append(opts, health.WithChecker(db))
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	rdb, err := do.Invoke[*redis.Client](i)
	if err != nil {
		return nil, err
	}
	opts = append(opts, health.WithChecker(rdb))
{%- endif %}
	return health.NewRegistry(opts...), nil
}
{%- if cookiecutter.__examples == 'y' %}

// example:begin items
func provideItemRepository(i do.Injector) (repository.ItemRepository, error) {
	db, err := do.Invoke[*postgres.DB](i)
	if err != nil {
		return nil, err
	}
	return repository.NewPostgres(db), nil
}

func provideItemService(i do.Injector) (*service.ItemService, error) {
	repo, err := do.Invoke[repository.ItemRepository](i)
	if err != nil {
		return nil, err
	}
	return service.NewItemService(repo, do.MustInvoke[*slog.Logger](i)), nil
}

// example:end
{%- endif %}

func provideRouterAndServer(i do.Injector) (*ihttp.Server, error) {
	cfg := do.MustInvoke[*config.Config](i)
	log := do.MustInvoke[*slog.Logger](i)

	registry, err := do.Invoke[*health.Registry](i)
	if err != nil {
		return nil, err
	}
{%- if cookiecutter.__examples == 'y' %}
	// example:begin items
	svc, err := do.Invoke[*service.ItemService](i)
	if err != nil {
		return nil, err
	}
	// example:end
{%- endif %}

	router, err := ihttp.NewRouter(ihttp.Deps{
		BasePath:       cfg.HTTP.BasePath,
		TrustedProxies: cfg.HTTP.TrustedProxies,
		RequestTimeout: cfg.HTTP.RequestTimeout,
		Log:            log,
		Health:         handler.NewHealth(registry),
{%- if cookiecutter.__examples == 'y' %}
		Items:          handler.NewItems(svc), // example
{%- endif %}
	})
	if err != nil {
		return nil, err
	}
	return ihttp.NewServer(router,
		ihttp.WithAddr(cfg.HTTP.Addr),
		ihttp.WithReadHeaderTimeout(cfg.HTTP.ReadHeaderTimeout),
		ihttp.WithShutdownTimeout(cfg.HTTP.ShutdownTimeout),
		ihttp.WithLogger(log),
	), nil
}
