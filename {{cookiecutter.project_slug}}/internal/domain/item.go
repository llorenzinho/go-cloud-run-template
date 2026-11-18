package domain

import (
	"time"
	"uuid"
)

// Item is the example resource of the template: replace it with your own.
type Item struct {
	ID          uuid.UUID
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ItemPatch holds the fields to change; nil fields are left untouched.
type ItemPatch struct {
	Name        *string
	Description *string
}

// Page bounds a list query.
type Page struct {
	Limit  int32
	Offset int32
}
