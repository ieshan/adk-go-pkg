package aguiadk

import (
	"context"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"

	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// drainEvents collects all events currently buffered in ch and returns them.
// The caller must close ch before calling this.
func drainEvents(ch <-chan events.Event) []events.Event {
	var got []events.Event
	for ev := range ch {
		got = append(got, ev)
	}
	return got
}

// newTestTranslator builds an eventTranslator writing to a buffered channel
// and returns both the translator and the channel. Tests close the channel
// and drain it to inspect emitted events.
func newTestTranslator(buf int) (*eventTranslator, chan events.Event) {
	ch := make(chan events.Event, buf)
	emitter := agui.NewEventEmitter(ch)
	return newEventTranslator(emitter, false, nil, false, false, "test-agent"), ch
}

// TestStreamingReasoningLifecycle verifies that multiple partial thought
// chunks stream as REASONING_MESSAGE_CONTENT deltas under a single
// reasoningID instead of spawning a separate reasoning block per chunk.
func TestStreamingReasoningLifecycle(t *testing.T) {
	translator, ch := newTestTranslator(32)

	ev1 := &session.Event{
		LLMResponse: model.LLMResponse{
			Partial: true,
			Content: &genai.Content{
				Parts: []*genai.Part{{Thought: true, Text: "Thinking step 1"}},
			},
		},
	}
	ev2 := &session.Event{
		LLMResponse: model.LLMResponse{
			Partial: true,
			Content: &genai.Content{
				Parts: []*genai.Part{{Thought: true, Text: "Thinking step 1 and step 2"}},
			},
		},
	}
	ev3 := &session.Event{
		LLMResponse: model.LLMResponse{
			Partial: false,
			Content: &genai.Content{
				Parts: []*genai.Part{{Thought: true, Text: "Thinking step 1 and step 2 completed"}},
			},
		},
	}

	translator.translate(ev1)
	translator.translate(ev2)
	translator.translate(ev3)
	close(ch)

	received := drainEvents(ch)

	var starts, ends, contents int
	for _, ev := range received {
		switch ev.Type() {
		case events.EventTypeReasoningStart:
			starts++
		case events.EventTypeReasoningEnd:
			ends++
		case events.EventTypeReasoningMessageContent:
			contents++
		}
	}
	if starts != 1 || ends != 1 {
		t.Fatalf("got starts=%d, ends=%d, want 1 start and 1 end", starts, ends)
	}
	if contents < 2 {
		t.Fatalf("got %d content deltas, want at least 2", contents)
	}
}

// TestReasoningClosesOnText verifies that an open reasoning block is closed
// before a text message starts, so reasoning and text blocks never overlap.
func TestReasoningClosesOnText(t *testing.T) {
	translator, ch := newTestTranslator(32)

	thoughtEv := &session.Event{
		LLMResponse: model.LLMResponse{
			Partial: true,
			Content: &genai.Content{
				Parts: []*genai.Part{{Thought: true, Text: "reasoning"}},
			},
		},
	}
	textEv := &session.Event{
		LLMResponse: model.LLMResponse{
			Partial: false,
			Content: &genai.Content{
				Parts: []*genai.Part{{Text: "answer"}},
			},
		},
	}

	translator.translate(thoughtEv)
	translator.translate(textEv)
	close(ch)

	received := drainEvents(ch)

	// Expect: REASONING_START, REASONING_MESSAGE_START, REASONING_MESSAGE_CONTENT,
	// REASONING_MESSAGE_END, REASONING_END (from closeOpenReasoning in emitText),
	// TEXT_MESSAGE_START, TEXT_MESSAGE_CONTENT, TEXT_MESSAGE_END.
	var reasoningEndedBeforeText bool
	for i, ev := range received {
		if ev.Type() == events.EventTypeTextMessageStart {
			for j := 0; j < i; j++ {
				if received[j].Type() == events.EventTypeReasoningEnd {
					reasoningEndedBeforeText = true
				}
			}
		}
	}
	if !reasoningEndedBeforeText {
		t.Fatal("got TEXT_MESSAGE_START before REASONING_END, want REASONING_END first")
	}
}

// TestDeterministicToolResultID verifies that tool result events emit
// result-<toolCallID> deterministically instead of random UUIDs.
func TestDeterministicToolResultID(t *testing.T) {
	translator, ch := newTestTranslator(10)

	callID := "call_42"
	translator.toolCallIDs["fc_1"] = callID

	translator.emitFunctionResponse(&genai.FunctionResponse{
		ID:       "fc_1",
		Response: map[string]any{"ok": true},
	})
	close(ch)

	for ev := range ch {
		if tr, ok := ev.(*events.ToolCallResultEvent); ok {
			expected := "result-" + callID
			if tr.MessageID != expected {
				t.Fatalf("got %q, want message ID %q", tr.MessageID, expected)
			}
			return
		}
	}
	t.Fatal("got ToolCallResultEvent emitted, want not emitted")
}

// TestNonTextArtifactsFallback verifies that FileData and InlineData parts
// emit readable text fallbacks instead of being silently dropped.
func TestNonTextArtifactsFallback(t *testing.T) {
	t.Run("FileData", func(t *testing.T) {
		translator, ch := newTestTranslator(10)

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{FileData: &genai.FileData{FileURI: "https://storage.googleapis.com/test.pdf", MIMEType: "application/pdf"}},
					},
				},
			},
		}
		translator.translate(ev)
		close(ch)

		found := false
		for ev := range ch {
			if ev.Type() == events.EventTypeTextMessageContent {
				found = true
			}
		}
		if !found {
			t.Fatal("got no text fallback for non-text FileData, want one")
		}
	})

	t.Run("InlineData", func(t *testing.T) {
		translator, ch := newTestTranslator(10)

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{InlineData: &genai.Blob{MIMEType: "image/png", Data: []byte("fake-bytes")}},
					},
				},
			},
		}
		translator.translate(ev)
		close(ch)

		found := false
		for ev := range ch {
			if ev.Type() == events.EventTypeTextMessageContent {
				found = true
			}
		}
		if !found {
			t.Fatal("got no text fallback for non-text InlineData, want one")
		}
	})
}

