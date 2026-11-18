package middleware

import (
	"log/slog"
	"net"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger logs one structured line per request. Requests to quietPaths (e.g.
// health probes) are logged only when they fail with a 5xx.
//
// remote_ip is the TCP peer, i.e. the service calling us (normally the Google
// front end on Cloud Run). client_ip is resolved from X-Forwarded-For / X-Real-IP only when the
// peer is a trusted proxy (http.trusted_proxies), otherwise it equals remote_ip.
func Logger(log *slog.Logger, quietPaths ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		if status < 500 && slices.Contains(quietPaths, c.Request.URL.Path) {
			return
		}
		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", c.FullPath()),
			slog.String("uri", c.Request.URL.RequestURI()),
			slog.Int("status", status),
			slog.Duration("latency", time.Since(start)),
			slog.String("remote_ip", remoteIP(c)),
			slog.String("client_ip", c.ClientIP()),
			slog.String("request_id", GetRequestID(c)),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.String("error", c.Errors.String()))
		}
		log.LogAttrs(c.Request.Context(), level, "http request", attrs...)
	}
}

func remoteIP(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}
