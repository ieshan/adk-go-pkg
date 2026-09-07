package eval

import (
	"context"
	"fmt"
	"sync"
)

// InMemoryEvalSetsManager stores eval sets in memory. Useful for testing.
type InMemoryEvalSetsManager struct {
	mu       sync.RWMutex
	evalSets map[string]map[string]*EvalSet // appName → evalSetID → EvalSet
}

// NewInMemoryEvalSetsManager creates a new InMemoryEvalSetsManager.
func NewInMemoryEvalSetsManager() *InMemoryEvalSetsManager {
	return &InMemoryEvalSetsManager{
		evalSets: make(map[string]map[string]*EvalSet),
	}
}

func (m *InMemoryEvalSetsManager) getOrCreateAppMap(appName string) map[string]*EvalSet {
	appMap, ok := m.evalSets[appName]
	if !ok {
		appMap = make(map[string]*EvalSet)
		m.evalSets[appName] = appMap
	}
	return appMap
}

// GetEvalSet returns the eval set for the given app and eval set ID.
func (m *InMemoryEvalSetsManager) GetEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	appMap, ok := m.evalSets[appName]
	if !ok {
		return nil, NewNotFoundError("eval set", evalSetID)
	}
	evalSet, ok := appMap[evalSetID]
	if !ok {
		return nil, NewNotFoundError("eval set", evalSetID)
	}
	return evalSet, nil
}

// CreateEvalSet creates a new empty eval set in memory.
func (m *InMemoryEvalSetsManager) CreateEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	appMap := m.getOrCreateAppMap(appName)
	if _, exists := appMap[evalSetID]; exists {
		return nil, fmt.Errorf("%w: eval set %q already exists", ErrAlreadyExists, evalSetID)
	}
	evalSet := NewEvalSet(evalSetID)
	appMap[evalSetID] = evalSet
	return evalSet, nil
}

// ListEvalSets returns the IDs of all eval sets for the given app.
func (m *InMemoryEvalSetsManager) ListEvalSets(ctx context.Context, appName string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	appMap, ok := m.evalSets[appName]
	if !ok {
		return []string{}, nil
	}
	result := make([]string, 0, len(appMap))
	for id := range appMap {
		result = append(result, id)
	}
	return result, nil
}

// GetEvalCase returns a specific eval case from an eval set.
func (m *InMemoryEvalSetsManager) GetEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) (*EvalCase, error) {
	evalSet, err := m.GetEvalSet(ctx, appName, evalSetID)
	if err != nil {
		return nil, err
	}
	evalCase := GetEvalCaseFromEvalSet(evalSet, evalCaseID)
	if evalCase == nil {
		return nil, NewNotFoundError("eval case", evalCaseID)
	}
	return evalCase, nil
}

// AddEvalCase adds a new eval case to an eval set.
func (m *InMemoryEvalSetsManager) AddEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	appMap, ok := m.evalSets[appName]
	if !ok {
		return NewNotFoundError("eval set", evalSetID)
	}
	evalSet, ok := appMap[evalSetID]
	if !ok {
		return NewNotFoundError("eval set", evalSetID)
	}
	updated, err := AddEvalCaseToEvalSet(evalSet, evalCase)
	if err != nil {
		return err
	}
	appMap[evalSetID] = updated
	return nil
}

// UpdateEvalCase updates an existing eval case in an eval set.
func (m *InMemoryEvalSetsManager) UpdateEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	appMap, ok := m.evalSets[appName]
	if !ok {
		return NewNotFoundError("eval set", evalSetID)
	}
	evalSet, ok := appMap[evalSetID]
	if !ok {
		return NewNotFoundError("eval set", evalSetID)
	}
	updated, err := UpdateEvalCaseInEvalSet(evalSet, evalCase)
	if err != nil {
		return err
	}
	appMap[evalSetID] = updated
	return nil
}

// DeleteEvalCase removes an eval case from an eval set.
func (m *InMemoryEvalSetsManager) DeleteEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	appMap, ok := m.evalSets[appName]
	if !ok {
		return NewNotFoundError("eval set", evalSetID)
	}
	evalSet, ok := appMap[evalSetID]
	if !ok {
		return NewNotFoundError("eval set", evalSetID)
	}
	updated, err := DeleteEvalCaseFromEvalSet(evalSet, evalCaseID)
	if err != nil {
		return err
	}
	appMap[evalSetID] = updated
	return nil
}

// Compile-time interface check.
var _ EvalSetsManager = (*InMemoryEvalSetsManager)(nil)
