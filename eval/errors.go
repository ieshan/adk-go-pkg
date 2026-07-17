package eval

import (
	"errors"
	"fmt"
)

// Sentinel errors for the eval package.
var (
	// ErrNotFound is returned when an eval set, eval case, or eval result
	// is not found.
	ErrNotFound = errors.New("eval: not found")

	// ErrAlreadyExists is returned when an eval set or eval case already exists.
	ErrAlreadyExists = errors.New("eval: already exists")
)

// NotFoundError wraps ErrNotFound with contextual detail.
type NotFoundError struct {
	Item string
	Name string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found", e.Item, e.Name)
}

func (e *NotFoundError) Unwrap() error { return ErrNotFound }

// NewNotFoundError creates a NotFoundError for the given item type and name.
func NewNotFoundError(item, name string) *NotFoundError {
	return &NotFoundError{Item: item, Name: name}
}
