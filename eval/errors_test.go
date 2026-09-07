package eval_test

import (
	"errors"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestSentinelErrors(t *testing.T) {
	if eval.ErrNotFound == nil {
		t.Fatal("ErrNotFound should be non-nil")
	}
	if eval.ErrAlreadyExists == nil {
		t.Fatal("ErrAlreadyExists should be non-nil")
	}
}

func TestNewNotFoundError(t *testing.T) {
	err := eval.NewNotFoundError("eval set", "test-set")
	if err == nil {
		t.Fatal("got nil error, want non-nil")
	}
	if !errors.Is(err, eval.ErrNotFound) {
		t.Error("got error not wrapping ErrNotFound, want ErrNotFound")
	}
}

func TestNotFoundError_Unwrap(t *testing.T) {
	err := eval.NewNotFoundError("eval case", "case-1")
	if !errors.Is(err, eval.ErrNotFound) {
		t.Error("errors.Is(err, ErrNotFound) should be true")
	}
}

func TestNotFoundError_ErrorString(t *testing.T) {
	err := eval.NewNotFoundError("eval set", "test-set")
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}
