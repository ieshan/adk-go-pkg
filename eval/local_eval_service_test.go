package eval_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func newTestLocalEvalService(t *testing.T) (*eval.LocalEvalSetsManager, *eval.LocalEvalSetResultsManager) {
	t.Helper()
	dir := t.TempDir()
	setsMgr, err := eval.NewLocalEvalSetsManager(dir)
	if err != nil {
		t.Fatalf("NewLocalEvalSetsManager: %v", err)
	}
	t.Cleanup(func() { _ = setsMgr.Close() })
	resultsMgr, err := eval.NewLocalEvalSetResultsManager(dir)
	if err != nil {
		t.Fatalf("NewLocalEvalSetResultsManager: %v", err)
	}
	t.Cleanup(func() { _ = resultsMgr.Close() })
	return setsMgr, resultsMgr
}

func TestLocalEvalService_PerformInference(t *testing.T) {
	ctx := context.Background()
	setsMgr, resultsMgr := newTestLocalEvalService(t)

	_, err := setsMgr.CreateEvalSet(ctx, "app", "test-set")
	if err != nil {
		t.Fatalf("CreateEvalSet failed: %v", err)
	}

	err = setsMgr.AddEvalCase(ctx, "app", "test-set", eval.EvalCase{
		EvalID: "case-1",
		Conversation: []eval.Invocation{{
			UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		}},
	})
	if err != nil {
		t.Fatalf("AddEvalCase failed: %v", err)
	}

	runner := &fakeAgentRunner{
		events: []*session.Event{
			{
				Author: "user",
				LLMResponse: model.LLMResponse{
					Content:      &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
					TurnComplete: true,
				},
			},
			{
				Author: "agent",
				LLMResponse: model.LLMResponse{
					Content:      &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "hello"}}},
					TurnComplete: true,
				},
			},
		},
	}

	svc := eval.NewLocalEvalService(setsMgr, resultsMgr, eval.DefaultMetricEvaluatorRegistry(), runner, nil)

	results := []*eval.InferenceResult{}
	for result, err := range svc.PerformInference(ctx, &eval.InferenceRequest{
		AppName:         "app",
		EvalSetID:       "test-set",
		InferenceConfig: eval.InferenceConfig{Parallelism: 1},
	}) {
		if err != nil {
			t.Fatalf("PerformInference error: %v", err)
		}
		results = append(results, result)
	}

	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
	if results[0].Status != eval.InferenceStatusSuccess {
		t.Errorf("Status = %v, want %v", results[0].Status, eval.InferenceStatusSuccess)
	}
	if len(results[0].Inferences) != 1 {
		t.Errorf("len(Inferences) = %d, want 1", len(results[0].Inferences))
	}
}

func TestLocalEvalService_PerformInference_EvalSetNotFound(t *testing.T) {
	ctx := context.Background()
	setsMgr, resultsMgr := newTestLocalEvalService(t)
	runner := &fakeAgentRunner{}

	svc := eval.NewLocalEvalService(setsMgr, resultsMgr, nil, runner, nil)

	for _, err := range svc.PerformInference(ctx, &eval.InferenceRequest{
		AppName:   "app",
		EvalSetID: "nonexistent",
	}) {
		if err == nil {
			t.Errorf("got nil error, want non-nil error for non-existent eval set")
		}
		return
	}
}

func TestLocalEvalService_PerformInference_EmptyEvalSet(t *testing.T) {
	ctx := context.Background()
	setsMgr, resultsMgr := newTestLocalEvalService(t)

	_, err := setsMgr.CreateEvalSet(ctx, "app", "empty-set")
	if err != nil {
		t.Fatalf("CreateEvalSet failed: %v", err)
	}

	runner := &fakeAgentRunner{}
	svc := eval.NewLocalEvalService(setsMgr, resultsMgr, nil, runner, nil)

	results := []*eval.InferenceResult{}
	for result, err := range svc.PerformInference(ctx, &eval.InferenceRequest{
		AppName:         "app",
		EvalSetID:       "empty-set",
		InferenceConfig: eval.InferenceConfig{Parallelism: 1},
	}) {
		if err != nil {
			t.Fatalf("PerformInference error: %v", err)
		}
		results = append(results, result)
	}

	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0 for empty eval set", len(results))
	}
}

func TestLocalEvalService_Evaluate(t *testing.T) {
	ctx := context.Background()
	setsMgr, resultsMgr := newTestLocalEvalService(t)

	_, err := setsMgr.CreateEvalSet(ctx, "app", "test-set")
	if err != nil {
		t.Fatalf("CreateEvalSet failed: %v", err)
	}

	err = setsMgr.AddEvalCase(ctx, "app", "test-set", eval.EvalCase{
		EvalID: "case-1",
		Conversation: []eval.Invocation{{
			UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		}},
	})
	if err != nil {
		t.Fatalf("AddEvalCase failed: %v", err)
	}

	threshold := 1.0
	metrics := []eval.EvalMetric{{
		MetricName: string(eval.ToolTrajectoryAvgScore),
		Threshold:  &threshold,
	}}

	svc := eval.NewLocalEvalService(setsMgr, resultsMgr, eval.DefaultMetricEvaluatorRegistry(), &fakeAgentRunner{}, nil)

	inferenceResults := []eval.InferenceResult{{
		AppName:    "app",
		EvalSetID:  "test-set",
		EvalCaseID: "case-1",
		Status:     eval.InferenceStatusSuccess,
		Inferences: []eval.Invocation{{
			FinalResponse: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "hello"}}},
		}},
	}}

	var caseResults []*eval.EvalCaseResult
	for result, err := range svc.Evaluate(ctx, &eval.EvaluateRequest{
		InferenceResults: inferenceResults,
		EvaluateConfig: eval.EvaluateConfig{
			EvalMetrics: metrics,
			Parallelism: 1,
		},
	}) {
		if err != nil {
			t.Fatalf("Evaluate error: %v", err)
		}
		caseResults = append(caseResults, result)
	}

	if len(caseResults) != 1 {
		t.Fatalf("len(caseResults) = %d, want 1", len(caseResults))
	}
	if caseResults[0].EvalID != "case-1" {
		t.Errorf("EvalID = %q, want %q", caseResults[0].EvalID, "case-1")
	}
}
