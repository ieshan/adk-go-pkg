package eval

import "context"

// EvalSetResultsManager is the interface for managing eval set results.
type EvalSetResultsManager interface {
	// SaveEvalSetResult saves eval set results.
	SaveEvalSetResult(ctx context.Context, appName, evalSetID string, results []EvalCaseResult) error

	// GetEvalSetResult returns the eval set result with the given ID.
	GetEvalSetResult(ctx context.Context, appName, evalSetResultID string) (*EvalSetResult, error)

	// ListEvalSetResults returns the IDs of all eval set results for the given app.
	ListEvalSetResults(ctx context.Context, appName string) ([]string, error)
}
