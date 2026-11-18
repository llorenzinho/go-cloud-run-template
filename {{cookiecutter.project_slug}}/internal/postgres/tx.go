package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"{{ cookiecutter.module_path }}/internal/db/sqlc"
)

// rollbackTimeout bounds the rollback, which runs even when ctx is done.
const rollbackTimeout = 5 * time.Second

// InTx runs fn in a transaction: it commits when fn returns nil and rolls
// back when fn returns an error or panics. Pass tx to the sqlc methods; it
// applies the default query timeout like DBTX.
//
//	err := db.InTx(ctx, pgx.TxOptions{}, func(tx sqlc.DBTX) error {
//		if _, err := q.CreateItem(ctx, tx, a); err != nil {
//			return err
//		}
//		_, err := q.CreateItem(ctx, tx, b)
//		return err
//	})
func (d *DB) InTx(ctx context.Context, opts pgx.TxOptions, fn func(tx sqlc.DBTX) error) (err error) {
	tx, err := d.Pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			rollback(ctx, tx)
			panic(p)
		}
		if err != nil {
			rollback(ctx, tx)
		}
	}()

	if err := fn(timeoutDB{inner: tx, timeout: d.queryTimeout}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func rollback(ctx context.Context, tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()
	// A failed rollback makes pgx discard the connection: nothing else to do.
	_ = tx.Rollback(ctx)
}
