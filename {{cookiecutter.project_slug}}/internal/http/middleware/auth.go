package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"{{ cookiecutter.module_path }}/internal/auth"
)

// Authenticate verifies the Bearer token of every request whose route is not
// under one of publicPrefixes (matched on the route pattern, not on the raw
// URL), and stores the principal in the request context (auth.FromContext).
//
// On Cloud Run the caller's Google identity for IAM travels in
// X-Serverless-Authorization (checked and removed by Cloud Run), so
// Authorization is left to the end-user token.
func Authenticate(a auth.Authenticator, log *slog.Logger, publicPrefixes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		route := c.FullPath()
		for _, p := range publicPrefixes {
			if route == p || strings.HasPrefix(route, p+"/") {
				c.Next()
				return
			}
		}

		// No token: "" (the Dev authenticator still logs in its principal).
		token, _ := bearer(c.GetHeader("Authorization"))
		p, err := a.Authenticate(c.Request.Context(), token)
		if err != nil {
			challenge := `Bearer error="invalid_token"`
			if errors.Is(err, auth.ErrMissingToken) {
				challenge = `Bearer`
			}
			unauthenticated(c, log, challenge, err)
			return
		}
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
		c.Next()
	}
}

// RequireRole lets the request through when the principal has at least one of
// roles; otherwise it answers 403. Use it after Authenticate.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := auth.FromContext(c.Request.Context())
		if !ok {
			WriteError(c, auth.ErrUnauthenticated)
			return
		}
		if !p.HasAnyRole(roles...) {
			WriteError(c, fmt.Errorf("%w: requires one of the roles %v", auth.ErrForbidden, roles))
			return
		}
		c.Next()
	}
}

// bearer extracts the token of an "Authorization: Bearer <token>" header (RFC 6750).
func bearer(h string) (string, bool) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(h), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func unauthenticated(c *gin.Context, log *slog.Logger, challenge string, err error) {
	// The reason stays in the logs: the client only learns that it failed.
	log.InfoContext(c.Request.Context(), "authentication failed",
		"reason", err.Error(), "path", c.FullPath(), "request_id", GetRequestID(c))
	c.Header("WWW-Authenticate", challenge)
	WriteError(c, auth.ErrUnauthenticated)
}
