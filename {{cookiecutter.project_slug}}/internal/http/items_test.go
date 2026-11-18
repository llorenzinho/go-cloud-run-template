package http_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// CRUD runs against Postgres in test/integration; here only what fails before the database.
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

