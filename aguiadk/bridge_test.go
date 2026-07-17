package aguiadk_test

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"strings"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"github.com/ieshan/adk-go-pkg/aguiadk"
	"github.com/ieshan/adk-go-pkg/testutil"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// collectEvents runs the bridge agent and collects all emitted AG-UI events.
func collectEvents(t *testing.T, aguiAgent interface {
	Run(context.Context, types.RunAgentInput) iter.Seq2[events.Event, error]
}, input types.RunAgentInput) []events.Event {
	t.Helper()
	ctx := context.Background()
	var collected []events.Event
	for ev, err := range aguiAgent.Run(ctx, input) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ev != nil {
			collected = append(collected, ev)
		}
	}
	return collected
}

// defaultInput returns a minimal RunAgentInput with a user message.
func defaultInput() types.RunAgentInput {
	return types.RunAgentInput{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Messages: []types.Message{
			{
				ID:      "msg-1",
				Role:    types.RoleUser,
				Content: "Hello",
			},
		},
	}
}

func TestBridge_ConfigValidation(t *testing.T) {
	t.Run("missing agent", func(t *testing.T) {
		_, err := aguiadk.New(aguiadk.Config{})
		if err == nil {
			t.Fatal("expected error for missing agent")
		}
	})

	t.Run("both AppName and AppNameFunc", func(t *testing.T) {
		a := testutil.MustNewFakeAgent("test")
		_, err := aguiadk.New(aguiadk.Config{
			Agent:       a,
			AppName:     "app1",
			AppNameFunc: func(r *http.Request) string { return "app2" },
		})
		if err == nil {
			t.Fatal("expected error for both AppName and AppNameFunc")
		}
	})

	t.Run("both UserID and UserIDFunc", func(t *testing.T) {
		a := testutil.MustNewFakeAgent("test")
		_, err := aguiadk.New(aguiadk.Config{
			Agent:  a,
			UserID: "user1",
			UserIDFunc: func(r *http.Request) string {
				return "user2"
			},
		})
		if err == nil {
			t.Fatal("expected error for both UserID and UserIDFunc")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		a := testutil.MustNewFakeAgent("test")
		_, err := aguiadk.New(aguiadk.Config{
			Agent:   a,
			AppName: "myapp",
			UserID:  "user1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestBridge_TextParts(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Hello, world!"}},
		},
		Partial: false,
	}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	// Expect: RUN_STARTED, STATE_SNAPSHOT, TEXT_MESSAGE_START, TEXT_MESSAGE_CONTENT, TEXT_MESSAGE_END, RUN_FINISHED
	typeSeq := eventTypes(collected)
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeTextMessageStart,
		events.EventTypeTextMessageContent,
		events.EventTypeTextMessageEnd,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_StreamingTextParts(t *testing.T) {
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Hello"}},
		},
		Partial: true,
	}

	ev2 := session.NewEvent(context.Background(), "inv-1")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Hello, world!"}},
		},
		Partial: false,
	}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev1, nil) {
				return
			}
			if !yield(ev2, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	typeSeq := eventTypes(collected)
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeTextMessageStart,
		events.EventTypeTextMessageContent, // "Hello"
		events.EventTypeTextMessageContent, // ", world!" (delta)
		events.EventTypeTextMessageEnd,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_FunctionCallParts(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{
					FunctionCall: &genai.FunctionCall{
						Name: "get_weather",
						Args: map[string]any{"city": "SF"},
					},
				},
			},
		},
		Partial: false,
	}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	typeSeq := eventTypes(collected)
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs,
		events.EventTypeToolCallEnd,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_FunctionResponseParts(t *testing.T) {
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{
					FunctionCall: &genai.FunctionCall{
						ID:   "fc-1",
						Name: "get_weather",
						Args: map[string]any{"city": "SF"},
					},
				},
			},
		},
		Partial: false,
	}

	ev2 := session.NewEvent(context.Background(), "inv-2")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{
					FunctionResponse: &genai.FunctionResponse{
						ID:       "fc-1",
						Name:     "get_weather",
						Response: map[string]any{"temp": 72},
					},
				},
			},
		},
		Partial: false,
	}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev1, nil) {
				return
			}
			if !yield(ev2, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs,
		events.EventTypeToolCallEnd,
		events.EventTypeToolCallResult,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)

	// Verify TOOL_CALL_RESULT correlates to TOOL_CALL_START.
	var toolCallStartID string
	for _, ev := range collected {
		if ev.Type() == events.EventTypeToolCallStart {
			if tcse, ok := ev.(*events.ToolCallStartEvent); ok {
				toolCallStartID = tcse.ToolCallID
			}
		}
		if ev.Type() == events.EventTypeToolCallResult {
			tcre, ok := ev.(*events.ToolCallResultEvent)
			if !ok {
				t.Fatalf("expected *events.ToolCallResultEvent, got %T", ev)
			}
			if tcre.ToolCallID != toolCallStartID {
				t.Errorf("TOOL_CALL_RESULT ToolCallID = %q, want %q", tcre.ToolCallID, toolCallStartID)
			}
			if !strings.Contains(tcre.Content, `"temp":72`) {
				t.Errorf("TOOL_CALL_RESULT Content = %q, want it to contain %q", tcre.Content, `"temp":72`)
			}
		}
	}
}

