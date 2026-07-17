package eval

import "context"

// EvalSetsManager is the interface for managing eval sets and eval cases.
type EvalSetsManager interface {
	// GetEvalSet returns the eval set for the given app and eval set ID.
	GetEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error)

	// CreateEvalSet creates a new empty eval set.
	CreateEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error)

	// ListEvalSets returns the names of all eval sets for the given app.
	ListEvalSets(ctx context.Context, appName string) ([]string, error)

	// GetEvalCase returns a specific eval case from an eval set.
	GetEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) (*EvalCase, error)

	// AddEvalCase adds a new eval case to an eval set.
	AddEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error

	// UpdateEvalCase updates an existing eval case in an eval set.
	UpdateEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error

	// DeleteEvalCase removes an eval case from an eval set.
	DeleteEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) error
}
