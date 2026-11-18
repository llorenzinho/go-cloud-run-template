// Package service holds the business logic. It runs the sqlc queries: pass
// db.DBTX() outside transactions, the tx of db.InTx inside them.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"{{ cookiecutter.module_path }}/internal/db/sqlc"
	"{{ cookiecutter.module_path }}/internal/domain"
	"{{ cookiecutter.module_path }}/internal/postgres"
)

const pgUniqueViolation = "23505"

// ItemService manages items. Errors are domain errors (domain.ErrNotFound,
// domain.ErrConflict) or wrapped database errors.
type ItemService struct {
	db  *postgres.DB
	q   *sqlc.Queries
	log *slog.Logger
}

// NewItemService returns an ItemService.
func NewItemService(db *postgres.DB, log *slog.Logger) *ItemService {
	return &ItemService{db: db, q: sqlc.New(), log: log}
}

// Create stores a new item.
func (s *ItemService) Create(ctx context.Context, name, description string) (sqlc.Item, error) {
	item, err := s.q.CreateItem(ctx, s.db.DBTX(), sqlc.CreateItemParams{ID: uuid.NewV7(), Name: name, Description: description})
	if err != nil {
		return sqlc.Item{}, mapErr("create item", err)
	}
	s.log.InfoContext(ctx, "item created", "id", item.ID)
	return item, nil
}

// Get returns an item.
func (s *ItemService) Get(ctx context.Context, id uuid.UUID) (sqlc.Item, error) {
	item, err := s.q.GetItem(ctx, s.db.DBTX(), id)
	if err != nil {
		return sqlc.Item{}, mapErr("get item", err)
	}
	return item, nil
}

// List returns a page of items, newest first.
func (s *ItemService) List(ctx context.Context, limit, offset int32) ([]sqlc.Item, error) {
	items, err := s.q.ListItems(ctx, s.db.DBTX(), sqlc.ListItemsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, mapErr("list items", err)
	}
	return items, nil
}

// Update changes the non-nil fields. It shows a transaction: the row is
// locked first, so concurrent updates of the same item are serialized.
func (s *ItemService) Update(ctx context.Context, id uuid.UUID, name, description *string) (sqlc.Item, error) {
	var item sqlc.Item
	err := s.db.InTx(ctx, pgx.TxOptions{}, func(tx sqlc.DBTX) error {
		if _, err := s.q.GetItemForUpdate(ctx, tx, id); err != nil {
			return err
		}
		var err error
		item, err = s.q.UpdateItem(ctx, tx, sqlc.UpdateItemParams{ID: id, Name: name, Description: description})
		return err
	})
	if err != nil {
		return sqlc.Item{}, mapErr("update item", err)
	}
	return item, nil
}

// Delete removes an item.
func (s *ItemService) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := s.q.DeleteItem(ctx, s.db.DBTX(), id)
	if err != nil {
		return mapErr("delete item", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
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
