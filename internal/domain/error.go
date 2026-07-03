package domain

import "errors"

var (
	ErrValidate  = errors.New("validation failed")
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrQueueFull = errors.New("chanel overcrowded")
)
