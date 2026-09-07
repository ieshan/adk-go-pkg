package aguiadk

import (
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

// TestConvertInboundMessages_MultiTurnHistory verifies that prior user and
// assistant messages are converted to session events and the latest user
// message becomes the current runner input.
func TestConvertInboundMessages_MultiTurnHistory(t *testing.T) {
	messages := []types.Message{
		{ID: "m1", Role: types.RoleUser, Content: "Hello"},
		{ID: "m2", Role: types.RoleAssistant, Content: "Hi there!"},
		{ID: "m3", Role: types.RoleUser, Content: "What is 2+2?"},
	}

	current, prior, err := convertInboundMessages(messages, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prior) != 2 {
		t.Fatalf("got %d prior events, want 2", len(prior))
	}
	if prior[0].Author != "user" {
		t.Errorf("prior[0] author = %q, want user", prior[0].Author)
	}
	if prior[1].Author != "model" {
		t.Errorf("prior[1] author = %q, want model", prior[1].Author)
	}

	if current == nil || len(current.Parts) == 0 {
		t.Fatal("got no parts in current user content, want parts")
	}
	if current.Parts[0].Text != "What is 2+2?" {
		t.Errorf("current text = %q, want 'What is 2+2?'", current.Parts[0].Text)
	}
}

// TestConvertInboundMessages_RoleTool verifies that RoleTool messages are
// converted to genai.FunctionResponse parts with preserved ToolCallID and
// Name, and JSON-parsed content.
func TestConvertInboundMessages_RoleTool(t *testing.T) {
	messages := []types.Message{
		{ID: "m1", Role: types.RoleUser, Content: "What is the weather?"},
		{
			ID:        "m2",
			Role:      types.RoleAssistant,
			Content:   "Let me check.",
			ToolCalls: []types.ToolCall{{ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "get_weather", Arguments: `{"city":"SF"}`}}},
		},
		{
			ID:         "m3",
			Role:       types.RoleTool,
			Content:    `{"temp": 72, "condition": "sunny"}`,
			ToolCallID: "call_1",
			Name:       "get_weather",
		},
		{ID: "m4", Role: types.RoleUser, Content: "Great, thanks!"},
	}

	_, prior, err := convertInboundMessages(messages, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prior) != 3 {
		t.Fatalf("got %d prior events, want 3", len(prior))
	}

	// The third prior event (index 2) should be the tool response.
	toolEv := prior[2]
	if toolEv.Content == nil || len(toolEv.Content.Parts) == 0 {
		t.Fatal("tool event has no parts")
	}
	fr := toolEv.Content.Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("got no FunctionResponse part, want one")
	}
	if fr.ID != "call_1" {
		t.Errorf("FunctionResponse ID = %q, want call_1", fr.ID)
	}
	if fr.Name != "get_weather" {
		t.Errorf("FunctionResponse Name = %q, want get_weather", fr.Name)
	}
	temp, ok := fr.Response["temp"]
	if !ok {
		t.Errorf("got %v, want 'temp' key in response", fr.Response)
	}
	if temp != float64(72) {
		t.Errorf("temp = %v, want 72", temp)
	}
}

// TestConvertInboundMessages_ToolResponseFallback verifies that non-JSON tool
// content falls back to {"result": content}.
func TestConvertInboundMessages_ToolResponseFallback(t *testing.T) {
	messages := []types.Message{
		{
			ID:         "m1",
			Role:       types.RoleTool,
			Content:    "plain text result",
			ToolCallID: "call_2",
			Name:       "search",
		},
	}

	current, _, err := convertInboundMessages(messages, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if current == nil || len(current.Parts) == 0 {
		t.Fatal("got no parts in current content, want parts")
	}
	fr := current.Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("got no FunctionResponse part, want one")
	}
	result, ok := fr.Response["result"]
	if !ok {
		t.Errorf("got %v, want 'result' fallback key", fr.Response)
	}
	if result != "plain text result" {
		t.Errorf("result = %v, want 'plain text result'", result)
	}
}

