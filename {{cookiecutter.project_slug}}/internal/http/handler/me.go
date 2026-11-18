package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"{{ cookiecutter.module_path }}/internal/auth"
)

// Me serves GET <base_path>/v1/me: the caller's identity, handy to check a
// token and the IdP configuration end to end.
type Me struct{}

// Register mounts the route on r (an authenticated group).
func (Me) Register(r gin.IRoutes) {
	r.GET("/me", func(c *gin.Context) {
		p, ok := auth.FromContext(c.Request.Context())
		if !ok {
			_ = c.Error(auth.ErrUnauthenticated)
			return
		}
		c.JSON(http.StatusOK, p)
	})
}
