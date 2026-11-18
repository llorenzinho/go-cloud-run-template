//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{ cookiecutter.module_path }}/db"
	"{{ cookiecutter.module_path }}/internal/db/sqlc"
	"{{ cookiecutter.module_path }}/internal/postgres"
	"{{ cookiecutter.module_path }}/internal/testutil"
)

func TestSchemaAndCheck(t *testing.T) {
	database, _ := testutil.Postgres(t)
	require.NoError(t, database.Check(t.Context()))

	schema, err := sqlc.New().CurrentSchema(t.Context(), database.DBTX())
	require.NoError(t, err)
	assert.Equal(t, db.Schema, schema)

	var table string
	require.NoError(t, database.Pool.QueryRow(t.Context(),
		"SELECT table_schema FROM information_schema.tables WHERE table_name = 'goose_db_version'").Scan(&table))
	assert.Equal(t, db.Schema, table, "goose version table in the service schema")
}

func TestQueryTimeout(t *testing.T) {
	database, _ := testutil.Postgres(t, postgres.WithQueryTimeout(50*time.Millisecond))

	start := time.Now()
	_, err := database.DBTX().Exec(t.Context(), "SELECT pg_sleep(2)")
	require.Error(t, err)
	assert.Less(t, time.Since(start), time.Second, "default query timeout applied")

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	_, err = database.DBTX().Exec(ctx, "SELECT pg_sleep(2)")
	require.Error(t, err, "an earlier caller deadline wins")

	rows, err := database.DBTX().Query(t.Context(), "SELECT generate_series(1, 3)")
	require.NoError(t, err)
	n := 0
	for rows.Next() {
		n++
	}
	rows.Close()
	require.NoError(t, rows.Err())
	assert.Equal(t, 3, n, "rows stay readable until closed")
}

func TestInTx(t *testing.T) {
	database, _ := testutil.Postgres(t)
	_, err := database.DBTX().Exec(t.Context(), "CREATE TABLE tx_probe (v int)")
	require.NoError(t, err)
	count := func() int {
		var n int
		require.NoError(t, database.DBTX().QueryRow(t.Context(), "SELECT count(*) FROM tx_probe").Scan(&n))
		return n
	}

	boom := errors.New("boom")
	err = database.InTx(t.Context(), pgx.TxOptions{}, func(tx sqlc.DBTX) error {
		if _, err := tx.Exec(t.Context(), "INSERT INTO tx_probe VALUES (1)"); err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.Equal(t, 0, count(), "rolled back on error")

	assert.Panics(t, func() {
		_ = database.InTx(t.Context(), pgx.TxOptions{}, func(tx sqlc.DBTX) error {
			_, _ = tx.Exec(t.Context(), "INSERT INTO tx_probe VALUES (1)")
			panic("boom")
		})
	})
	assert.Equal(t, 0, count(), "rolled back on panic")

	require.NoError(t, database.InTx(t.Context(), pgx.TxOptions{}, func(tx sqlc.DBTX) error {
		_, err := tx.Exec(t.Context(), "INSERT INTO tx_probe VALUES (1)")
		return err
	}))
	assert.Equal(t, 1, count(), "committed")
}

func TestConcurrentMigrations(t *testing.T) {
	database, _ := testutil.Postgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() { errs[i] = database.Migrate(t.Context(), "up", log) })
	}
	wg.Wait()
	for _, err := range errs {
		assert.NoError(t, err, "advisory lock serializes concurrent runs")
	}
}
