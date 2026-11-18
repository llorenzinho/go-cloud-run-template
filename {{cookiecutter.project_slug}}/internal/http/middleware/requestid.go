// Package middleware contains the gin middlewares used by the service.
package middleware

import (
	"uuid"

	"github.com/gin-gonic/gin"
)

// HeaderRequestID is the request correlation header.
const HeaderRequestID = "X-Request-ID"

const ctxRequestID = "request_id"

// RequestID propagates X-Request-ID, generating one when missing.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" || len(id) > 128 {
			id = uuid.New().String()
		}
		c.Set(ctxRequestID, id)
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}

// GetRequestID returns the request id set by RequestID.
func GetRequestID(c *gin.Context) string {
	return c.GetString(ctxRequestID)
}