func TestBridge_ThoughtParts(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{Text: "Let me think...", Thought: true},
			},
		},
		Partial: false,
	}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	typeSeq := eventTypes(collected)
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeReasoningStart,
		events.EventTypeReasoningMessageStart,
		events.EventTypeReasoningMessageContent,
		events.EventTypeReasoningMessageEnd,
		events.EventTypeReasoningEnd,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_StateSnapshot(t *testing.T) {
	t.Run("enabled by default", func(t *testing.T) {
		a := testutil.MustNewFakeAgent("test-agent")
		bridgeAgent, err := aguiadk.New(aguiadk.Config{
			Agent:   a,
			AppName: "testapp",
			UserID:  "user1",
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		collected := collectEvents(t, bridgeAgent, defaultInput())
		typeSeq := eventTypes(collected)

		found := false
		for _, et := range typeSeq {
			if et == events.EventTypeStateSnapshot {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("expected STATE_SNAPSHOT event, got none")
		}
	})

	t.Run("disabled", func(t *testing.T) {
		a := testutil.MustNewFakeAgent("test-agent")
		bridgeAgent, err := aguiadk.New(aguiadk.Config{
			Agent:             a,
			AppName:           "testapp",
			UserID:            "user1",
			EmitStateSnapshot: new(false),
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		collected := collectEvents(t, bridgeAgent, defaultInput())
		typeSeq := eventTypes(collected)

		for _, et := range typeSeq {
			if et == events.EventTypeStateSnapshot {
				t.Fatal("STATE_SNAPSHOT should not be emitted when disabled")
			}
		}
	})
}

func TestBridge_StateDelta(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "done"}},
		},
		Partial: false,
	}
	ev.Actions.StateDelta["key1"] = "value1"

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	found := false
	for _, et := range typeSeq {
		if et == events.EventTypeStateDelta {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected STATE_DELTA event")
	}
}

func TestBridge_MessagesSnapshot(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Hi"}},
		},
		Partial: false,
	}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:                a,
		AppName:              "testapp",
		UserID:               "user1",
		EmitMessagesSnapshot: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	found := false
	for _, et := range typeSeq {
		if et == events.EventTypeMessagesSnapshot {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected MESSAGES_SNAPSHOT event")
	}
}

func TestBridge_MixedParts(t *testing.T) {
	// Event with text followed by function call.
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{Text: "I'll look that up."},
				{FunctionCall: &genai.FunctionCall{Name: "search", Args: map[string]any{"q": "test"}}},
			},
		},
		Partial: false,
	}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	// Text message should be started, then closed before tool call starts.
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeTextMessageStart,
		events.EventTypeTextMessageContent,
		events.EventTypeTextMessageEnd, // closed before tool call
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs,
		events.EventTypeToolCallEnd,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_NoUserMessage(t *testing.T) {
	// No events from agent, no user message.
	a := testutil.MustNewFakeAgent("test-agent")

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	input := types.RunAgentInput{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Messages: nil,
	}

	collected := collectEvents(t, bridgeAgent, input)
	typeSeq := eventTypes(collected)

	// Should still get RUN_STARTED, STATE_SNAPSHOT, RUN_FINISHED.
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_PanicRecovery(t *testing.T) {
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			panic("intentional test panic")
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	hasRunError := false
	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunError {
			hasRunError = true
			errEvt, ok := ev.(*events.RunErrorEvent)
			if !ok {
				t.Fatalf("expected *events.RunErrorEvent, got %T", ev)
			}
			if errEvt.RunID() != "run-1" {
				t.Errorf("RunID = %q, want %q", errEvt.RunID(), "run-1")
			}
			if !strings.Contains(errEvt.Message, "intentional test panic") {
				t.Errorf("Message = %q, want it to contain %q", errEvt.Message, "intentional test panic")
			}
		}
	}
	if !hasRunError {
		t.Fatalf("expected RUN_ERROR event, got: %v", eventTypes(collected))
	}
}

func TestBridge_RunErrorHasRunID(t *testing.T) {
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			yield(nil, fmt.Errorf("agent failure"))
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunError {
			errEvt, ok := ev.(*events.RunErrorEvent)
			if !ok {
				t.Fatalf("expected *events.RunErrorEvent, got %T", ev)
			}
			if errEvt.RunID() != "run-1" {
				t.Errorf("RunID = %q, want %q", errEvt.RunID(), "run-1")
			}
			return
		}
	}
	t.Fatalf("expected RUN_ERROR event, got: %v", eventTypes(collected))
}

