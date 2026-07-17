package eval

import (
	"context"
	"fmt"
)

// GetEvalSetFromAppAndID returns an EvalSet if found; otherwise, it returns
// a NotFoundError. This is a convenience wrapper around EvalSetsManager.
func GetEvalSetFromAppAndID(ctx context.Context, manager EvalSetsManager, appName, evalSetID string) (*EvalSet, error) {
	if manager == nil {
		return nil, fmt.Errorf("eval sets manager is nil")
	}
	evalSet, err := manager.GetEvalSet(ctx, appName, evalSetID)
	if err != nil {
		return nil, err
	}
	if evalSet == nil {
		return nil, NewNotFoundError("eval set", evalSetID)
	}
	return evalSet, nil
}

// GetEvalCaseFromEvalSet finds an eval case by ID within an eval set.
// Returns nil if not found.
func GetEvalCaseFromEvalSet(evalSet *EvalSet, evalCaseID string) *EvalCase {
	if evalSet == nil {
		return nil
	}
	for i, ec := range evalSet.EvalCases {
		if ec.EvalID == evalCaseID {
			return &evalSet.EvalCases[i]
		}
	}
	return nil
}

// AddEvalCaseToEvalSet adds an eval case to an eval set, rejecting duplicates.
// Returns a new copy of the eval set.
func AddEvalCaseToEvalSet(evalSet *EvalSet, evalCase EvalCase) (*EvalSet, error) {
	if evalSet == nil {
		return nil, fmt.Errorf("eval set is nil")
	}
	if GetEvalCaseFromEvalSet(evalSet, evalCase.EvalID) != nil {
		return nil, fmt.Errorf("%w: eval case %q already exists", ErrAlreadyExists, evalCase.EvalID)
	}
	updated := *evalSet
	updated.EvalCases = append([]EvalCase(evalSet.EvalCases), evalCase)
	return &updated, nil
}

// UpdateEvalCaseInEvalSet replaces an existing eval case by ID.
// Returns a new copy of the eval set.
func UpdateEvalCaseInEvalSet(evalSet *EvalSet, updatedCase EvalCase) (*EvalSet, error) {
	if evalSet == nil {
		return nil, fmt.Errorf("eval set is nil")
	}
	found := false
	cases := make([]EvalCase, len(evalSet.EvalCases))
	copy(cases, evalSet.EvalCases)
	for i, ec := range cases {
		if ec.EvalID == updatedCase.EvalID {
			cases[i] = updatedCase
			found = true
			break
		}
	}
	if !found {
		return nil, NewNotFoundError("eval case", updatedCase.EvalID)
	}
	updated := *evalSet
	updated.EvalCases = cases
	return &updated, nil
}

// DeleteEvalCaseFromEvalSet removes an eval case by ID.
// Returns a new copy of the eval set.
func DeleteEvalCaseFromEvalSet(evalSet *EvalSet, evalCaseID string) (*EvalSet, error) {
	if evalSet == nil {
		return nil, fmt.Errorf("eval set is nil")
	}
	found := false
	var cases []EvalCase
	for _, ec := range evalSet.EvalCases {
		if ec.EvalID == evalCaseID {
			found = true
			continue
		}
		cases = append(cases, ec)
	}
	if !found {
		return nil, NewNotFoundError("eval case", evalCaseID)
	}
	updated := *evalSet
	updated.EvalCases = cases
	return &updated, nil
}
