// Package handler contains the gin handlers.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"{{ cookiecutter.module_path }}/internal/health"
)

// Probe paths.
const (
	HealthzPath = "/healthz"
	ReadyzPath  = "/readyz"
)

// Health serves liveness and readiness probes.
type Health struct {
	registry *health.Registry
}

// NewHealth returns health handlers; the registry checks run on /readyz.
func NewHealth(registry *health.Registry) *Health {
	return &Health{registry: registry}
}

// Register mounts /healthz and /readyz on r.
func (h *Health) Register(r gin.IRoutes) {
	r.GET(HealthzPath, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": health.StatusUp}) })
	r.GET(ReadyzPath, h.ready)
}

// ready returns 200 with every check up, 503 otherwise.
func (h *Health) ready(c *gin.Context) {
	rep := h.registry.Run(c.Request.Context())
	status := http.StatusOK
	if !rep.Up() {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, rep)
}