func TestBridge_InputState(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "ok"}},
		},
		Partial: false,
	}

	var capturedState map[string]any
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			// Capture state from the session.
			for k, v := range ctx.Session().State().All() {
				if capturedState == nil {
					capturedState = make(map[string]any)
				}
				capturedState[k] = v
			}
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	input := defaultInput()
	input.State = map[string]any{"custom_key": "custom_value"}

	collectEvents(t, bridgeAgent, input)

	if capturedState == nil {
		t.Fatal("expected non-nil captured state")
	}
	if v, ok := capturedState["custom_key"]; !ok || v != "custom_value" {
		t.Errorf("capturedState[custom_key] = %v, want custom_value", v)
	}
}

func TestBridge_MultimodalMessage(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "received"}},
		},
		Partial: false,
	}

	var partCount int
	var hasText, hasInlineData bool
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			content := ctx.UserContent()
			if content != nil {
				for _, p := range content.Parts {
					partCount++
					if p.Text != "" {
						hasText = true
					}
					if p.InlineData != nil {
						hasInlineData = true
					}
				}
			}
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Small valid PNG base64 (1x1 transparent pixel).
	pngBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	input := types.RunAgentInput{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Messages: []types.Message{
			{
				ID:   "msg-1",
				Role: types.RoleUser,
				Content: []types.InputContent{
					{Type: types.InputContentTypeText, Text: "describe this"},
					{
						Type: types.InputContentTypeImage,
						Source: &types.InputContentSource{
							Type:     types.InputContentSourceTypeData,
							Value:    pngBase64,
							MimeType: "image/png",
						},
					},
				},
			},
		},
	}

	collectEvents(t, bridgeAgent, input)

	if partCount != 2 {
		t.Errorf("partCount = %d, want 2", partCount)
	}
	if !hasText {
		t.Error("expected a text part")
	}
	if !hasInlineData {
		t.Error("expected an inline data part")
	}
}

func TestBridge_LongRunningToolInterrupt(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{
					FunctionCall: &genai.FunctionCall{
						ID:   "fc-1",
						Name: "approve",
						Args: map[string]any{"action": "approve"},
					},
				},
			},
		},
		Partial: false,
	}
	ev.LongRunningToolIDs = []string{"fc-1"}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs,
		events.EventTypeToolCallEnd,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)

	// Verify RUN_FINISHED has interrupt outcome.
	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunFinished {
			finEvt, ok := ev.(*events.RunFinishedEvent)
			if !ok {
				t.Fatalf("expected *events.RunFinishedEvent, got %T", ev)
			}
			if finEvt.Outcome == nil {
				t.Fatal("expected non-nil Outcome")
			}
			if finEvt.Outcome.Type != events.RunFinishedOutcomeTypeInterrupt {
				t.Errorf("Outcome.Type = %q, want %q", finEvt.Outcome.Type, events.RunFinishedOutcomeTypeInterrupt)
			}
			if len(finEvt.Outcome.Interrupts) != 1 {
				t.Fatalf("expected 1 interrupt, got %d", len(finEvt.Outcome.Interrupts))
			}
			if finEvt.Outcome.Interrupts[0].ID != "fc-1" {
				t.Errorf("Interrupts[0].ID = %q, want %q", finEvt.Outcome.Interrupts[0].ID, "fc-1")
			}
		}
	}
}

func TestBridge_ResumeEntries(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "resumed"}},
		},
		Partial: false,
	}

	var sawFunctionResponse bool
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			// Check session events for FunctionResponse.
			for e := range ctx.Session().Events().All() {
				if e.Content != nil {
					for _, p := range e.Content.Parts {
						if p.FunctionResponse != nil {
							sawFunctionResponse = true
						}
					}
				}
			}
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	input := defaultInput()
	input.Resume = []types.ResumeEntry{
		{
			InterruptID: "fc-1",
			Status:      types.ResumeStatusResolved,
			Payload:     "approved",
		},
	}

	collectEvents(t, bridgeAgent, input)

	if !sawFunctionResponse {
		t.Error("expected to see a FunctionResponse in session events from resume")
	}
}

// --- test helpers ---

func eventTypes(inpEvents []events.Event) []events.EventType {
	var evtTypes []events.EventType
	for _, ev := range inpEvents {
		evtTypes = append(evtTypes, ev.Type())
	}
	return evtTypes
}

func assertEventSequence(t *testing.T, got, want []events.EventType) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("event count mismatch: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("event[%d] mismatch: got %s, want %s\nfull got:  %v\nfull want: %v", i, got[i], want[i], got, want)
		}
	}
}
