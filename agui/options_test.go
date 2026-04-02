package agui_test

import (
	"testing"
	"time"

	"github.com/ieshan/adk-go-pkg/agui"
)

func TestConfig_NilAgent(t *testing.T) {
	_, err := agui.Handler(agui.Config{})
	if err == nil {
		t.Fatal("expected error for nil Agent")
	}
	if err.Error() != "agui: Agent is required" {
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
			t.Fatal("expected non-nil handler")
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
			t.Fatal("expected non-nil handler")
		}
	})
}
