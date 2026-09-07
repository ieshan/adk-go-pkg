package eval_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func TestConvertEventsToInvocation_Basic(t *testing.T) {
	userContent := &genai.Content{
		Parts: []*genai.Part{{Text: "hello"}},
		Role:  "user",
	}
	agentResponse := &genai.Content{
		Parts: []*genai.Part{{Text: "hi there"}},
		Role:  "model",
	}

	events := []*session.Event{
		{
			Author: "user",
			LLMResponse: model.LLMResponse{
				Content:      userContent,
				TurnComplete: true,
			},
		},
		{
			Author: "agent",
			LLMResponse: model.LLMResponse{
				Content:      agentResponse,
				TurnComplete: true,
			},
		},
	}

	invocation := eval.ConvertEventsToInvocation(events, userContent, nil)
	if invocation.UserContent == nil {
		t.Errorf("got nil, want non-nil UserContent")
	}
	if invocation.FinalResponse == nil {
		t.Errorf("got nil, want non-nil FinalResponse")
	}
	if invocation.IntermediateData == nil {
		t.Errorf("got nil, want non-nil IntermediateData")
	}
	events2 := invocation.IntermediateData.GetInvocationEvents()
	if len(events2) != 2 {
		t.Errorf("got %d invocation events, want 2", len(events2))
	}
}

func TestConvertEventsToInvocation_ExtractsUserContentFromEvents(t *testing.T) {
	userContent := &genai.Content{
		Parts: []*genai.Part{{Text: "hello"}},
		Role:  "user",
	}
	agentResponse := &genai.Content{
		Parts: []*genai.Part{{Text: "hi"}},
		Role:  "model",
	}

	events := []*session.Event{
		{
			Author: "user",
			LLMResponse: model.LLMResponse{
				Content:      userContent,
				TurnComplete: true,
			},
		},
		{
			Author: "agent",
			LLMResponse: model.LLMResponse{
				Content:      agentResponse,
				TurnComplete: true,
			},
		},
	}

	invocation := eval.ConvertEventsToInvocation(events, nil, nil)
	if invocation.UserContent == nil {
		t.Errorf("got nil, want non-nil UserContent extracted from events")
	}
}

func TestConvertEventsToInvocation_EmptyEvents(t *testing.T) {
	invocation := eval.ConvertEventsToInvocation(nil, nil, nil)
	if invocation.UserContent != nil {
		t.Errorf("got non-nil, want nil UserContent for empty events")
	}
	if invocation.FinalResponse != nil {
		t.Errorf("got non-nil, want nil FinalResponse for empty events")
	}
	if invocation.IntermediateData != nil {
		t.Errorf("got non-nil, want nil IntermediateData for empty events")
	}
}

func TestConvertEventsToEvalInvocations_GroupsByInvocationID(t *testing.T) {
	events := []*session.Event{
		{
			InvocationID: "inv1",
			Author:       "user",
			LLMResponse: model.LLMResponse{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "hello"}}, Role: "user"},
				TurnComplete: true,
			},
		},
		{
			InvocationID: "inv1",
			Author:       "agent",
			LLMResponse: model.LLMResponse{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "hi"}}, Role: "model"},
				TurnComplete: true,
			},
		},
		{
			InvocationID: "inv2",
			Author:       "user",
			LLMResponse: model.LLMResponse{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "bye"}}, Role: "user"},
				TurnComplete: true,
			},
		},
		{
			InvocationID: "inv2",
			Author:       "agent",
			LLMResponse: model.LLMResponse{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "goodbye"}}, Role: "model"},
				TurnComplete: true,
			},
		},
	}

	invocations := eval.ConvertEventsToEvalInvocations(events, nil)
	if len(invocations) != 2 {
		t.Fatalf("got %d invocations, want 2", len(invocations))
	}
	if invocations[0].InvocationID != "inv1" {
		t.Errorf("got %s, want inv1", invocations[0].InvocationID)
	}
	if invocations[1].InvocationID != "inv2" {
		t.Errorf("got %s, want inv2", invocations[1].InvocationID)
	}
}
