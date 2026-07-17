package eval

import (
	"testing"

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

	invocation := ConvertEventsToInvocation(events, userContent, nil)
	if invocation.UserContent == nil {
		t.Error("expected non-nil UserContent")
	}
	if invocation.FinalResponse == nil {
		t.Error("expected non-nil FinalResponse")
	}
	if invocation.IntermediateData == nil {
		t.Error("expected non-nil IntermediateData")
	}
	events2 := invocation.IntermediateData.GetInvocationEvents()
	if len(events2) != 2 {
		t.Errorf("expected 2 invocation events, got %d", len(events2))
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

	invocation := ConvertEventsToInvocation(events, nil, nil)
	if invocation.UserContent == nil {
		t.Error("expected to extract UserContent from events")
	}
}

func TestConvertEventsToInvocation_EmptyEvents(t *testing.T) {
	invocation := ConvertEventsToInvocation(nil, nil, nil)
	if invocation.UserContent != nil {
		t.Error("expected nil UserContent for empty events")
	}
	if invocation.FinalResponse != nil {
		t.Error("expected nil FinalResponse for empty events")
	}
	if invocation.IntermediateData != nil {
		t.Error("expected nil IntermediateData for empty events")
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

	invocations := ConvertEventsToEvalInvocations(events, nil)
	if len(invocations) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(invocations))
	}
	if invocations[0].InvocationID != "inv1" {
		t.Errorf("expected inv1, got %s", invocations[0].InvocationID)
	}
	if invocations[1].InvocationID != "inv2" {
		t.Errorf("expected inv2, got %s", invocations[1].InvocationID)
	}
}
