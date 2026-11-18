//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ihttp "{{ cookiecutter.module_path }}/internal/http"
	"{{ cookiecutter.module_path }}/internal/http/handler"
	"{{ cookiecutter.module_path }}/internal/repository"
	"{{ cookiecutter.module_path }}/internal/service"
)

func withItems(d *ihttp.Deps, st stack) {
	d.Items = handler.NewItems(service.NewItemService(repository.NewPostgres(st.DB), st.Log))
}

func call(t *testing.T, method, url string, body any) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, url, r)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	var out map[string]any
	if res.StatusCode != http.StatusNoContent {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&out))
	}
	return res.StatusCode, out
}

func TestItemsAgainstPostgres(t *testing.T) {
	base := startServer(t, withItems)
	api := base + ihttp.APIPath("{{ cookiecutter.base_path }}") + "/items"

	code, created := call(t, http.MethodPost, api, map[string]string{"name": "widget", "description": "blue"})
	require.Equal(t, http.StatusCreated, code)
	id := created["id"].(string)

	code, _ = call(t, http.MethodPost, api, map[string]string{"name": "widget"})
	assert.Equal(t, http.StatusConflict, code, "unique name")

	code, got := call(t, http.MethodPatch, api+"/"+id, map[string]string{"name": "gadget"})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "gadget", got["name"])
	assert.Equal(t, "blue", got["description"], "omitted fields unchanged")

	code, list := call(t, http.MethodGet, api+"?limit=5", nil)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, list["items"], 1)

	code, _ = call(t, http.MethodDelete, api+"/"+id, nil)
	assert.Equal(t, http.StatusNoContent, code)
	code, _ = call(t, http.MethodDelete, api+"/"+id, nil)
	assert.Equal(t, http.StatusNotFound, code)
}
