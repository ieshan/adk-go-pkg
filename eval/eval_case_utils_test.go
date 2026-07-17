package eval

import (
	"testing"

	"google.golang.org/genai"
)

func TestGetAllToolCalls_LegacyFormat(t *testing.T) {
	inv := Invocation{
		IntermediateData: &LegacyIntermediateData{
			ToolUses: []genai.FunctionCall{
				{Name: "get_weather", Args: map[string]any{"city": "SF"}},
				{Name: "get_time"},
			},
		},
	}

	calls := GetAllToolCalls(inv)
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2", len(calls))
	}
	if calls[0].Name != "get_weather" {
		t.Errorf("calls[0].Name = %q, want %q", calls[0].Name, "get_weather")
	}
}

func TestGetAllToolCalls_NilIntermediateData(t *testing.T) {
	inv := Invocation{}
	calls := GetAllToolCalls(inv)
	if calls != nil {
		t.Errorf("expected nil calls, got %v", calls)
	}
}

func TestGetAllToolResponses_LegacyFormat(t *testing.T) {
	inv := Invocation{
		IntermediateData: &LegacyIntermediateData{
			ToolResponses: []genai.FunctionResponse{
				{Name: "get_weather", Response: map[string]any{"temp": 72}},
			},
		},
	}

	responses := GetAllToolResponses(inv)
	if len(responses) != 1 {
		t.Fatalf("len(responses) = %d, want 1", len(responses))
	}
	if responses[0].Name != "get_weather" {
		t.Errorf("responses[0].Name = %q, want %q", responses[0].Name, "get_weather")
	}
}

func TestGetAllToolResponses_NilIntermediateData(t *testing.T) {
	inv := Invocation{}
	responses := GetAllToolResponses(inv)
	if responses != nil {
		t.Errorf("expected nil responses, got %v", responses)
	}
}

func TestGetAllToolCallsWithResponses_LegacyFormat(t *testing.T) {
	inv := Invocation{
		IntermediateData: &LegacyIntermediateData{
			ToolUses: []genai.FunctionCall{
				{Name: "get_weather"},
			},
			ToolResponses: []genai.FunctionResponse{
				{Name: "get_weather", Response: map[string]any{"temp": 72}},
			},
		},
	}

	pairs := GetAllToolCallsWithResponses(inv)
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
