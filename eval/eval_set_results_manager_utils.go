package eval

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CreateEvalSetResult creates an EvalSetResult from eval case results.
func CreateEvalSetResult(appName, evalSetID string, results []EvalCaseResult) *EvalSetResult {
	timestamp := float64(time.Now().Unix())
	resultID := fmt.Sprintf("%s_%s_%d", appName, evalSetID, int64(timestamp))
	resultName := fmt.Sprintf("%s_%s", appName, evalSetID)

	return &EvalSetResult{
		EvalSetResultID:   resultID,
		EvalSetResultName: resultName,
		EvalSetID:         evalSetID,
		EvalCaseResults:   results,
		CreationTimestamp: timestamp,
	}
}

// SanitizeEvalSetResultName replaces path separators with underscores.
func SanitizeEvalSetResultName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	return strings.ReplaceAll(name, "\\", "_")
}

// ParseEvalSetResultJSON parses eval set result JSON, handling
// double-encoded JSON for backward compatibility.
func ParseEvalSetResultJSON(data []byte) (*EvalSetResult, error) {
	var result EvalSetResult
	if err := json.Unmarshal(data, &result); err != nil {
		// Try double-encoded JSON (string within JSON).
		var doubleEncoded string
		if err2 := json.Unmarshal(data, &doubleEncoded); err2 == nil {
			if err3 := json.Unmarshal([]byte(doubleEncoded), &result); err3 == nil {
				return &result, nil
			}
		}
		return nil, fmt.Errorf("failed to parse eval set result JSON: %w", err)
	}
	return &result, nil
}
