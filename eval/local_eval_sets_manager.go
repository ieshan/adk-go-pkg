package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LocalEvalSetsManager stores eval sets as .evalset.json files on disk.
type LocalEvalSetsManager struct {
	agentsDir string
}

// NewLocalEvalSetsManager creates a new LocalEvalSetsManager.
func NewLocalEvalSetsManager(agentsDir string) *LocalEvalSetsManager {
	return &LocalEvalSetsManager{agentsDir: agentsDir}
}

func (m *LocalEvalSetsManager) evalSetPath(appName, evalSetID string) string {
	return filepath.Join(m.agentsDir, appName, "eval", evalSetID+".evalset.json")
}

func (m *LocalEvalSetsManager) evalDir(appName string) string {
	return filepath.Join(m.agentsDir, appName, "eval")
}

func (m *LocalEvalSetsManager) validateAndPath(appName, evalSetID string) (string, error) {
	if err := ValidatePathSegment(appName, "appName"); err != nil {
		return "", err
	}
	if err := ValidatePathSegment(evalSetID, "evalSetID"); err != nil {
		return "", err
	}
	return m.evalSetPath(appName, evalSetID), nil
}

func (m *LocalEvalSetsManager) GetEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error) {
	path, err := m.validateAndPath(appName, evalSetID)
	if err != nil {
		return nil, err
	}
	return LoadEvalSetFromFile(path)
}

func (m *LocalEvalSetsManager) CreateEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error) {
	path, err := m.validateAndPath(appName, evalSetID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("%w: eval set %q already exists", ErrAlreadyExists, evalSetID)
	}
	evalSet := NewEvalSet(evalSetID)
	if err := m.saveEvalSet(path, evalSet); err != nil {
		return nil, err
	}
	return evalSet, nil
}

func (m *LocalEvalSetsManager) ListEvalSets(ctx context.Context, appName string) ([]string, error) {
	if err := ValidatePathSegment(appName, "appName"); err != nil {
		return nil, err
	}
	dir := m.evalDir(appName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list eval sets: %w", err)
	}
	var result []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".evalset.json") {
			id := strings.TrimSuffix(name, ".evalset.json")
			result = append(result, id)
		}
	}
	return result, nil
}

func (m *LocalEvalSetsManager) GetEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) (*EvalCase, error) {
	evalSet, err := m.GetEvalSet(ctx, appName, evalSetID)
	if err != nil {
		return nil, err
	}
	ec := GetEvalCaseFromEvalSet(evalSet, evalCaseID)
	if ec == nil {
		return nil, NewNotFoundError("eval case", evalCaseID)
	}
	return ec, nil
}

func (m *LocalEvalSetsManager) AddEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error {
	path, err := m.validateAndPath(appName, evalSetID)
	if err != nil {
		return err
	}
	evalSet, err := LoadEvalSetFromFile(path)
	if err != nil {
		return err
	}
	updated, err := AddEvalCaseToEvalSet(evalSet, evalCase)
	if err != nil {
		return err
	}
	return m.saveEvalSet(path, updated)
}

func (m *LocalEvalSetsManager) UpdateEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error {
	path, err := m.validateAndPath(appName, evalSetID)
	if err != nil {
		return err
	}
	evalSet, err := LoadEvalSetFromFile(path)
	if err != nil {
		return err
	}
	updated, err := UpdateEvalCaseInEvalSet(evalSet, evalCase)
	if err != nil {
		return err
	}
	return m.saveEvalSet(path, updated)
}

func (m *LocalEvalSetsManager) DeleteEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) error {
	path, err := m.validateAndPath(appName, evalSetID)
	if err != nil {
		return err
	}
	evalSet, err := LoadEvalSetFromFile(path)
	if err != nil {
		return err
	}
	updated, err := DeleteEvalCaseFromEvalSet(evalSet, evalCaseID)
	if err != nil {
		return err
	}
	return m.saveEvalSet(path, updated)
}

func (m *LocalEvalSetsManager) saveEvalSet(path string, evalSet *EvalSet) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create eval directory: %w", err)
	}
	data, err := json.MarshalIndent(evalSet, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal eval set: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write eval set file: %w", err)
	}
	return nil
}

// Compile-time interface check.
var _ EvalSetsManager = (*LocalEvalSetsManager)(nil)
