// Package service holds the business logic, independent of HTTP and storage.
package service

import (
	"context"
	"log/slog"
	"uuid"

	"{{ cookiecutter.module_path }}/internal/domain"
	"{{ cookiecutter.module_path }}/internal/repository"
)

// ItemService manages items.
type ItemService struct {
	repo repository.ItemRepository
	log  *slog.Logger
}

// NewItemService returns an ItemService.
func NewItemService(repo repository.ItemRepository, log *slog.Logger) *ItemService {
	return &ItemService{repo: repo, log: log}
}

// Create stores a new item.
func (s *ItemService) Create(ctx context.Context, name, description string) (*domain.Item, error) {
	item, err := s.repo.Create(ctx, &domain.Item{ID: uuid.NewV7(), Name: name, Description: description})
	if err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "item created", "id", item.ID)
	return item, nil
}

// Get returns an item.
func (s *ItemService) Get(ctx context.Context, id uuid.UUID) (*domain.Item, error) {
	return s.repo.Get(ctx, id)
}

// List returns a page of items.
func (s *ItemService) List(ctx context.Context, p domain.Page) ([]*domain.Item, error) {
	return s.repo.List(ctx, p)
}

// Update changes the fields set in p.
func (s *ItemService) Update(ctx context.Context, id uuid.UUID, p domain.ItemPatch) (*domain.Item, error) {
	return s.repo.Update(ctx, id, p)
}

// Delete removes an item.
func (s *ItemService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
