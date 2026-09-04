package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LocalEvalSetsManager stores eval sets as .evalset.json files on disk.
// All filesystem access is scoped beneath an [os.Root] opened from agentsDir.
type LocalEvalSetsManager struct {
	root *os.Root
}

// NewLocalEvalSetsManager creates a new LocalEvalSetsManager backed by agentsDir.
// The directory is created if it does not exist. Callers must call Close to
// release the underlying file descriptor.
func NewLocalEvalSetsManager(agentsDir string) (*LocalEvalSetsManager, error) {
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		return nil, fmt.Errorf("create eval sets dir: %w", err)
	}
	root, err := os.OpenRoot(agentsDir)
	if err != nil {
		return nil, fmt.Errorf("open eval sets root: %w", err)
	}
	return &LocalEvalSetsManager{root: root}, nil
}

// Close releases the underlying [os.Root] file descriptor.
// It is safe to call multiple times.
func (m *LocalEvalSetsManager) Close() error {
	return m.root.Close()
}

func (m *LocalEvalSetsManager) evalSetPath(appName, evalSetID string) string {
	return filepath.Join(appName, "eval", evalSetID+".evalset.json")
}

func (m *LocalEvalSetsManager) evalDir(appName string) string {
	return filepath.Join(appName, "eval")
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

// GetEvalSet loads the eval set from a .evalset.json file on disk.
func (m *LocalEvalSetsManager) GetEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error) {
	path, err := m.validateAndPath(appName, evalSetID)
	if err != nil {
		return nil, err
	}
	return LoadEvalSetFromFile(m.root, path)
}

// CreateEvalSet creates a new empty eval set file on disk.
func (m *LocalEvalSetsManager) CreateEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error) {
	path, err := m.validateAndPath(appName, evalSetID)
	if err != nil {
		return nil, err
	}
	if _, err := m.root.Stat(path); err == nil {
		return nil, fmt.Errorf("%w: eval set %q already exists", ErrAlreadyExists, evalSetID)
	}
	evalSet := NewEvalSet(evalSetID)
	if err := m.saveEvalSet(path, evalSet); err != nil {
		return nil, err
	}
	return evalSet, nil
}

// ListEvalSets returns the IDs of all .evalset.json files in the app's eval directory.
func (m *LocalEvalSetsManager) ListEvalSets(ctx context.Context, appName string) ([]string, error) {
	if err := ValidatePathSegment(appName, "appName"); err != nil {
		return nil, err
	}
	dir := m.evalDir(appName)
	entries, err := fs.ReadDir(m.root.FS(), dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
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

// GetEvalCase returns a specific eval case from an eval set on disk.
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

// AddEvalCase adds a new eval case to an eval set file on disk.
func (m *LocalEvalSetsManager) AddEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error {
	path, err := m.validateAndPath(appName, evalSetID)
	if err != nil {
		return err
	}
	evalSet, err := LoadEvalSetFromFile(m.root, path)
	if err != nil {
		return err
	}
	updated, err := AddEvalCaseToEvalSet(evalSet, evalCase)
	if err != nil {
		return err
	}
	return m.saveEvalSet(path, updated)
}

// UpdateEvalCase updates an existing eval case in an eval set file on disk.
func (m *LocalEvalSetsManager) UpdateEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error {
	path, err := m.validateAndPath(appName, evalSetID)
	if err != nil {
		return err
	}
	evalSet, err := LoadEvalSetFromFile(m.root, path)
	if err != nil {
		return err
	}
	updated, err := UpdateEvalCaseInEvalSet(evalSet, evalCase)
	if err != nil {
		return err
	}
	return m.saveEvalSet(path, updated)
}

// DeleteEvalCase removes an eval case from an eval set file on disk.
func (m *LocalEvalSetsManager) DeleteEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) error {
	path, err := m.validateAndPath(appName, evalSetID)
	if err != nil {
		return err
	}
	evalSet, err := LoadEvalSetFromFile(m.root, path)
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
	if err := m.root.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create eval directory: %w", err)
	}
	data, err := json.MarshalIndent(evalSet, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal eval set: %w", err)
	}
	if err := m.root.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write eval set file: %w", err)
	}
	return nil
}

// Compile-time interface check.
var _ EvalSetsManager = (*LocalEvalSetsManager)(nil)
