// Command {{ cookiecutter.project_slug }} runs the {{ cookiecutter.project_name }} service.
//
// Usage:
//
//	{{ cookiecutter.project_slug }}                     # serve HTTP
{%- if cookiecutter.use_postgres == 'y' %}
//	{{ cookiecutter.project_slug }} migrate up|down|status
{%- endif %}
//	{{ cookiecutter.project_slug }} version
package main

import (
	"context"
{%- if cookiecutter.use_postgres == 'y' %}
	"errors"
{%- endif %}
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"

	"{{ cookiecutter.module_path }}/internal/config"
	"{{ cookiecutter.module_path }}/internal/di"
	ihttp "{{ cookiecutter.module_path }}/internal/http"
{%- if cookiecutter.use_postgres == 'y' %}
	"{{ cookiecutter.module_path }}/internal/postgres"
{%- endif %}
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "{{ cookiecutter.project_slug }}:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "version" {
		fmt.Println(version)
		return nil
	}

	cfg, err := config.Load("")
	if err != nil {
		return err
	}
	if cfg.Env != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	injector := di.New(cfg, version)
	log := do.MustInvoke[*slog.Logger](injector)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if rep := injector.ShutdownWithContext(ctx); rep != nil && !rep.Succeed {
			log.Error("shutdown", "error", rep.Error())
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) > 0 {
{%- if cookiecutter.use_postgres == 'y' %}
		switch args[0] {
		case "migrate":
			if len(args) < 2 {
				return errors.New("usage: {{ cookiecutter.project_slug }} migrate up|down|status")
			}
			return migrate(ctx, injector, args[1], log)
		default:
			return fmt.Errorf("unknown command %q", args[0])
		}
{%- else %}
		return fmt.Errorf("unknown command %q", args[0])
{%- endif %}
	}
{%- if cookiecutter.use_postgres == 'y' %}

	if cfg.DB.AutoMigrate {
		if err := migrate(ctx, injector, "up", log); err != nil {
			return err
		}
	}
{%- endif %}

	srv, err := do.Invoke[*ihttp.Server](injector)
	if err != nil {
		return err
	}
	log.Info("starting {{ cookiecutter.project_slug }}", "env", cfg.Env)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run(ctx) }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		return nil // deferred injector shutdown stops the server first (it depends on everything else)
	}
}

{%- if cookiecutter.use_postgres == 'y' %}

func migrate(ctx context.Context, i do.Injector, command string, log *slog.Logger) error {
	db, err := do.Invoke[*postgres.DB](i)
	if err != nil {
		return err
	}
	return db.Migrate(ctx, command, log)
}
{%- endif %}
