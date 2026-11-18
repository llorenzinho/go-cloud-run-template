package repository

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"{{ cookiecutter.module_path }}/internal/db/sqlc"
	"{{ cookiecutter.module_path }}/internal/domain"
	"{{ cookiecutter.module_path }}/internal/postgres"
)

const pgUniqueViolation = "23505"

// Postgres is an ItemRepository backed by Postgres through sqlc. The sqlc
// methods take the DBTX: db.DBTX() outside transactions, the tx inside InTx.
type Postgres struct {
	db *postgres.DB
	q  sqlc.Querier
}

var _ ItemRepository = (*Postgres)(nil)

// NewPostgres returns a Postgres repository.
func NewPostgres(db *postgres.DB) *Postgres {
	return &Postgres{db: db, q: sqlc.New()}
}

// Create inserts item.
func (r *Postgres) Create(ctx context.Context, item *domain.Item) (*domain.Item, error) {
	row, err := r.q.CreateItem(ctx, r.db.DBTX(), sqlc.CreateItemParams{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
	})
	if err != nil {
		return nil, mapErr("create item", err)
	}
	return toDomain(row), nil
}

// Get returns an item by id.
func (r *Postgres) Get(ctx context.Context, id uuid.UUID) (*domain.Item, error) {
	row, err := r.q.GetItem(ctx, r.db.DBTX(), id)
	if err != nil {
		return nil, mapErr("get item", err)
	}
	return toDomain(row), nil
}

// List returns a page of items, newest first.
func (r *Postgres) List(ctx context.Context, p domain.Page) ([]*domain.Item, error) {
	rows, err := r.q.ListItems(ctx, r.db.DBTX(), sqlc.ListItemsParams{Limit: p.Limit, Offset: p.Offset})
	if err != nil {
		return nil, mapErr("list items", err)
	}
	items := make([]*domain.Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, toDomain(row))
	}
	return items, nil
}

// Update applies the non-nil fields of p. It shows a transaction: the row is
// locked first, so concurrent updates of the same item are serialized.
func (r *Postgres) Update(ctx context.Context, id uuid.UUID, p domain.ItemPatch) (*domain.Item, error) {
	var row sqlc.Item
	err := r.db.InTx(ctx, pgx.TxOptions{}, func(tx sqlc.DBTX) error {
		if _, err := r.q.GetItemForUpdate(ctx, tx, id); err != nil {
			return err
		}
		var err error
		row, err = r.q.UpdateItem(ctx, tx, sqlc.UpdateItemParams{ID: id, Name: p.Name, Description: p.Description})
		return err
	})
	if err != nil {
		return nil, mapErr("update item", err)
	}
	return toDomain(row), nil
}

// Delete removes an item.
func (r *Postgres) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := r.q.DeleteItem(ctx, r.db.DBTX(), id)
	if err != nil {
		return mapErr("delete item", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func toDomain(row sqlc.Item) *domain.Item {
	return &domain.Item{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func mapErr(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgUniqueViolation {
		return fmt.Errorf("%w: %s", domain.ErrConflict, pgErr.ConstraintName)
	}
	return fmt.Errorf("%s: %w", op, err)
}
