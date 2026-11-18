package middleware

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"{{ cookiecutter.module_path }}/internal/domain"
)

// ErrorBody is the JSON error envelope.
type ErrorBody struct {
	Error     ErrorDetail `json:"error"`
	RequestID string      `json:"request_id,omitempty"`
}

// ErrorDetail describes an error.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Errors renders the last error attached with c.Error, unless a response was written.
func Errors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		WriteError(c, c.Errors.Last().Err)
	}
}

// WriteError maps err to a status code and writes the JSON envelope.
func WriteError(c *gin.Context, err error) {
	status, code, msg := classify(err)
	c.AbortWithStatusJSON(status, ErrorBody{
		Error:     ErrorDetail{Code: code, Message: msg},
		RequestID: GetRequestID(c),
	})
}

func classify(err error) (status int, code, msg string) {
	var maxBytes *http.MaxBytesError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found", "resource not found"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict", err.Error()
	case errors.Is(err, domain.ErrInvalidInput):
		// The code already says "invalid_input": keep only the details.
		return http.StatusBadRequest, "invalid_input", strings.TrimPrefix(err.Error(), domain.ErrInvalidInput.Error()+": ")
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "timeout", "request timed out"
	case errors.As(err, &maxBytes):
		return http.StatusRequestEntityTooLarge, "too_large", "request body too large"
	default:
		return http.StatusInternalServerError, "internal", "internal server error"
	}
}

// Recovery turns panics into a 500 JSON response.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, rec any) {
		log.ErrorContext(c.Request.Context(), "panic recovered", "panic", rec, "request_id", GetRequestID(c))
		WriteError(c, errors.New("panic"))
	})
}
