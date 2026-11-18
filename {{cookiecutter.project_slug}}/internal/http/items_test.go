package http_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{ cookiecutter.module_path }}/internal/domain"
)

// memRepo is an in-memory repository.ItemRepository.
type memRepo struct {
	mu    sync.Mutex
	items map[uuid.UUID]*domain.Item
}

func (m *memRepo) Create(_ context.Context, i *domain.Item) (*domain.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, other := range m.items {
		if other.Name == i.Name {
			return nil, domain.ErrConflict
		}
	}
	m.items[i.ID] = i
	return i, nil
}

func (m *memRepo) Get(_ context.Context, id uuid.UUID) (*domain.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i, ok := m.items[id]; ok {
		return i, nil
	}
	return nil, domain.ErrNotFound
}

func (m *memRepo) List(context.Context, domain.Page) ([]*domain.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*domain.Item, 0, len(m.items))
	for _, i := range m.items {
		out = append(out, i)
	}
	return out, nil
}

func (m *memRepo) Update(ctx context.Context, id uuid.UUID, p domain.ItemPatch) (*domain.Item, error) {
	i, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Name != nil {
		i.Name = *p.Name
	}
	if p.Description != nil {
		i.Description = *p.Description
	}
	return i, nil
}

func (m *memRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := m.Get(ctx, id); err != nil {
		return err
	}
	delete(m.items, id)
	return nil
}

func TestItemsCRUD(t *testing.T) {
	r := newRouter(t)

	code, created := send(t, r, http.MethodPost, "/api/v1/items", `{"name":"first","description":"d"}`)
	require.Equal(t, http.StatusCreated, code)
	id := created["id"].(string)

	code, _ = send(t, r, http.MethodPost, "/api/v1/items", `{"name":"first"}`)
	assert.Equal(t, http.StatusConflict, code)

	code, got := send(t, r, http.MethodPatch, "/api/v1/items/"+id, `{"description":"changed"}`)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "first", got["name"])
	assert.Equal(t, "changed", got["description"])

	code, list := send(t, r, http.MethodGet, "/api/v1/items?limit=10", "")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, list["items"], 1)

	code, _ = send(t, r, http.MethodDelete, "/api/v1/items/"+id, "")
	assert.Equal(t, http.StatusNoContent, code)
	code, _ = send(t, r, http.MethodGet, "/api/v1/items/"+id, "")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestValidationErrors(t *testing.T) {
	r := newRouter(t)
	tests := []struct {
		name, method, path, body, want string
	}{
		{"blank name", http.MethodPost, "/api/v1/items", `{"name":"  "}`, "name: is required"},
		{"bad json", http.MethodPost, "/api/v1/items", `{`, "unexpected EOF"},
		{"bad id", http.MethodGet, "/api/v1/items/nope", "", "id: must be a valid UUID"},
		{"bad limit", http.MethodGet, "/api/v1/items?limit=0", "", "limit: must be greater than or equal to 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := send(t, r, tt.method, tt.path, tt.body)
			assert.Equal(t, http.StatusBadRequest, code)
			assert.Equal(t, "invalid_input", body["error"].(map[string]any)["code"])
			assert.Contains(t, body["error"].(map[string]any)["message"], tt.want)
		})
	}
}

