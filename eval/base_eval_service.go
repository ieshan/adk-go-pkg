package eval

import (
	"context"
	"iter"
)

// InferenceConfig controls how inference is run for evaluation.
type InferenceConfig struct {
	// Labels are optional labels to attach to inference results.
	Labels map[string]string `json:"labels,omitempty"`

	// Parallelism controls the number of concurrent inference runs.
	// Default is 4.
	Parallelism int `json:"parallelism,omitempty"`

	// UseLive enables live mode inference. Not yet implemented.
	UseLive bool `json:"useLive,omitempty"`

	// LiveTimeoutSeconds is the timeout for live mode sessions.
	// Default is DefaultLiveTimeoutSeconds (300).
	LiveTimeoutSeconds int `json:"liveTimeoutSeconds,omitempty"`
}

// EvaluateConfig controls how evaluation is performed.
type EvaluateConfig struct {
	// EvalMetrics is the list of metrics to evaluate.
	EvalMetrics []EvalMetric `json:"evalMetrics,omitempty"`

	// Parallelism controls the number of concurrent evaluations.
	// Default is 4.
	Parallelism int `json:"parallelism,omitempty"`
}

// InferenceRequest specifies which eval cases to run inference on.
type InferenceRequest struct {
	AppName         string          `json:"appName"`
	EvalSetID       string          `json:"evalSetId"`
	EvalCaseIDs     []string        `json:"evalCaseIds,omitempty"`
	InferenceConfig InferenceConfig `json:"inferenceConfig"`
}

// InferenceStatus represents the status of an inference run.
type InferenceStatus string

const (
	InferenceStatusUnknown InferenceStatus = "unknown"
	InferenceStatusSuccess InferenceStatus = "success"
	InferenceStatusFailure InferenceStatus = "failure"
)

// InferenceResult holds the result of running inference on a single eval case.
type InferenceResult struct {
	AppName      string          `json:"appName"`
	EvalSetID    string          `json:"evalSetId"`
	EvalCaseID   string          `json:"evalCaseId"`
	Inferences   []Invocation    `json:"inferences,omitempty"`
	SessionID    string          `json:"sessionId,omitempty"`
	Status       InferenceStatus `json:"status"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
}

// EvaluateRequest specifies the inference results to evaluate.
type EvaluateRequest struct {
	InferenceResults []InferenceResult `json:"inferenceResults"`
	EvaluateConfig   EvaluateConfig    `json:"evaluateConfig"`
}

// BaseEvalService is the interface for running inference and evaluation.
type BaseEvalService interface {
	// PerformInference runs the agent on eval cases and yields results.
	PerformInference(ctx context.Context, req *InferenceRequest) iter.Seq2[*InferenceResult, error]

	// Evaluate evaluates inference results against expected results.
	Evaluate(ctx context.Context, req *EvaluateRequest) iter.Seq2[*EvalCaseResult, error]
}
