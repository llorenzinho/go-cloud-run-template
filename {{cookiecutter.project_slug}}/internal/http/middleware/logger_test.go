package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLoggedRouter(t *testing.T, trusted []string) (*gin.Engine, *bytes.Buffer) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	r := gin.New()
	require.NoError(t, r.SetTrustedProxies(trusted))
	r.Use(Logger(slog.New(slog.NewJSONHandler(&buf, nil)), "/healthz"))
	r.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/objects", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r, &buf
}

func serve(r http.Handler, path, remote, xff string) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	r.ServeHTTP(httptest.NewRecorder(), req)
}

func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &m))
		out = append(out, m)
	}
	return out
}

func TestLoggerQuietPaths(t *testing.T) {
	r, buf := newLoggedRouter(t, nil)
	serve(r, "/healthz", "10.0.0.1:1234", "")
	assert.Empty(t, buf.String(), "successful probe not logged")

	var failBuf bytes.Buffer
	r2 := gin.New()
	r2.Use(Logger(slog.New(slog.NewJSONHandler(&failBuf, nil)), "/down"))
	r2.GET("/down", func(c *gin.Context) { c.Status(http.StatusServiceUnavailable) })
	serve(r2, "/down", "10.0.0.1:1234", "")
	logged := lines(t, &failBuf)
	require.Len(t, logged, 1, "failing probe is logged")
	assert.EqualValues(t, 503, logged[0]["status"])

	serve(r, "/objects", "10.0.0.1:1234", "")
	assert.Len(t, lines(t, buf), 1, "other paths are logged")
}

func TestLoggerCallerIP(t *testing.T) {
	tests := []struct {
		name     string
		trusted  []string
		xff      string
		wantPeer string
		wantIP   string
	}{
		{"no proxy trusted: header ignored", nil, "203.0.113.7", "10.0.0.5", "10.0.0.5"},
		{"trusted gateway: header used", []string{"10.0.0.0/8"}, "203.0.113.7", "10.0.0.5", "203.0.113.7"},
		{"untrusted peer: header ignored", []string{"192.168.0.0/16"}, "203.0.113.7", "10.0.0.5", "10.0.0.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, buf := newLoggedRouter(t, tt.trusted)
			serve(r, "/objects", "10.0.0.5:4321", tt.xff)
			logged := lines(t, buf)
			require.Len(t, logged, 1)
			assert.Equal(t, tt.wantPeer, logged[0]["remote_ip"])
			assert.Equal(t, tt.wantIP, logged[0]["client_ip"])
		})
	}
}
