package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout puts a deadline d on the request context; handlers and queries see
// it through c.Request.Context() (gin falls back to it: ContextWithFallback).
// A call that runs out of time fails with context.DeadlineExceeded, rendered
// as 504 by Errors. 0 disables it.
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d <= 0 {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
