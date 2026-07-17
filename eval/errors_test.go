package eval

import (
	"errors"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	if ErrNotFound == nil {
		t.Fatal("ErrNotFound should be non-nil")
	}
	if ErrAlreadyExists == nil {
		t.Fatal("ErrAlreadyExists should be non-nil")
	}
}

func TestNewNotFoundError(t *testing.T) {
	err := NewNotFoundError("eval set", "test-set")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Error("expected NotFoundError to wrap ErrNotFound")
	}
}

func TestNotFoundError_Unwrap(t *testing.T) {
	err := NewNotFoundError("eval case", "case-1")
	if !errors.Is(err, ErrNotFound) {
		t.Error("errors.Is(err, ErrNotFound) should be true")
	}
}

func TestNotFoundError_ErrorString(t *testing.T) {
	err := NewNotFoundError("eval set", "test-set")
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}
