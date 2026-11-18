package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.ContextWithFallback = true
	r.Use(Errors(), Timeout(20*time.Millisecond))
	r.GET("/slow", func(c *gin.Context) {
		<-c.Done()
		_ = c.Error(c.Err())
	})
	r.GET("/deadline", func(c *gin.Context) {
		_, ok := c.Deadline()
		c.JSON(http.StatusOK, ok)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/slow", nil))
	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/deadline", nil))
	assert.Equal(t, "true", rec.Body.String())
}
