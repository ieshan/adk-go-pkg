package eval

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// fakeAgentRunner implements AgentRunner for testing.
type fakeAgentRunner struct {
	events []*session.Event
	err    error
}

func (f *fakeAgentRunner) RunForSession(ctx context.Context, sessionID, userID, appName string, userContent *genai.Content) ([]*session.Event, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events, nil
}

func TestAgentEvaluator_Evaluate(t *testing.T) {
	ctx := context.Background()
	setsMgr := NewInMemoryEvalSetsManager()

	_, err := setsMgr.CreateEvalSet(ctx, "app", "test-set")
	if err != nil {
		t.Fatalf("CreateEvalSet failed: %v", err)
	}

	err = setsMgr.AddEvalCase(ctx, "app", "test-set", EvalCase{
		EvalID: "case-1",
		Conversation: []Invocation{{
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

	agentEval := NewAgentEvaluator(
		runner,
		setsMgr,
		nil,
		DefaultMetricEvaluatorRegistry(),
		nil,
	)

	config := EvalConfig{}
	config.Criteria = map[string]json.RawMessage{}
	config.Criteria["tool_trajectory_avg_score"] = json.RawMessage("1.0")

	result, err := agentEval.Evaluate(ctx, "app", "test-set", config)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.EvalCaseResults) != 1 {
		t.Errorf("len(EvalCaseResults) = %d, want 1", len(result.EvalCaseResults))
	}
}

func TestAgentEvaluator_EvaluateEmptyEvalSet(t *testing.T) {
	ctx := context.Background()
	setsMgr := NewInMemoryEvalSetsManager()

	_, err := setsMgr.CreateEvalSet(ctx, "app", "empty-set")
	if err != nil {
		t.Fatalf("CreateEvalSet failed: %v", err)
	}

	runner := &fakeAgentRunner{}
	agentEval := NewAgentEvaluator(runner, setsMgr, nil, nil, nil)

	config := EvalConfig{}
	config.Criteria = map[string]json.RawMessage{}
	config.Criteria["tool_trajectory_avg_score"] = json.RawMessage("1.0")

	result, err := agentEval.Evaluate(ctx, "app", "empty-set", config)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if len(result.EvalCaseResults) != 0 {
		t.Errorf("len(EvalCaseResults) = %d, want 0", len(result.EvalCaseResults))
	}
}

func TestAgentEvaluator_EvalSetNotFound(t *testing.T) {
	ctx := context.Background()
	setsMgr := NewInMemoryEvalSetsManager()
	runner := &fakeAgentRunner{}
	agentEval := NewAgentEvaluator(runner, setsMgr, nil, nil, nil)

	_, err := agentEval.Evaluate(ctx, "app", "nonexistent", EvalConfig{})
	if err == nil {
		t.Error("expected error for non-existent eval set")
	}
}

func TestAgentEvaluator_GenerateInvocations_DynamicNotImplemented(t *testing.T) {
	ctx := context.Background()
	runner := &fakeAgentRunner{}
	agentEval := NewAgentEvaluator(runner, nil, nil, nil, nil)

	_, err := agentEval.generateInvocations(ctx, "app", EvalCase{})
	if err == nil {
		t.Error("expected error for dynamic conversation (not implemented)")
	}
}
