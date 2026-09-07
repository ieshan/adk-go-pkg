package aguiadk_test

import (
	"testing"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"

	"github.com/ieshan/adk-go-pkg/aguiadk"
)

func TestPresets_AgenticChat(t *testing.T) {
	cfg := aguiadk.AgenticChatPreset(aguiadk.Config{})
	if cfg.EmitStateSnapshot == nil || !*cfg.EmitStateSnapshot {
		t.Errorf("EmitStateSnapshot = %v, want true", cfg.EmitStateSnapshot)
	}
	if cfg.SessionTimeout != 20*time.Minute {
		t.Errorf("SessionTimeout = %v, want 20m", cfg.SessionTimeout)
	}
	if cfg.ClientTools == nil {
		t.Fatal("got nil ClientTools, want non-nil")
	}
	if cfg.ClientTools.Mode != aguiadk.ClientToolModeNextRun {
		t.Errorf("ClientTools.Mode = %v, want NextRun", cfg.ClientTools.Mode)
	}
}

func TestPresets_AgenticChatPreservesBase(t *testing.T) {
	base := aguiadk.Config{AppName: "myapp"}
	cfg := aguiadk.AgenticChatPreset(base)
	if cfg.AppName != "myapp" {
		t.Errorf("AppName = %q, want %q (base field should be preserved)", cfg.AppName, "myapp")
	}
}

func TestPresets_GenerativeUI(t *testing.T) {
	cfg := aguiadk.GenerativeUIPreset(aguiadk.Config{})
	if cfg.EmitStateSnapshot == nil || !*cfg.EmitStateSnapshot {
		t.Errorf("EmitStateSnapshot = %v, want true", cfg.EmitStateSnapshot)
	}
	if cfg.ClientTools == nil {
		t.Fatal("got nil ClientTools, want non-nil")
	}
	if cfg.ClientTools.Mode != aguiadk.ClientToolModeNextRun {
		t.Errorf("ClientTools.Mode = %v, want NextRun", cfg.ClientTools.Mode)
	}
}

func TestPresets_HumanInTheLoop(t *testing.T) {
	cfg := aguiadk.HumanInTheLoopPreset(aguiadk.Config{}, false)
	if cfg.EmitStateSnapshot == nil || !*cfg.EmitStateSnapshot {
		t.Errorf("EmitStateSnapshot = %v, want true", cfg.EmitStateSnapshot)
	}
	if cfg.SessionTimeout != 30*time.Minute {
		t.Errorf("SessionTimeout = %v, want 30m", cfg.SessionTimeout)
	}
	if cfg.ClientTools == nil {
		t.Fatal("got nil ClientTools, want non-nil")
	}
	if cfg.ClientTools.Mode != aguiadk.ClientToolModeNextRun {
		t.Errorf("ClientTools.Mode = %v, want NextRun", cfg.ClientTools.Mode)
	}
	if cfg.RunStore == nil {
		t.Error("got nil RunStore, want non-nil when autoApprove=false")
	}
	t.Cleanup(cfg.RunStore.Stop)
}

func TestPresets_HumanInTheLoopAutoApprove(t *testing.T) {
	t.Parallel()
	cfg := aguiadk.HumanInTheLoopPreset(aguiadk.Config{}, true)
	if cfg.RunStore != nil {
		t.Errorf("RunStore = %v, want nil when autoApprove=true", cfg.RunStore)
	}
	if cfg.ApprovalModeFunc == nil {
		t.Fatal("got nil ApprovalModeFunc, want non-nil when autoApprove=true")
	}
}

func TestPresets_AutoApprove_FuncReturnsTrue(t *testing.T) {
	t.Parallel()
	cfg := aguiadk.HumanInTheLoopPreset(aguiadk.Config{}, true)
	if cfg.ApprovalModeFunc == nil {
		t.Fatal("got nil ApprovalModeFunc, want non-nil when autoApprove=true")
	}
	if got := cfg.ApprovalModeFunc(nil); !got {
		t.Errorf("ApprovalModeFunc(nil) = %v, want true", got)
	}
}

func TestPresets_SharedState(t *testing.T) {
	mapper := func(name string, args map[string]any) ([]events.JSONPatchOperation, bool) {
		return []events.JSONPatchOperation{
			{Op: "add", Path: "/tool/" + name, Value: args},
		}, true
	}
	cfg := aguiadk.SharedStatePreset(aguiadk.Config{}, mapper)
	if cfg.EmitStateSnapshot == nil || !*cfg.EmitStateSnapshot {
		t.Errorf("EmitStateSnapshot = %v, want true", cfg.EmitStateSnapshot)
	}
	if !cfg.SuppressToolEvents {
		t.Errorf("SuppressToolEvents = %v, want true", cfg.SuppressToolEvents)
	}
	if cfg.ToolToStateMapper == nil {
		t.Fatal("got nil ToolToStateMapper, want non-nil")
	}
	ops, _ := cfg.ToolToStateMapper("search", map[string]any{"q": "hello"})
	if len(ops) != 1 || ops[0].Path != "/tool/search" {
		t.Errorf("mapper ops = %v, want one op with path /tool/search", ops)
	}
}

