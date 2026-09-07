package eval_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"google.golang.org/genai"
)

func TestGetAllToolCalls_LegacyFormat(t *testing.T) {
	inv := eval.Invocation{
		IntermediateData: &eval.LegacyIntermediateData{
			ToolUses: []genai.FunctionCall{
				{Name: "get_weather", Args: map[string]any{"city": "SF"}},
				{Name: "get_time"},
			},
		},
	}

	calls := eval.GetAllToolCalls(inv)
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2", len(calls))
	}
	if calls[0].Name != "get_weather" {
		t.Errorf("calls[0].Name = %q, want %q", calls[0].Name, "get_weather")
	}
}

func TestGetAllToolCalls_NilIntermediateData(t *testing.T) {
	inv := eval.Invocation{}
	calls := eval.GetAllToolCalls(inv)
	if calls != nil {
		t.Errorf("got %v, want nil", calls)
	}
}

func TestGetAllToolResponses_LegacyFormat(t *testing.T) {
	inv := eval.Invocation{
		IntermediateData: &eval.LegacyIntermediateData{
			ToolResponses: []genai.FunctionResponse{
				{Name: "get_weather", Response: map[string]any{"temp": 72}},
			},
		},
	}

	responses := eval.GetAllToolResponses(inv)
	if len(responses) != 1 {
		t.Fatalf("len(responses) = %d, want 1", len(responses))
	}
	if responses[0].Name != "get_weather" {
		t.Errorf("responses[0].Name = %q, want %q", responses[0].Name, "get_weather")
	}
}

func TestGetAllToolResponses_NilIntermediateData(t *testing.T) {
	inv := eval.Invocation{}
	responses := eval.GetAllToolResponses(inv)
	if responses != nil {
		t.Errorf("got %v, want nil", responses)
	}
}

func TestGetAllToolCallsWithResponses_LegacyFormat(t *testing.T) {
	inv := eval.Invocation{
		IntermediateData: &eval.LegacyIntermediateData{
			ToolUses: []genai.FunctionCall{
				{Name: "get_weather"},
			},
			ToolResponses: []genai.FunctionResponse{
				{Name: "get_weather", Response: map[string]any{"temp": 72}},
			},
		},
	}

	pairs := eval.GetAllToolCallsWithResponses(inv)
	if len(pairs) != 1 {
		t.Fatalf("len(pairs) = %d, want 1", len(pairs))
	}
	if pairs[0].Call == nil || pairs[0].Call.Name != "get_weather" {
		t.Error("Call not set correctly")
	}
	if pairs[0].Response == nil || pairs[0].Response.Name != "get_weather" {
		t.Error("Response not set correctly")
	}
}

func TestGetAllToolCallsWithResponses_EventsFormat_PairsByID(t *testing.T) {
	inv := eval.Invocation{
		IntermediateData: &eval.InvocationEventsData{
			Events: []eval.InvocationEvent{
				{
					Author: "model",
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							{FunctionCall: &genai.FunctionCall{ID: "call-1", Name: "get_weather"}},
							{FunctionCall: &genai.FunctionCall{ID: "call-2", Name: "get_time"}},
						},
					},
				},
				{
					Author: "user",
					Content: &genai.Content{
						Role: "user",
						Parts: []*genai.Part{
							{FunctionResponse: &genai.FunctionResponse{ID: "call-2", Name: "get_time", Response: map[string]any{"hour": 14}}},
							{FunctionResponse: &genai.FunctionResponse{ID: "call-1", Name: "get_weather", Response: map[string]any{"temp": 72}}},
						},
					},
				},
			},
		},
	}

	pairs := eval.GetAllToolCallsWithResponses(inv)
	if len(pairs) != 2 {
		t.Fatalf("len(pairs) = %d, want 2", len(pairs))
	}

	// call-1 → get_weather response (out of order in the events list).
	if pairs[0].Call == nil || pairs[0].Call.ID != "call-1" {
		t.Fatalf("pairs[0].Call = %+v, want ID call-1", pairs[0].Call)
	}
	if pairs[0].Response == nil || pairs[0].Response.ID != "call-1" {
		t.Errorf("pairs[0].Response = %+v, want ID call-1", pairs[0].Response)
	}

	// call-2 → get_time response.
	if pairs[1].Call == nil || pairs[1].Call.ID != "call-2" {
		t.Fatalf("pairs[1].Call = %+v, want ID call-2", pairs[1].Call)
	}
	if pairs[1].Response == nil || pairs[1].Response.ID != "call-2" {
		t.Errorf("pairs[1].Response = %+v, want ID call-2", pairs[1].Response)
	}
}

func TestGetAllToolCallsWithResponses_EventsFormat_UnpairedCall(t *testing.T) {
	inv := eval.Invocation{
		IntermediateData: &eval.InvocationEventsData{
			Events: []eval.InvocationEvent{
				{
					Author: "model",
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							{FunctionCall: &genai.FunctionCall{ID: "call-1", Name: "get_weather"}},
						},
					},
				},
			},
		},
	}

	pairs := eval.GetAllToolCallsWithResponses(inv)
	if len(pairs) != 1 {
		t.Fatalf("len(pairs) = %d, want 1", len(pairs))
	}
	if pairs[0].Call == nil || pairs[0].Call.ID != "call-1" {
		t.Fatalf("Call = %+v, want ID call-1", pairs[0].Call)
	}
	if pairs[0].Response != nil {
		t.Errorf("Response = %+v, want nil (no matching response)", pairs[0].Response)
	}
}
