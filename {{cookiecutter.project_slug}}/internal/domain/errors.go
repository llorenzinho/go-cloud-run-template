// Package domain holds the business types and the sentinel errors shared across layers.
package domain

import "errors"

// Sentinel errors shared across layers; the HTTP layer maps them to status codes.
var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrInvalidInput = errors.New("invalid input")
)