func TestPresets_InlineTools(t *testing.T) {
	cfg := aguiadk.InlineToolsPreset(aguiadk.Config{})
	if cfg.EmitStateSnapshot == nil || !*cfg.EmitStateSnapshot {
		t.Errorf("EmitStateSnapshot = %v, want true", cfg.EmitStateSnapshot)
	}
	if cfg.ClientTools == nil {
		t.Fatal("got nil ClientTools, want non-nil")
	}
	if cfg.ClientTools.Mode != aguiadk.ClientToolModeInline {
		t.Errorf("ClientTools.Mode = %v, want Inline", cfg.ClientTools.Mode)
	}
	if cfg.ClientTools.Timeout != 5*time.Minute {
		t.Errorf("ClientTools.Timeout = %v, want 5m", cfg.ClientTools.Timeout)
	}
}

func TestPresets_PredictiveState(t *testing.T) {
	cfg := aguiadk.PredictiveStatePreset(aguiadk.Config{})
	if cfg.EmitStateSnapshot == nil || !*cfg.EmitStateSnapshot {
		t.Errorf("EmitStateSnapshot = %v, want true", cfg.EmitStateSnapshot)
	}
	if !cfg.EmitActivityDeltas {
		t.Errorf("EmitActivityDeltas = %v, want true", cfg.EmitActivityDeltas)
	}
	if cfg.EmitStepEvents == nil || !*cfg.EmitStepEvents {
		t.Errorf("EmitStepEvents = %v, want true", cfg.EmitStepEvents)
	}
	if cfg.SessionTimeout != 20*time.Minute {
		t.Errorf("SessionTimeout = %v, want 20m", cfg.SessionTimeout)
	}
}

func TestPresets_PredictiveStatePreservesBase(t *testing.T) {
	base := aguiadk.Config{AppName: "myapp"}
	cfg := aguiadk.PredictiveStatePreset(base)
	if cfg.AppName != "myapp" {
		t.Errorf("AppName = %q, want %q (base field should be preserved)", cfg.AppName, "myapp")
	}
}

func TestPresets_AgenticGenerativeUI(t *testing.T) {
	mapper := func(name string, args map[string]any) ([]events.JSONPatchOperation, bool) {
		return []events.JSONPatchOperation{
			{Op: "add", Path: "/ui/" + name, Value: args},
		}, true
	}
	cfg := aguiadk.AgenticGenerativeUIPreset(aguiadk.Config{}, mapper)
	if cfg.EmitStateSnapshot == nil || !*cfg.EmitStateSnapshot {
		t.Errorf("EmitStateSnapshot = %v, want true", cfg.EmitStateSnapshot)
	}
	if !cfg.EmitMessagesSnapshot {
		t.Errorf("EmitMessagesSnapshot = %v, want true", cfg.EmitMessagesSnapshot)
	}
	if cfg.EmitStepEvents == nil || !*cfg.EmitStepEvents {
		t.Errorf("EmitStepEvents = %v, want true", cfg.EmitStepEvents)
	}
	if !cfg.SuppressToolEvents {
		t.Errorf("SuppressToolEvents = %v, want true", cfg.SuppressToolEvents)
	}
	if cfg.ToolToStateMapper == nil {
		t.Fatal("got nil ToolToStateMapper, want non-nil")
	}
	ops, _ := cfg.ToolToStateMapper("render", map[string]any{"component": "card"})
	if len(ops) != 1 || ops[0].Path != "/ui/render" {
		t.Errorf("mapper ops = %v, want one op with path /ui/render", ops)
	}
	if cfg.SessionTimeout != 20*time.Minute {
		t.Errorf("SessionTimeout = %v, want 20m", cfg.SessionTimeout)
	}
}

func TestPresets_AgenticGenerativeUIPreservesBase(t *testing.T) {
	base := aguiadk.Config{AppName: "guiapp"}
	cfg := aguiadk.AgenticGenerativeUIPreset(base, func(string, map[string]any) ([]events.JSONPatchOperation, bool) {
		return nil, false
	})
	if cfg.AppName != "guiapp" {
		t.Errorf("AppName = %q, want %q (base field should be preserved)", cfg.AppName, "guiapp")
	}
}
