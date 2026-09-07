package eval_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"google.golang.org/genai"
)

func TestInMemoryEvalSetsManager_CRUD(t *testing.T) {
	ctx := context.Background()
	manager := eval.NewInMemoryEvalSetsManager()

	_, err := manager.CreateEvalSet(ctx, "app", "test-set")
	if err != nil {
		t.Fatalf("CreateEvalSet failed: %v", err)
	}

	err = manager.AddEvalCase(ctx, "app", "test-set", eval.EvalCase{
		EvalID: "case-1",
		Conversation: []eval.Invocation{{
			UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		}},
	})
	if err != nil {
		t.Fatalf("AddEvalCase failed: %v", err)
	}

	got, err := manager.GetEvalSet(ctx, "app", "test-set")
	if err != nil {
		t.Fatalf("GetEvalSet failed: %v", err)
	}
	if got.EvalSetID != "test-set" {
		t.Errorf("EvalSetID = %q, want %q", got.EvalSetID, "test-set")
	}
	if len(got.EvalCases) != 1 {
		t.Errorf("len(EvalCases) = %d, want 1", len(got.EvalCases))
	}

	// List eval sets.
	sets, err := manager.ListEvalSets(ctx, "app")
	if err != nil {
		t.Fatalf("ListEvalSets failed: %v", err)
	}
	if len(sets) != 1 {
		t.Errorf("len(sets) = %d, want 1", len(sets))
	}

	// Get eval case.
	case1, err := manager.GetEvalCase(ctx, "app", "test-set", "case-1")
	if err != nil {
		t.Fatalf("GetEvalCase failed: %v", err)
	}
	if case1.EvalID != "case-1" {
		t.Errorf("EvalID = %q, want %q", case1.EvalID, "case-1")
	}

	// Update eval case.
	err = manager.UpdateEvalCase(ctx, "app", "test-set", eval.EvalCase{
		EvalID: "case-1",
		Conversation: []eval.Invocation{{
			UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "updated"}}},
		}},
	})
	if err != nil {
		t.Fatalf("UpdateEvalCase failed: %v", err)
	}

	// Delete eval case.
	err = manager.DeleteEvalCase(ctx, "app", "test-set", "case-1")
	if err != nil {
		t.Fatalf("DeleteEvalCase failed: %v", err)
	}
	got, err = manager.GetEvalSet(ctx, "app", "test-set")
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

func TestInMemoryEvalSetsManager_DuplicateCreate(t *testing.T) {
	ctx := context.Background()
	manager := eval.NewInMemoryEvalSetsManager()

	_, err := manager.CreateEvalSet(ctx, "app", "dup-set")
	if err != nil {
		t.Fatalf("first CreateEvalSet failed: %v", err)
	}

	_, err = manager.CreateEvalSet(ctx, "app", "dup-set")
	if err == nil {
		t.Error("got nil error, want error for duplicate creation")
	}
}

func TestInMemoryEvalSetsManager_GetNotFound(t *testing.T) {
	ctx := context.Background()
	manager := eval.NewInMemoryEvalSetsManager()
	_, err := manager.GetEvalSet(ctx, "app", "nonexistent")
	if err == nil {
		t.Error("got nil error, want error for non-existent eval set")
	}
}
