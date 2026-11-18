package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"{{ cookiecutter.module_path }}/internal/db/sqlc"
)

// timeoutDB wraps a pool or a transaction: every call gets the default query
// timeout unless its context already ends sooner. The deadline of Query lasts
// until the rows are closed, the one of QueryRow until Scan.
type timeoutDB struct {
	inner   sqlc.DBTX
	timeout time.Duration
}

var _ sqlc.DBTX = timeoutDB{}

func (t timeoutDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	ctx, cancel := withTimeout(ctx, t.timeout)
	defer cancel()
	return t.inner.Exec(ctx, sql, args...)
}

func (t timeoutDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	ctx, cancel := withTimeout(ctx, t.timeout)
	rows, err := t.inner.Query(ctx, sql, args...)
	if err != nil {
		cancel()
		return nil, err
	}
	return &cancelRows{Rows: rows, cancel: cancel}, nil
}

func (t timeoutDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	ctx, cancel := withTimeout(ctx, t.timeout)
	return cancelRow{row: t.inner.QueryRow(ctx, sql, args...), cancel: cancel}
}

type cancelRows struct {
	pgx.Rows
	cancel context.CancelFunc
}

func (r *cancelRows) Close() {
	r.Rows.Close()
	r.cancel()
}

type cancelRow struct {
	row    pgx.Row
	cancel context.CancelFunc
}

func (r cancelRow) Scan(dest ...any) error {
	defer r.cancel()
	return r.row.Scan(dest...)
}

// withTimeout returns ctx with a deadline d from now, unless d is 0 or ctx
// already ends sooner.
func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return ctx, func() {}
	}
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= d {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}
