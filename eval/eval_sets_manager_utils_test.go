package eval_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

// fakeEvalSetsManager is a test stub for EvalSetsManager.
type fakeEvalSetsManager struct {
	evalSet *eval.EvalSet
	err     error
}

func (f *fakeEvalSetsManager) GetEvalSet(ctx context.Context, appName, evalSetID string) (*eval.EvalSet, error) {
	return f.evalSet, f.err
}

func (f *fakeEvalSetsManager) CreateEvalSet(ctx context.Context, appName, evalSetID string) (*eval.EvalSet, error) {
	return nil, nil
}

func (f *fakeEvalSetsManager) ListEvalSets(ctx context.Context, appName string) ([]string, error) {
	return nil, nil
}

func (f *fakeEvalSetsManager) GetEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) (*eval.EvalCase, error) {
	return nil, nil
}

func (f *fakeEvalSetsManager) AddEvalCase(ctx context.Context, appName, evalSetID string, evalCase eval.EvalCase) error {
	return nil
}

func (f *fakeEvalSetsManager) UpdateEvalCase(ctx context.Context, appName, evalSetID string, evalCase eval.EvalCase) error {
	return nil
}

func (f *fakeEvalSetsManager) DeleteEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) error {
	return nil
}

func TestGetEvalSetFromAppAndID_Success(t *testing.T) {
	mgr := &fakeEvalSetsManager{evalSet: &eval.EvalSet{EvalSetID: "test_set"}}
	es, err := eval.GetEvalSetFromAppAndID(context.Background(), mgr, "app", "test_set")
	if err != nil {
		t.Fatalf("GetEvalSetFromAppAndID failed: %v", err)
	}
	if es == nil || es.EvalSetID != "test_set" {
		t.Errorf("got %v, want test_set", es)
	}
}

func TestGetEvalSetFromAppAndID_NotFound(t *testing.T) {
	mgr := &fakeEvalSetsManager{evalSet: nil}
	_, err := eval.GetEvalSetFromAppAndID(context.Background(), mgr, "app", "missing")
	if err == nil {
		t.Fatalf("got nil error, want non-nil error for missing eval set")
	}
	var nf *eval.NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("got %T: %v, want NotFoundError", err, err)
	}
}

func TestGetEvalSetFromAppAndID_NilManager(t *testing.T) {
	_, err := eval.GetEvalSetFromAppAndID(context.Background(), nil, "app", "test_set")
	if err == nil {
		t.Errorf("got nil error, want non-nil error for nil manager")
	}
}

func TestGetEvalSetFromAppAndID_ManagerError(t *testing.T) {
	mgr := &fakeEvalSetsManager{err: errors.New("disk error")}
	_, err := eval.GetEvalSetFromAppAndID(context.Background(), mgr, "app", "test_set")
	if err == nil {
		t.Errorf("got nil error, want non-nil error from manager")
	}
}

func TestGetEvalCaseFromEvalSet_Found(t *testing.T) {
	evalSet := &eval.EvalSet{
		EvalSetID: "test_set",
		EvalCases: []eval.EvalCase{
			{EvalID: "case1"},
			{EvalID: "case2"},
		},
	}
	ec := eval.GetEvalCaseFromEvalSet(evalSet, "case2")
	if ec == nil {
		t.Fatalf("got nil, want case2")
	}
	if ec.EvalID != "case2" {
		t.Errorf("got %s, want case2", ec.EvalID)
	}
}

func TestGetEvalCaseFromEvalSet_NotFound(t *testing.T) {
	evalSet := &eval.EvalSet{EvalSetID: "test_set"}
	ec := eval.GetEvalCaseFromEvalSet(evalSet, "nonexistent")
	if ec != nil {
		t.Errorf("got non-nil, want nil for nonexistent case")
	}
}

func TestGetEvalCaseFromEvalSet_NilSet(t *testing.T) {
	ec := eval.GetEvalCaseFromEvalSet(nil, "case1")
	if ec != nil {
		t.Errorf("got non-nil, want nil for nil eval set")
	}
}

func TestAddEvalCaseToEvalSet_Success(t *testing.T) {
	evalSet := eval.NewEvalSet("test_set")
	ec := eval.EvalCase{EvalID: "case1"}
	updated, err := eval.AddEvalCaseToEvalSet(evalSet, ec)
	if err != nil {
		t.Fatalf("AddEvalCaseToEvalSet failed: %v", err)
	}
	if len(updated.EvalCases) != 1 {
		t.Errorf("got %d cases, want 1", len(updated.EvalCases))
	}
}

func TestAddEvalCaseToEvalSet_Duplicate(t *testing.T) {
	evalSet := &eval.EvalSet{
		EvalSetID: "test_set",
		EvalCases: []eval.EvalCase{{EvalID: "case1"}},
	}
	_, err := eval.AddEvalCaseToEvalSet(evalSet, eval.EvalCase{EvalID: "case1"})
	if err == nil {
		t.Errorf("got nil error, want non-nil error for duplicate case")
	}
}

func TestUpdateEvalCaseInEvalSet_Success(t *testing.T) {
	evalSet := &eval.EvalSet{
		EvalSetID: "test_set",
		EvalCases: []eval.EvalCase{{EvalID: "case1", SessionInput: &eval.SessionInput{AppName: "old"}}},
	}
	updated, err := eval.UpdateEvalCaseInEvalSet(evalSet, eval.EvalCase{EvalID: "case1", SessionInput: &eval.SessionInput{AppName: "new"}})
	if err != nil {
		t.Fatalf("UpdateEvalCaseInEvalSet failed: %v", err)
	}
	if updated.EvalCases[0].SessionInput.AppName != "new" {
		t.Errorf("got %q, want new", updated.EvalCases[0].SessionInput.AppName)
	}
}

func TestUpdateEvalCaseInEvalSet_NotFound(t *testing.T) {
	evalSet := eval.NewEvalSet("test_set")
	_, err := eval.UpdateEvalCaseInEvalSet(evalSet, eval.EvalCase{EvalID: "nonexistent"})
	if err == nil {
		t.Errorf("got nil error, want non-nil error for nonexistent case")
	}
}

func TestDeleteEvalCaseFromEvalSet_Success(t *testing.T) {
	evalSet := &eval.EvalSet{
		EvalSetID: "test_set",
		EvalCases: []eval.EvalCase{{EvalID: "case1"}, {EvalID: "case2"}},
	}
	updated, err := eval.DeleteEvalCaseFromEvalSet(evalSet, "case1")
	if err != nil {
		t.Fatalf("DeleteEvalCaseFromEvalSet failed: %v", err)
	}
	if len(updated.EvalCases) != 1 {
		t.Errorf("got %d cases after delete, want 1", len(updated.EvalCases))
	}
	if updated.EvalCases[0].EvalID != "case2" {
		t.Errorf("got %s, want case2 to remain", updated.EvalCases[0].EvalID)
	}
}

func TestDeleteEvalCaseFromEvalSet_NotFound(t *testing.T) {
	evalSet := eval.NewEvalSet("test_set")
	_, err := eval.DeleteEvalCaseFromEvalSet(evalSet, "nonexistent")
	if err == nil {
		t.Errorf("got nil error, want non-nil error for deleting nonexistent case")
	}
}
