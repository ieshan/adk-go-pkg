package eval

// EvalMetricResultDetails contains additional details about a metric result.
type EvalMetricResultDetails struct {
	// RubricScores are the scores for individual rubrics.
	RubricScores []RubricScore `json:"rubricScores,omitempty"`
}

// EvalMetricResult represents the result of evaluating a single metric.
type EvalMetricResult struct {
	EvalMetric
	// Score is the score obtained for this metric.
	Score *float64 `json:"score,omitempty"`

	// EvalStatus is the evaluation status (PASSED, FAILED, NOT_EVALUATED).
	EvalStatus EvalStatus `json:"evalStatus,omitempty"`

	// Details contains additional details about the metric result.
	Details EvalMetricResultDetails `json:"details,omitempty"`
}

// EvalMetricResultPerInvocation represents metric results for a single
// invocation within an eval case.
type EvalMetricResultPerInvocation struct {
	// ActualInvocation is the actual invocation that was evaluated.
	ActualInvocation Invocation `json:"actualInvocation,omitempty"`

	// ExpectedInvocation is the expected invocation (if any).
	ExpectedInvocation *Invocation `json:"expectedInvocation,omitempty"`

	// EvalMetricResults is the list of metric results for this invocation.
	EvalMetricResults []EvalMetricResult `json:"evalMetricResults,omitempty"`
}

// EvalCaseResult represents the result of evaluating a single eval case.
type EvalCaseResult struct {
	// EvalSetID is the ID of the eval set this case belongs to.
	EvalSetID string `json:"evalSetId,omitempty"`

	// EvalID is the ID of the eval case.
	EvalID string `json:"evalId"`

	// FinalEvalStatus is the overall status (PASSED if all metrics pass,
	// FAILED if any fail).
	FinalEvalStatus EvalStatus `json:"finalEvalStatus,omitempty"`

	// OverallEvalMetricResults contains the results for each metric,
	// aggregated across invocations.
	OverallEvalMetricResults []EvalMetricResult `json:"overallEvalMetricResults,omitempty"`

	// EvalMetricResultPerInvocation contains per-invocation metric results.
	EvalMetricResultPerInvocation []EvalMetricResultPerInvocation `json:"evalMetricResultPerInvocation,omitempty"`

	// SessionID is the session ID used for inference.
	SessionID string `json:"sessionId,omitempty"`

	// UserID is the user ID used for inference.
	UserID string `json:"userId,omitempty"`
}

// EvalSetResult represents the results of evaluating an entire eval set.
type EvalSetResult struct {
	// EvalSetResultID is the unique identifier for this result.
	EvalSetResultID string `json:"evalSetResultId"`

	// EvalSetResultName is a human-readable name.
	EvalSetResultName string `json:"evalSetResultName,omitempty"`

	// EvalSetID is the ID of the eval set that was evaluated.
	EvalSetID string `json:"evalSetId,omitempty"`

	// EvalCaseResults contains the results for each eval case.
	EvalCaseResults []EvalCaseResult `json:"evalCaseResults,omitempty"`

	// CreationTimestamp is the Unix timestamp of the result.
	CreationTimestamp float64 `json:"creationTimestamp,omitempty"`
}
