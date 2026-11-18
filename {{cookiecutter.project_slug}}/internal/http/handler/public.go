package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Public is an example of a route without authentication, mounted on the
// <base_path>/v1/public group: GET <base_path>/v1/public/ping.
type Public struct{}

// Register mounts the route on r (the public group).
func (Public) Register(r gin.IRoutes) {
	r.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
}
