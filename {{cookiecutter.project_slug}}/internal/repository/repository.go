// Package repository persists the domain types.
package repository

import (
	"context"
	"uuid"

	"{{ cookiecutter.module_path }}/internal/domain"
)

// ItemRepository persists items. Implementations return domain errors
// (domain.ErrNotFound, domain.ErrConflict).
type ItemRepository interface {
	Create(ctx context.Context, item *domain.Item) (*domain.Item, error)
	Get(ctx context.Context, id uuid.UUID) (*domain.Item, error)
	List(ctx context.Context, p domain.Page) ([]*domain.Item, error)
	Update(ctx context.Context, id uuid.UUID, p domain.ItemPatch) (*domain.Item, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
