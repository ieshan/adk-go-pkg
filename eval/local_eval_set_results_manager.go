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

// LocalEvalSetResultsManager stores eval set results as .evalset_result.json
// files on disk. All filesystem access is scoped beneath an [os.Root] opened
// from agentsDir.
type LocalEvalSetResultsManager struct {
	root *os.Root
}

// NewLocalEvalSetResultsManager creates a new LocalEvalSetResultsManager backed
// by agentsDir. The directory is created if it does not exist. Callers must
// call Close to release the underlying file descriptor.
func NewLocalEvalSetResultsManager(agentsDir string) (*LocalEvalSetResultsManager, error) {
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		return nil, fmt.Errorf("create eval results dir: %w", err)
	}
	root, err := os.OpenRoot(agentsDir)
	if err != nil {
		return nil, fmt.Errorf("open eval results root: %w", err)
	}
	return &LocalEvalSetResultsManager{root: root}, nil
}

// Close releases the underlying [os.Root] file descriptor.
// It is safe to call multiple times.
func (m *LocalEvalSetResultsManager) Close() error {
	return m.root.Close()
}

func (m *LocalEvalSetResultsManager) resultsDir(appName string) string {
	return filepath.Join(appName, ".adk", "eval_history")
}

func (m *LocalEvalSetResultsManager) resultPath(appName, resultID string) string {
	return filepath.Join(m.resultsDir(appName), resultID+".evalset_result.json")
}

// SaveEvalSetResult writes eval set results to a .evalset_result.json file on disk.
func (m *LocalEvalSetResultsManager) SaveEvalSetResult(ctx context.Context, appName, evalSetID string, results []EvalCaseResult) error {
	if err := ValidatePathSegment(appName, "appName"); err != nil {
		return err
	}
	result := CreateEvalSetResult(appName, evalSetID, results)
	path := m.resultPath(appName, SanitizeEvalSetResultName(result.EvalSetResultID))
	dir := filepath.Dir(path)
	if err := m.root.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create results directory: %w", err)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal eval set result: %w", err)
	}
	if err := m.root.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write eval set result file: %w", err)
	}
	return nil
}

// GetEvalSetResult loads an eval set result from a .evalset_result.json file on disk.
func (m *LocalEvalSetResultsManager) GetEvalSetResult(ctx context.Context, appName, evalSetResultID string) (*EvalSetResult, error) {
	if err := ValidatePathSegment(appName, "appName"); err != nil {
		return nil, err
	}
	if err := ValidatePathSegment(evalSetResultID, "evalSetResultID"); err != nil {
		return nil, err
	}
	path := m.resultPath(appName, evalSetResultID)
	data, err := m.root.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, NewNotFoundError("eval set result", evalSetResultID)
		}
		return nil, fmt.Errorf("failed to read eval set result: %w", err)
	}
	return ParseEvalSetResultJSON(data)
}

// ListEvalSetResults returns the IDs of all .evalset_result.json files for the given app.
func (m *LocalEvalSetResultsManager) ListEvalSetResults(ctx context.Context, appName string) ([]string, error) {
	if err := ValidatePathSegment(appName, "appName"); err != nil {
		return nil, err
	}
	dir := m.resultsDir(appName)
	entries, err := fs.ReadDir(m.root.FS(), dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list eval set results: %w", err)
	}
	var result []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".evalset_result.json") {
			id := strings.TrimSuffix(name, ".evalset_result.json")
			result = append(result, id)
		}
	}
	return result, nil
}

// Compile-time interface check.
var _ EvalSetResultsManager = (*LocalEvalSetResultsManager)(nil)
