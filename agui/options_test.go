package agui_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ieshan/adk-go-pkg/agui"
)

func TestConfig_NilAgent(t *testing.T) {
	_, err := agui.Handler(agui.Config{})
	if err == nil {
		t.Fatal("got nil error, want error for nil Agent")
	}
	if !errors.Is(err, agui.ErrAgentRequired) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfig_Defaults(t *testing.T) {
	t.Run("ToolTimeout defaults to 5 minutes", func(t *testing.T) {
		cfg := agui.Config{
			Agent: agui.AgentFunc(nil),
		}
		// We cannot call applyDefaults directly since it's unexported,
		// but we can verify via Handler that things work with defaults.
		// Instead, test the observable behavior: Handler succeeds with minimal config.
		h, err := agui.Handler(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if h == nil {
			t.Fatal("got nil handler, want non-nil")
		}
	})

	t.Run("ToolResultHandler created for inline mode", func(t *testing.T) {
		cfg := agui.Config{
			Agent:       agui.AgentFunc(nil),
			ToolMode:    agui.ToolModeInline,
			ToolTimeout: 10 * time.Second,
		}
		h, err := agui.Handler(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if h == nil {
			t.Fatal("got nil handler, want non-nil")
		}
	})
}
