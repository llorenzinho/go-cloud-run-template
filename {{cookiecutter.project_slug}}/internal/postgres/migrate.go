package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"{{ cookiecutter.module_path }}/db"
)

// Migrate applies (up) or rolls back one (down) migration, or logs the status.
// Concurrent runs (several instances starting together) are serialized with a
// Postgres advisory lock. The version table is <schema>.goose_db_version.
func (d *DB) Migrate(ctx context.Context, command string, log *slog.Logger) error {
	// goose creates its version table before running 00001_init.sql, which
	// creates the schema: create it first.
	if _, err := d.Pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+pgx.Identifier{d.schema}.Sanitize()); err != nil {
		return fmt.Errorf("create schema %s: %w", d.schema, err)
	}

	sqlDB := stdlib.OpenDBFromPool(d.Pool)
	defer func() { _ = sqlDB.Close() }()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("goose locker: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, db.Migrations(),
		goose.WithTableName(d.schema+".goose_db_version"),
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}

	switch command {
	case "up":
		res, err := p.Up(ctx)
		for _, r := range res {
			log.InfoContext(ctx, "migration applied", "source", r.Source.Path, "duration", r.Duration)
		}
		return err
	case "down":
		r, err := p.Down(ctx)
		if r != nil {
			log.InfoContext(ctx, "migration rolled back", "source", r.Source.Path)
		}
		return err
	case "status":
		st, err := p.Status(ctx)
		for _, s := range st {
			log.InfoContext(ctx, "migration", "source", s.Source.Path, "state", s.State, "applied_at", s.AppliedAt)
		}
		return err
	default:
		return fmt.Errorf("unknown migrate command %q (up|down|status)", command)
	}
}
