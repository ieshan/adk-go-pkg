package eval

import (
	"context"
	"errors"
	"testing"
)

// fakeEvalSetsManager is a test stub for EvalSetsManager.
type fakeEvalSetsManager struct {
	evalSet *EvalSet
	err     error
}

func (f *fakeEvalSetsManager) GetEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error) {
	return f.evalSet, f.err
}

func (f *fakeEvalSetsManager) CreateEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error) {
	return nil, nil
}

func (f *fakeEvalSetsManager) ListEvalSets(ctx context.Context, appName string) ([]string, error) {
	return nil, nil
}

func (f *fakeEvalSetsManager) GetEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) (*EvalCase, error) {
	return nil, nil
}

func (f *fakeEvalSetsManager) AddEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error {
	return nil
}

func (f *fakeEvalSetsManager) UpdateEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error {
	return nil
}

func (f *fakeEvalSetsManager) DeleteEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) error {
	return nil
}

func TestGetEvalSetFromAppAndID_Success(t *testing.T) {
	mgr := &fakeEvalSetsManager{evalSet: &EvalSet{EvalSetID: "test_set"}}
	es, err := GetEvalSetFromAppAndID(context.Background(), mgr, "app", "test_set")
	if err != nil {
		t.Fatalf("GetEvalSetFromAppAndID failed: %v", err)
	}
	if es == nil || es.EvalSetID != "test_set" {
		t.Errorf("expected test_set, got %v", es)
	}
}

func TestGetEvalSetFromAppAndID_NotFound(t *testing.T) {
	mgr := &fakeEvalSetsManager{evalSet: nil}
	_, err := GetEvalSetFromAppAndID(context.Background(), mgr, "app", "missing")
	if err == nil {
		t.Fatal("expected error for missing eval set")
	}
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestGetEvalSetFromAppAndID_NilManager(t *testing.T) {
	_, err := GetEvalSetFromAppAndID(context.Background(), nil, "app", "test_set")
	if err == nil {
		t.Error("expected error for nil manager")
	}
}

func TestGetEvalSetFromAppAndID_ManagerError(t *testing.T) {
	mgr := &fakeEvalSetsManager{err: errors.New("disk error")}
	_, err := GetEvalSetFromAppAndID(context.Background(), mgr, "app", "test_set")
	if err == nil {
		t.Error("expected error from manager")
	}
}

func TestGetEvalCaseFromEvalSet_Found(t *testing.T) {
	evalSet := &EvalSet{
		EvalSetID: "test_set",
		EvalCases: []EvalCase{
			{EvalID: "case1"},
			{EvalID: "case2"},
		},
	}
	ec := GetEvalCaseFromEvalSet(evalSet, "case2")
	if ec == nil {
		t.Fatal("expected to find case2")
	}
	if ec.EvalID != "case2" {
		t.Errorf("got %s, want case2", ec.EvalID)
	}
}

func TestGetEvalCaseFromEvalSet_NotFound(t *testing.T) {
	evalSet := &EvalSet{EvalSetID: "test_set"}
	ec := GetEvalCaseFromEvalSet(evalSet, "nonexistent")
	if ec != nil {
		t.Error("expected nil for nonexistent case")
	}
}

func TestGetEvalCaseFromEvalSet_NilSet(t *testing.T) {
	ec := GetEvalCaseFromEvalSet(nil, "case1")
	if ec != nil {
		t.Error("expected nil for nil eval set")
	}
}

func TestAddEvalCaseToEvalSet_Success(t *testing.T) {
	evalSet := NewEvalSet("test_set")
	ec := EvalCase{EvalID: "case1"}
	updated, err := AddEvalCaseToEvalSet(evalSet, ec)
	if err != nil {
		t.Fatalf("AddEvalCaseToEvalSet failed: %v", err)
	}
	if len(updated.EvalCases) != 1 {
		t.Errorf("expected 1 case, got %d", len(updated.EvalCases))
	}
}

func TestAddEvalCaseToEvalSet_Duplicate(t *testing.T) {
	evalSet := &EvalSet{
		EvalSetID: "test_set",
		EvalCases: []EvalCase{{EvalID: "case1"}},
	}
	_, err := AddEvalCaseToEvalSet(evalSet, EvalCase{EvalID: "case1"})
	if err == nil {
		t.Error("expected error for duplicate case")
	}
}

func TestUpdateEvalCaseInEvalSet_Success(t *testing.T) {
	evalSet := &EvalSet{
		EvalSetID: "test_set",
		EvalCases: []EvalCase{{EvalID: "case1", SessionInput: &SessionInput{AppName: "old"}}},
	}
	updated, err := UpdateEvalCaseInEvalSet(evalSet, EvalCase{EvalID: "case1", SessionInput: &SessionInput{AppName: "new"}})
	if err != nil {
		t.Fatalf("UpdateEvalCaseInEvalSet failed: %v", err)
	}
	if updated.EvalCases[0].SessionInput.AppName != "new" {
		t.Error("expected updated app name")
	}
}

func TestUpdateEvalCaseInEvalSet_NotFound(t *testing.T) {
	evalSet := NewEvalSet("test_set")
	_, err := UpdateEvalCaseInEvalSet(evalSet, EvalCase{EvalID: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent case")
	}
}

func TestDeleteEvalCaseFromEvalSet_Success(t *testing.T) {
	evalSet := &EvalSet{
		EvalSetID: "test_set",
		EvalCases: []EvalCase{{EvalID: "case1"}, {EvalID: "case2"}},
	}
	updated, err := DeleteEvalCaseFromEvalSet(evalSet, "case1")
	if err != nil {
		t.Fatalf("DeleteEvalCaseFromEvalSet failed: %v", err)
	}
	if len(updated.EvalCases) != 1 {
		t.Errorf("expected 1 case after delete, got %d", len(updated.EvalCases))
	}
	if updated.EvalCases[0].EvalID != "case2" {
		t.Errorf("expected case2 to remain, got %s", updated.EvalCases[0].EvalID)
	}
}

func TestDeleteEvalCaseFromEvalSet_NotFound(t *testing.T) {
	evalSet := NewEvalSet("test_set")
	_, err := DeleteEvalCaseFromEvalSet(evalSet, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent case")
	}
}