// TestToolDisambiguation_ServerVsClient verifies that server tools do not
// emit TOOL_CALL_START when client tools are configured. Server tools
// should emit an ACTIVITY_SNAPSHOT instead so the client is not confused
// about which calls it must execute.
func TestToolDisambiguation_ServerVsClient(t *testing.T) {
	ch := make(chan events.Event, 32)
	emitter := agui.NewEventEmitter(ch)
	clientTools := map[string]struct{}{"client_ask_user": {}}
	translator := newEventTranslatorWithClientTools(emitter, false, nil, false, false, "test-agent", clientTools)

	translator.emitFunctionCall(&genai.FunctionCall{
		ID:   "call_server",
		Name: "db_lookup",
		Args: map[string]any{"id": "user_1"},
	}, false)
	close(ch)

	for ev := range ch {
		if ev.Type() == events.EventTypeToolCallStart {
			t.Fatalf("server tool should not emit TOOL_CALL_START when client tools are configured")
		}
	}
}

// TestSessionEventsToMessages_CompleteFidelity verifies that
// sessionEventsToMessages preserves tool calls on assistant messages, emits
// separate RoleTool messages for FunctionResponse parts with deterministic
// "result-<id>" IDs, and preserves the event Author as the message Name for
// sub-agent attribution.
func TestSessionEventsToMessages_CompleteFidelity(t *testing.T) {
	svc := session.InMemoryService()
	ctx := context.Background()
	createResp, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "test-app",
		UserID:  "test-user",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess := createResp.Session

	// Assistant message with text + tool call.
	assistantEv := session.NewEvent(ctx, "inv-1")
	assistantEv.Author = "weather-agent"
	assistantEv.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Parts: []*genai.Part{
				{Text: "Checking the weather..."},
				{FunctionCall: &genai.FunctionCall{ID: "call_1", Name: "get_weather", Args: map[string]any{"city": "Paris"}}},
			},
		},
	}
	if err := svc.AppendEvent(ctx, sess, assistantEv); err != nil {
		t.Fatalf("AppendEvent assistant: %v", err)
	}

	// Tool response.
	toolEv := session.NewEvent(ctx, "inv-2")
	toolEv.Author = "user"
	toolEv.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Parts: []*genai.Part{
				{FunctionResponse: &genai.FunctionResponse{ID: "call_1", Name: "get_weather", Response: map[string]any{"temp": 20}}},
			},
		},
	}
	if err := svc.AppendEvent(ctx, sess, toolEv); err != nil {
		t.Fatalf("AppendEvent tool: %v", err)
	}

	msgs := sessionEventsToMessages(sess.Events())
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2: %+v", len(msgs), msgs)
	}

	// First message: assistant with text + tool call, attributed to sub-agent.
	if msgs[0].Role != types.RoleAssistant {
		t.Errorf("msgs[0].Role = %q, want assistant", msgs[0].Role)
	}
	if msgs[0].Name != "weather-agent" {
		t.Errorf("msgs[0].Name = %q, want weather-agent", msgs[0].Name)
	}
	if msgs[0].Content != "Checking the weather..." {
		t.Errorf("msgs[0].Content = %q, want 'Checking the weather...'", msgs[0].Content)
	}
	if len(msgs[0].ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(msgs[0].ToolCalls))
	}
	tc := msgs[0].ToolCalls[0]
	if tc.ID != "call_1" {
		t.Errorf("tool call ID = %q, want call_1", tc.ID)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("tool call name = %q, want get_weather", tc.Function.Name)
	}
	if tc.Function.Arguments != `{"city":"Paris"}` {
		t.Errorf("tool call args = %q, want {\"city\":\"Paris\"}", tc.Function.Arguments)
	}

	// Second message: RoleTool with deterministic ID and ToolCallID.
	if msgs[1].Role != types.RoleTool {
		t.Errorf("msgs[1].Role = %q, want tool", msgs[1].Role)
	}
	if msgs[1].ID != "result-call_1" {
		t.Errorf("msgs[1].ID = %q, want result-call_1", msgs[1].ID)
	}
	if msgs[1].ToolCallID != "call_1" {
		t.Errorf("msgs[1].ToolCallID = %q, want call_1", msgs[1].ToolCallID)
	}
	if msgs[1].Name != "get_weather" {
		t.Errorf("msgs[1].Name = %q, want get_weather", msgs[1].Name)
	}
}
