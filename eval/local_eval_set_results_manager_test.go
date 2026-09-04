package eval

import (
	"context"
	"testing"
)

func newTestEvalSetResultsManager(t *testing.T) *LocalEvalSetResultsManager {
	t.Helper()
	mgr, err := NewLocalEvalSetResultsManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalEvalSetResultsManager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	return mgr
}

func TestLocalEvalSetResultsManager_SaveAndGet(t *testing.T) {
	ctx := context.Background()
	mgr := newTestEvalSetResultsManager(t)

	results := []EvalCaseResult{
		{EvalID: "case-1", FinalEvalStatus: EvalStatusPassed},
		{EvalID: "case-2", FinalEvalStatus: EvalStatusFailed},
	}

	err := mgr.SaveEvalSetResult(ctx, "app", "test-set", results)
	if err != nil {
		t.Fatalf("SaveEvalSetResult failed: %v", err)
	}

	list, err := mgr.ListEvalSetResults(ctx, "app")
	if err != nil {
		t.Fatalf("ListEvalSetResults failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}

	got, err := mgr.GetEvalSetResult(ctx, "app", list[0])
	if err != nil {
		t.Fatalf("GetEvalSetResult failed: %v", err)
	}
	if len(got.EvalCaseResults) != 2 {
		t.Errorf("len(EvalCaseResults) = %d, want 2", len(got.EvalCaseResults))
	}
	if got.EvalCaseResults[0].EvalID != "case-1" {
		t.Errorf("EvalCaseResults[0].EvalID = %q, want case-1", got.EvalCaseResults[0].EvalID)
	}
}

func TestLocalEvalSetResultsManager_GetNotFound(t *testing.T) {
	ctx := context.Background()
	mgr := newTestEvalSetResultsManager(t)

	_, err := mgr.GetEvalSetResult(ctx, "app", "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent result")
	}
}

func TestLocalEvalSetResultsManager_ListEmpty(t *testing.T) {
	ctx := context.Background()
	mgr := newTestEvalSetResultsManager(t)

	list, err := mgr.ListEvalSetResults(ctx, "app")
	if err != nil {
		t.Fatalf("ListEvalSetResults failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("len(list) = %d, want 0", len(list))
	}
}

func TestLocalEvalSetResultsManager_InvalidPath(t *testing.T) {
	ctx := context.Background()
	mgr := newTestEvalSetResultsManager(t)

	err := mgr.SaveEvalSetResult(ctx, "../etc", "test", nil)
	if err == nil {
		t.Error("expected error for path traversal in appName")
	}
}
