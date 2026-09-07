package eval_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"google.golang.org/genai"
)

func newTestEvalSetsManager(t *testing.T) *eval.LocalEvalSetsManager {
	t.Helper()
	mgr, err := eval.NewLocalEvalSetsManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalEvalSetsManager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	return mgr
}

func TestLocalEvalSetsManager_CRUD(t *testing.T) {
	ctx := context.Background()
	mgr := newTestEvalSetsManager(t)

	_, err := mgr.CreateEvalSet(ctx, "app", "test-set")
	if err != nil {
		t.Fatalf("CreateEvalSet failed: %v", err)
	}

	err = mgr.AddEvalCase(ctx, "app", "test-set", eval.EvalCase{
		EvalID: "case-1",
		Conversation: []eval.Invocation{{
			UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		}},
	})
	if err != nil {
		t.Fatalf("AddEvalCase failed: %v", err)
	}

	got, err := mgr.GetEvalSet(ctx, "app", "test-set")
	if err != nil {
		t.Fatalf("GetEvalSet failed: %v", err)
	}
	if got.EvalSetID != "test-set" {
		t.Errorf("EvalSetID = %q, want %q", got.EvalSetID, "test-set")
	}
	if len(got.EvalCases) != 1 {
		t.Errorf("len(EvalCases) = %d, want 1", len(got.EvalCases))
	}

	sets, err := mgr.ListEvalSets(ctx, "app")
	if err != nil {
		t.Fatalf("ListEvalSets failed: %v", err)
	}
	if len(sets) != 1 || sets[0] != "test-set" {
		t.Errorf("ListEvalSets = %v, want [test-set]", sets)
	}

	case1, err := mgr.GetEvalCase(ctx, "app", "test-set", "case-1")
	if err != nil {
		t.Fatalf("GetEvalCase failed: %v", err)
	}
	if case1.EvalID != "case-1" {
		t.Errorf("EvalID = %q, want case-1", case1.EvalID)
	}

	err = mgr.UpdateEvalCase(ctx, "app", "test-set", eval.EvalCase{
		EvalID: "case-1",
		Conversation: []eval.Invocation{{
			UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "updated"}}},
		}},
	})
	if err != nil {
		t.Fatalf("UpdateEvalCase failed: %v", err)
	}

	err = mgr.DeleteEvalCase(ctx, "app", "test-set", "case-1")
	if err != nil {
		t.Fatalf("DeleteEvalCase failed: %v", err)
	}
	got, err = mgr.GetEvalSet(ctx, "app", "test-set")
	if err != nil {
		t.Fatalf("GetEvalSet after delete: %v", err)
	}
	if got == nil {
		t.Fatal("nil eval set")
	}
	if len(got.EvalCases) != 0 {
		t.Errorf("len(EvalCases) after delete = %d, want 0", len(got.EvalCases))
	}
}

func TestLocalEvalSetsManager_DuplicateCreate(t *testing.T) {
	ctx := context.Background()
	mgr := newTestEvalSetsManager(t)

	_, err := mgr.CreateEvalSet(ctx, "app", "dup-set")
	if err != nil {
		t.Fatalf("first CreateEvalSet failed: %v", err)
	}

	_, err = mgr.CreateEvalSet(ctx, "app", "dup-set")
	if err == nil {
		t.Errorf("got nil error, want non-nil error for duplicate creation")
	}
}

func TestLocalEvalSetsManager_GetNotFound(t *testing.T) {
	ctx := context.Background()
	mgr := newTestEvalSetsManager(t)

	_, err := mgr.GetEvalSet(ctx, "app", "nonexistent")
	if err == nil {
		t.Errorf("got nil error, want non-nil error for non-existent eval set")
	}
}

func TestLocalEvalSetsManager_InvalidPath(t *testing.T) {
	ctx := context.Background()
	mgr := newTestEvalSetsManager(t)

	_, err := mgr.GetEvalSet(ctx, "../etc", "test")
	if err == nil {
		t.Errorf("got nil error, want non-nil error for path traversal in appName")
	}
}

func TestLocalEvalSetsManager_ListEmpty(t *testing.T) {
	ctx := context.Background()
	mgr := newTestEvalSetsManager(t)

	sets, err := mgr.ListEvalSets(ctx, "app")
	if err != nil {
		t.Fatalf("ListEvalSets failed: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("len(sets) = %d, want 0", len(sets))
	}
}

func TestLocalEvalSetsManager_OldFormatMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Create old format file (array of eval cases with query/reference fields).
	evalDir := filepath.Join(dir, "app", "eval")
	if err := os.MkdirAll(evalDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	oldData := `[
		{"query": "what is the weather?", "reference": "It is sunny."}
	]`
	oldPath := filepath.Join(evalDir, "old-set.evalset.json")
	if err := os.WriteFile(oldPath, []byte(oldData), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	mgr, err := eval.NewLocalEvalSetsManager(dir)
	if err != nil {
		t.Fatalf("NewLocalEvalSetsManager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	got, err := mgr.GetEvalSet(ctx, "app", "old-set")
	if err != nil {
		t.Fatalf("GetEvalSet with old format failed: %v", err)
	}
	// Old format doesn't have evalSetId, so it will be empty.
	// The migration creates eval cases from query/reference fields.
	if len(got.EvalCases) != 1 {
		t.Errorf("len(EvalCases) = %d, want 1", len(got.EvalCases))
	}
	if got.EvalCases[0].EvalID != "eval_case_0" {
		t.Errorf("EvalID = %q, want eval_case_0", got.EvalCases[0].EvalID)
	}
}
