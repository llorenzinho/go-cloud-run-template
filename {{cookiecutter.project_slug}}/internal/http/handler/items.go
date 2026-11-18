package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"
	"uuid"

	"github.com/gin-gonic/gin"

	"{{ cookiecutter.module_path }}/internal/domain"
{%- if cookiecutter.use_auth == 'y' %}
	"{{ cookiecutter.module_path }}/internal/http/middleware"
{%- endif %}
	"{{ cookiecutter.module_path }}/internal/service"
)

// Items serves the example CRUD API under <base_path>/v1/items.
type Items struct {
	svc *service.ItemService
}

// NewItems returns the items handlers.
func NewItems(svc *service.ItemService) *Items {
	return &Items{svc: svc}
}

{%- if cookiecutter.use_auth == 'y' %}

// RoleAdmin may create, change and delete items; any authenticated caller may read them.
const RoleAdmin = "admin"

// Register mounts the routes on r (an authenticated group).
func (h *Items) Register(r gin.IRoutes) {
	admin := middleware.RequireRole(RoleAdmin)
	r.POST("/items", admin, h.create)
	r.GET("/items", h.list)
	r.GET("/items/:id", h.get)
	r.PATCH("/items/:id", admin, h.update)
	r.DELETE("/items/:id", admin, h.delete)
}
{%- else %}

// Register mounts the routes on r.
func (h *Items) Register(r gin.IRoutes) {
	r.POST("/items", h.create)
	r.GET("/items", h.list)
	r.GET("/items/:id", h.get)
	r.PATCH("/items/:id", h.update)
	r.DELETE("/items/:id", h.delete)
}
{%- endif %}

// ItemResponse is the JSON representation of an item.
type ItemResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateItemRequest is the body of POST /items.
type CreateItemRequest struct {
	Name        string `json:"name" validate:"notblank,max=200"`
	Description string `json:"description" validate:"max=2000"`
}

// UpdateItemRequest is the body of PATCH /items/:id; omitted fields are unchanged.
type UpdateItemRequest struct {
	Name        *string `json:"name" validate:"omitnil,notblank,max=200"`
	Description *string `json:"description" validate:"omitnil,max=2000"`
}

// ListItemsQuery are the query parameters of GET /items.
type ListItemsQuery struct {
	Limit  int32 `form:"limit,default=50" json:"limit" validate:"gte=1,lte=200"`
	Offset int32 `form:"offset,default=0" json:"offset" validate:"gte=0"`
}

func toResponse(i *domain.Item) ItemResponse {
	return ItemResponse{
		ID:          i.ID.String(),
		Name:        i.Name,
		Description: i.Description,
		CreatedAt:   i.CreatedAt,
		UpdatedAt:   i.UpdatedAt,
	}
}

func (h *Items) create(c *gin.Context) {
	var req CreateItemRequest
	if !bind(c, c.ShouldBindJSON(&req)) {
		return
	}
	item, err := h.svc.Create(c, req.Name, req.Description)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toResponse(item))
}

func (h *Items) list(c *gin.Context) {
	var q ListItemsQuery
	if !bind(c, c.ShouldBindQuery(&q)) {
		return
	}
	items, err := h.svc.List(c, domain.Page{Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		_ = c.Error(err)
		return
	}
	out := make([]ItemResponse, 0, len(items))
	for _, i := range items {
		out = append(out, toResponse(i))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Items) get(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	item, err := h.svc.Get(c, id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toResponse(item))
}

func (h *Items) update(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req UpdateItemRequest
	if !bind(c, c.ShouldBindJSON(&req)) {
		return
	}
	item, err := h.svc.Update(c, id, domain.ItemPatch{Name: req.Name, Description: req.Description})
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toResponse(item))
}

func (h *Items) delete(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c, id); err != nil {
		_ = c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

// bind reports whether binding succeeded; otherwise it records a 400 error.
func bind(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if !errors.Is(err, domain.ErrInvalidInput) {
		err = fmt.Errorf("%w: %w", domain.ErrInvalidInput, err)
	}
	_ = c.Error(err)
	return false
}

func pathID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(fmt.Errorf("%w: id: must be a valid UUID", domain.ErrInvalidInput))
		return uuid.UUID{}, false
	}
	return id, true
}