// TestConvertInboundMessages_AssistantWithToolCalls verifies that assistant
// messages with tool calls are preserved as FunctionCall parts in prior
// events.
func TestConvertInboundMessages_AssistantWithToolCalls(t *testing.T) {
	messages := []types.Message{
		{ID: "m1", Role: types.RoleUser, Content: "Search for X"},
		{
			ID:      "m2",
			Role:    types.RoleAssistant,
			Content: "Searching now.",
			ToolCalls: []types.ToolCall{
				{ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "search", Arguments: `{"q":"X"}`}},
			},
		},
		{ID: "m3", Role: types.RoleUser, Content: "Show results"},
	}

	_, prior, err := convertInboundMessages(messages, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prior) != 2 {
		t.Fatalf("got %d prior events, want 2", len(prior))
	}

	assistantEv := prior[1]
	if assistantEv.Content == nil {
		t.Fatal("assistant event has no content")
	}
	var hasText, hasFunctionCall bool
	for _, p := range assistantEv.Content.Parts {
		if p.Text != "" {
			hasText = true
		}
		if p.FunctionCall != nil {
			hasFunctionCall = true
			if p.FunctionCall.ID != "call_1" {
				t.Errorf("FunctionCall ID = %q, want call_1", p.FunctionCall.ID)
			}
			if p.FunctionCall.Name != "search" {
				t.Errorf("FunctionCall Name = %q, want search", p.FunctionCall.Name)
			}
		}
	}
	if !hasText {
		t.Error("got no text part in assistant event, want one")
	}
	if !hasFunctionCall {
		t.Error("got no FunctionCall part in assistant event, want one")
	}
}

// TestConvertInboundMessages_NoUserMessage verifies that when there is no user
// message, tool responses are prepended to a user container.
func TestConvertInboundMessages_NoUserMessage(t *testing.T) {
	messages := []types.Message{
		{
			ID:         "m1",
			Role:       types.RoleTool,
			Content:    `{"ok": true}`,
			ToolCallID: "call_1",
			Name:       "tool_a",
		},
	}

	current, _, err := convertInboundMessages(messages, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if current == nil {
		t.Fatal("got nil current content, want non-nil")
	}
	if current.Role != "user" {
		t.Errorf("Role = %q, want user", current.Role)
	}
	if len(current.Parts) != 1 {
		t.Fatalf("got %d parts, want 1", len(current.Parts))
	}
	if current.Parts[0].FunctionResponse == nil {
		t.Fatal("got no FunctionResponse part, want one")
	}
}

// TestConvertInboundMessages_Empty verifies the empty-messages edge case.
func TestConvertInboundMessages_Empty(t *testing.T) {
	current, prior, err := convertInboundMessages(nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prior) != 0 {
		t.Errorf("got %d prior events, want 0", len(prior))
	}
	if current == nil || len(current.Parts) == 0 {
		t.Error("got empty current content, want non-empty")
	}
}

// TestParseToolResponseContent covers the JSON parse and fallback paths.
func TestParseToolResponseContent(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		got := parseToolResponseContent(`{"key":"value"}`)
		if got["key"] != "value" {
			t.Errorf("got %v, want key=value", got)
		}
	})
	t.Run("invalid JSON string", func(t *testing.T) {
		got := parseToolResponseContent("not json")
		if got["result"] != "not json" {
			t.Errorf("got %v, want result fallback", got)
		}
	})
	t.Run("nil", func(t *testing.T) {
		got := parseToolResponseContent(nil)
		if got["result"] != "" {
			t.Errorf("got %v, want empty result", got)
		}
	})
	t.Run("map directly", func(t *testing.T) {
		input := map[string]any{"k": "v"}
		got := parseToolResponseContent(input)
		if got["k"] != "v" {
			t.Errorf("got %v, want k=v", got)
		}
	})
}

// FuzzParseToolResponseContent verifies that parseToolResponseContent never
// panics on arbitrary string input. Valid JSON strings should parse into a
// map; invalid JSON should fall back to a {"result": content} map (no panic).
func FuzzParseToolResponseContent(f *testing.F) {
	// Seed: valid JSON object.
	f.Add(`{"key":"value"}`)
	// Seed: malformed JSON.
	f.Add(`invalid json`)
	// Seed: empty string.
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		got := parseToolResponseContent(input)
		// The function must not panic — reaching here is the primary assertion.
		// It always returns a non-nil map.
		if got == nil {
			t.Error("parseToolResponseContent returned nil map")
		}
	})
}
