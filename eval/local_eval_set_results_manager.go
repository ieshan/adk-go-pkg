package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LocalEvalSetResultsManager stores eval set results as .evalset_result.json
// files on disk.
type LocalEvalSetResultsManager struct {
	agentsDir string
}

// NewLocalEvalSetResultsManager creates a new LocalEvalSetResultsManager.
func NewLocalEvalSetResultsManager(agentsDir string) *LocalEvalSetResultsManager {
	return &LocalEvalSetResultsManager{agentsDir: agentsDir}
}

func (m *LocalEvalSetResultsManager) resultsDir(appName string) string {
	return filepath.Join(m.agentsDir, appName, ".adk", "eval_history")
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create results directory: %w", err)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal eval set result: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
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
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
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
