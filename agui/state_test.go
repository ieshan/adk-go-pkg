package agui_test

import (
	"sync"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ieshan/adk-go-pkg/agui"
)

func TestStateManager_NewWithInitialState(t *testing.T) {
	initial := map[string]any{"key": "value", "nested": map[string]any{"a": float64(1)}}
	sm, err := agui.NewStateManager(initial)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := sm.Snapshot().(map[string]any)

	if snap["key"] != "value" {
		t.Fatalf("got %v, want key=value", snap["key"])
	}
	nested := snap["nested"].(map[string]any)
	if nested["a"] != float64(1) {
		t.Fatalf("got %v, want nested.a=1", nested["a"])
	}

	// Mutating original should not affect state manager
	initial["key"] = "mutated"
	snap2 := sm.Snapshot().(map[string]any)
	if snap2["key"] != "value" {
		t.Fatalf("got %v, want value (initial mutation should not affect state manager)", snap2["key"])
	}
}

func TestStateManager_SnapshotDeepCopy(t *testing.T) {
	sm, err := agui.NewStateManager(map[string]any{"x": "original"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := sm.Snapshot().(map[string]any)
	snap["x"] = "mutated"

	snap2 := sm.Snapshot().(map[string]any)
	if snap2["x"] != "original" {
		t.Fatalf("got %v, want original (snapshot mutation should not affect internal state)", snap2["x"])
	}
}

func TestStateManager_Set(t *testing.T) {
	sm, err := agui.NewStateManager(map[string]any{"old": "data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := sm.Set(map[string]any{"new": "data"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snap := sm.Snapshot().(map[string]any)
	if _, ok := snap["old"]; ok {
		t.Fatal("old key should not exist after Set")
	}
	if snap["new"] != "data" {
		t.Fatalf("got %v, want new=data", snap["new"])
	}
}

func TestStateManager_Apply_Add(t *testing.T) {
	sm, err := agui.NewStateManager(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = sm.Apply([]events.JSONPatchOperation{
		{Op: "add", Path: "/greeting", Value: "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := sm.Snapshot().(map[string]any)
	if snap["greeting"] != "hello" {
		t.Fatalf("got %v, want greeting=hello", snap["greeting"])
	}
}

func TestStateManager_Apply_Remove(t *testing.T) {
	sm, err := agui.NewStateManager(map[string]any{"a": "1", "b": "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = sm.Apply([]events.JSONPatchOperation{
		{Op: "remove", Path: "/a"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := sm.Snapshot().(map[string]any)
	if _, ok := snap["a"]; ok {
		t.Fatal("key 'a' should have been removed")
	}
	if snap["b"] != "2" {
		t.Fatalf("got %v, want b=2", snap["b"])
	}
}

func TestStateManager_Apply_Replace(t *testing.T) {
	sm, err := agui.NewStateManager(map[string]any{"count": float64(1)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = sm.Apply([]events.JSONPatchOperation{
		{Op: "replace", Path: "/count", Value: float64(42)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := sm.Snapshot().(map[string]any)
	if snap["count"] != float64(42) {
		t.Fatalf("got %v, want count=42", snap["count"])
	}
}

func TestStateManager_Apply_Move(t *testing.T) {
	sm, err := agui.NewStateManager(map[string]any{"src": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = sm.Apply([]events.JSONPatchOperation{
		{Op: "move", From: "/src", Path: "/dst"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := sm.Snapshot().(map[string]any)
	if _, ok := snap["src"]; ok {
		t.Fatal("src should have been removed after move")
	}
	if snap["dst"] != "value" {
		t.Fatalf("got %v, want dst=value", snap["dst"])
	}
}

func TestStateManager_Apply_Copy(t *testing.T) {
	sm, err := agui.NewStateManager(map[string]any{"original": "data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = sm.Apply([]events.JSONPatchOperation{
		{Op: "copy", From: "/original", Path: "/duplicate"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := sm.Snapshot().(map[string]any)
	if snap["original"] != "data" {
		t.Fatalf("got %v, want data (original should still exist)", snap["original"])
	}
	if snap["duplicate"] != "data" {
		t.Fatalf("got %v, want duplicate=data", snap["duplicate"])
	}
}

func TestStateManager_Apply_Test(t *testing.T) {
	sm, err := agui.NewStateManager(map[string]any{"status": "active"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test success: value matches
	err = sm.Apply([]events.JSONPatchOperation{
		{Op: "test", Path: "/status", Value: "active"},
	})
	if err != nil {
		t.Fatalf("test should pass for matching value: %v", err)
	}

	// Test failure: value mismatch
	err = sm.Apply([]events.JSONPatchOperation{
		{Op: "test", Path: "/status", Value: "inactive"},
	})
	if err == nil {
		t.Fatal("test should fail for mismatching value")
	}
}

func TestStateManager_Diff(t *testing.T) {
	sm, err := agui.NewStateManager(map[string]any{
		"keep":   "same",
		"change": "old",
		"remove": "me",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	newState := map[string]any{
		"keep":   "same",
		"change": "new",
		"added":  "fresh",
	}

	ops, err := sm.Diff(newState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build a map of operations for easier assertions
	opMap := make(map[string]events.JSONPatchOperation)
	for _, op := range ops {
		opMap[op.Path] = op
	}

	// "keep" should not appear (no change)
	if _, ok := opMap["/keep"]; ok {
		t.Fatal("unchanged key should not produce a patch op")
	}

	// "change" should be a replace
	if op, ok := opMap["/change"]; !ok || op.Op != "replace" || op.Value != "new" {
		t.Fatalf("got %+v, want replace /change=new", opMap["/change"])
	}

	// "remove" should be a remove
	if op, ok := opMap["/remove"]; !ok || op.Op != "remove" {
		t.Fatalf("got %+v, want remove /remove", opMap["/remove"])
	}

	// "added" should be an add
	if op, ok := opMap["/added"]; !ok || op.Op != "add" || op.Value != "fresh" {
		t.Fatalf("got %+v, want add /added=fresh", opMap["/added"])
	}
}

func TestStateManager_ConcurrentAccess(t *testing.T) {
	sm, err := agui.NewStateManager(map[string]any{"counter": float64(0)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent writers
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if err := sm.Apply([]events.JSONPatchOperation{
				{Op: "replace", Path: "/counter", Value: float64(n)},
			}); err != nil {
				t.Errorf("concurrent Apply: %v", err)
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sm.Snapshot()
		}()
	}

	// Concurrent Set calls
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if err := sm.Set(map[string]any{"counter": float64(n)}); err != nil {
				t.Errorf("concurrent Set: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify state is still valid (any value is fine, just no panic/corruption)
	snap := sm.Snapshot().(map[string]any)
	if _, ok := snap["counter"]; !ok {
		t.Fatal("counter key should still exist after concurrent access")
	}
}
