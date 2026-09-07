package aguiadk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"github.com/ieshan/adk-go-pkg/agui"
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
			t.Fatal("got nil error, want error for missing agent")
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
			t.Fatal("got nil error, want error for both AppName and AppNameFunc")
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
			t.Fatal("got nil error, want error for both UserID and UserIDFunc")
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
	// No FunctionResponse in this test → no tool_use activity snapshot
	// (the snapshot is emitted at execution time, not proposal time).
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
		events.EventTypeActivitySnapshot, // tool_use, emitted at FunctionResponse time
		events.EventTypeToolCallResult,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)

	// Verify TOOL_CALL_RESULT correlates to TOOL_CALL_START, and the
	// tool_use ACTIVITY_SNAPSHOT has the expected type and content shape
	// (matching the example server's settlePendingToolCalls).
	var toolCallStartID string
	sawToolUseSnapshot := false
	for _, ev := range collected {
		if ev.Type() == events.EventTypeToolCallStart {
			if tcse, ok := ev.(*events.ToolCallStartEvent); ok {
				toolCallStartID = tcse.ToolCallID
			}
		}
		if ev.Type() == events.EventTypeActivitySnapshot {
			ase, ok := ev.(*events.ActivitySnapshotEvent)
			if !ok {
				t.Fatalf("got %T, want *events.ActivitySnapshotEvent", ev)
			}
			if ase.ActivityType != "tool_use" {
				t.Errorf("ACTIVITY_SNAPSHOT ActivityType = %q, want %q", ase.ActivityType, "tool_use")
			}
			contentMap, _ := ase.Content.(map[string]any)
			text, _ := contentMap["text"].(string)
			if !strings.HasPrefix(text, "Running get_weather(") {
				t.Errorf("ACTIVITY_SNAPSHOT text = %q, want prefix %q", text, "Running get_weather(")
			}
			sawToolUseSnapshot = true
		}
		if ev.Type() == events.EventTypeToolCallResult {
			tcre, ok := ev.(*events.ToolCallResultEvent)
			if !ok {
				t.Fatalf("got %T, want *events.ToolCallResultEvent", ev)
			}
			if tcre.ToolCallID != toolCallStartID {
				t.Errorf("TOOL_CALL_RESULT ToolCallID = %q, want %q", tcre.ToolCallID, toolCallStartID)
			}
			if !strings.Contains(tcre.Content, `"temp":72`) {
				t.Errorf("TOOL_CALL_RESULT Content = %q, want it to contain %q", tcre.Content, `"temp":72`)
			}
		}
	}
	if !sawToolUseSnapshot {
		t.Error("got no tool_use ACTIVITY_SNAPSHOT, want one")
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
			t.Fatal("got none, want STATE_SNAPSHOT event")
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
		t.Fatal("got no STATE_DELTA event, want one")
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
		t.Fatal("got no MESSAGES_SNAPSHOT event, want one")
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
		// No tool_use activity snapshot — no FunctionResponse in this test.
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
				t.Fatalf("got %T, want *events.RunErrorEvent", ev)
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
		t.Fatalf("got %v, want RUN_ERROR event", eventTypes(collected))
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
				t.Fatalf("got %T, want *events.RunErrorEvent", ev)
			}
			if errEvt.RunID() != "run-1" {
				t.Errorf("RunID = %q, want %q", errEvt.RunID(), "run-1")
			}
			return
		}
	}
	t.Fatalf("got %v, want RUN_ERROR event", eventTypes(collected))
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
		t.Fatal("got nil captured state, want non-nil")
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
		t.Error("got no text part, want one")
	}
	if !hasInlineData {
		t.Error("got no inline data part, want one")
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
		events.EventTypeActivitySnapshot, // approval_request (no tool_use — paused)
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)

	// Verify RUN_FINISHED has interrupt outcome with ResponseSchema and Message.
	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunFinished {
			finEvt, ok := ev.(*events.RunFinishedEvent)
			if !ok {
				t.Fatalf("got %T, want *events.RunFinishedEvent", ev)
			}
			if finEvt.Outcome == nil {
				t.Fatal("got nil Outcome, want non-nil")
			}
			if finEvt.Outcome.Type != events.RunFinishedOutcomeTypeInterrupt {
				t.Errorf("Outcome.Type = %q, want %q", finEvt.Outcome.Type, events.RunFinishedOutcomeTypeInterrupt)
			}
			if len(finEvt.Outcome.Interrupts) != 1 {
				t.Fatalf("got %d interrupts, want 1", len(finEvt.Outcome.Interrupts))
			}
			intr := finEvt.Outcome.Interrupts[0]
			if intr.ID != "fc-1" {
				t.Errorf("Interrupts[0].ID = %q, want %q", intr.ID, "fc-1")
			}
			if intr.Message == "" {
				t.Error("got empty Message on interrupt, want non-empty")
			}
			if intr.ResponseSchema == nil {
				t.Error("got nil ResponseSchema on interrupt, want non-nil")
			}
			if _, ok := intr.ResponseSchema["properties"]; !ok {
				t.Errorf("ResponseSchema missing 'properties': %v", intr.ResponseSchema)
			}
		}
	}
}

func TestBridge_ClientToolNextRunInterrupt(t *testing.T) {
	// Verifies the NextRun client-tool hand-back: when Config.ClientTools is set
	// to NextRun mode and ADK emits a FunctionCall + LongRunningToolIDs (which
	// is what ADK does after the clientProxyHandler returns nil), the bridge
	// emits TOOL_CALL_* events and finishes with an interrupt. The client
	// fulfills the tool call in a follow-up run.
	//
	// This simulates the ADK event sequence that the FakeAgent cannot produce
	// on its own (the FakeAgent bypasses the LLM/tool flow), so we emit the
	// events directly as ADK would.
	interruptEv := session.NewEvent(context.Background(), "inv-1")
	interruptEv.Author = "test-agent"
	interruptEv.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-client-1",
					Name: "client_search",
					Args: map[string]any{"query": "golang adk"},
				},
			}},
		},
		Partial: false,
	}
	// LongRunningToolIDs is set by ADK after the handler returns nil with
	// IsLongRunning=true — this is the NextRun pause signal.
	interruptEv.LongRunningToolIDs = []string{"fc-client-1"}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(interruptEv, nil) {
				return
			}
		}
	})

	store := aguiadk.NewRunStore()
	t.Cleanup(store.Stop)

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:    a,
		AppName:  "testapp",
		UserID:   "user1",
		RunStore: store,
		ClientTools: &aguiadk.ClientToolConfig{
			Mode: aguiadk.ClientToolModeNextRun,
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	input := defaultInput()
	input.Tools = []types.Tool{
		{Name: "client_search", Description: "Search the web"},
	}
	collected := collectEvents(t, bridgeAgent, input)
	typeSeq := eventTypes(collected)

	// TOOL_CALL_* emitted by the eventTranslator, then ACTIVITY_SNAPSHOT
	// (approval_request from the interrupt path), then RUN_FINISHED with
	// interrupt outcome. No tool_use snapshot — the tool is paused, not
	// executed.
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs,
		events.EventTypeToolCallEnd,
		events.EventTypeActivitySnapshot, // approval_request
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)

	// Verify RUN_FINISHED has interrupt outcome for the client tool call.
	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunFinished {
			finEvt, ok := ev.(*events.RunFinishedEvent)
			if !ok {
				t.Fatalf("got %T, want *events.RunFinishedEvent", ev)
			}
			if finEvt.Outcome == nil {
				t.Fatal("got nil Outcome, want non-nil")
			}
			if finEvt.Outcome.Type != events.RunFinishedOutcomeTypeInterrupt {
				t.Errorf("Outcome.Type = %q, want %q", finEvt.Outcome.Type, events.RunFinishedOutcomeTypeInterrupt)
			}
			if len(finEvt.Outcome.Interrupts) != 1 {
				t.Fatalf("got %d interrupts, want 1", len(finEvt.Outcome.Interrupts))
			}
			if finEvt.Outcome.Interrupts[0].ID != "fc-client-1" {
				t.Errorf("Interrupts[0].ID = %q, want %q", finEvt.Outcome.Interrupts[0].ID, "fc-client-1")
			}
		}
	}

	// Verify the paused run was saved to the runstore for resume.
	saved, ok := store.Load(aguiadk.RunKey(input.ThreadID, input.RunID))
	if !ok {
		t.Fatal("got no paused run in runstore, want one saved")
	}
	if len(saved.Pending) != 1 {
		t.Fatalf("got %d pending tool calls, want 1", len(saved.Pending))
	}
	if saved.Pending[0].ID != "fc-client-1" {
		t.Errorf("Pending[0].ID = %q, want %q", saved.Pending[0].ID, "fc-client-1")
	}
	if saved.Pending[0].Name != "client_search" {
		t.Errorf("Pending[0].Name = %q, want %q", saved.Pending[0].Name, "client_search")
	}
}

func TestBridge_ResumeEntries(t *testing.T) {
	// Phase 1: trigger a long-running tool interrupt to populate the runstore.
	interruptEv := session.NewEvent(context.Background(), "inv-1")
	interruptEv.Author = "test-agent"
	interruptEv.LLMResponse = model.LLMResponse{
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
	interruptEv.LongRunningToolIDs = []string{"fc-1"}

	resumeEv := session.NewEvent(context.Background(), "inv-2")
	resumeEv.Author = "test-agent"
	resumeEv.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "resumed"}},
		},
		Partial: false,
	}

	var sawFunctionResponse bool
	var callCount int32
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			n := atomic.AddInt32(&callCount, 1)
			if n == 1 {
				// Phase 1: emit the interrupt event.
				if !yield(interruptEv, nil) {
					return
				}
				return
			}
			// Phase 2: check session events for FunctionResponse (set by the resume path).
			for e := range ctx.Session().Events().All() {
				if e.Content != nil {
					for _, p := range e.Content.Parts {
						if p.FunctionResponse != nil {
							sawFunctionResponse = true
						}
					}
				}
			}
			if !yield(resumeEv, nil) {
				return
			}
		}
	})

	store := aguiadk.NewRunStore()
	t.Cleanup(store.Stop)

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:    a,
		AppName:  "testapp",
		UserID:   "user1",
		RunStore: store,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Phase 1: run with the interrupt event to populate the runstore.
	collectEvents(t, bridgeAgent, defaultInput())

	// Phase 2: resume with an approved entry.
	input := defaultInput()
	input.Resume = []types.ResumeEntry{
		{
			InterruptID: "fc-1",
			Status:      types.ResumeStatusResolved,
			Payload:     map[string]any{"approved": true},
		},
	}

	collected := collectEvents(t, bridgeAgent, input)

	// The resume path re-emits the tool proposal, a tool_use ACTIVITY_SNAPSHOT
	// (approved call → tool executes), and a TOOL_CALL_RESULT.
	hasToolCallResult := false
	hasToolUseSnapshot := false
	for _, ev := range collected {
		if ev.Type() == events.EventTypeToolCallResult {
			hasToolCallResult = true
		}
		if ev.Type() == events.EventTypeActivitySnapshot {
			if ase, ok := ev.(*events.ActivitySnapshotEvent); ok && ase.ActivityType == "tool_use" {
				hasToolUseSnapshot = true
			}
		}
	}
	if !hasToolCallResult {
		t.Errorf("got %v, want TOOL_CALL_RESULT event from resume settlement", eventTypes(collected))
	}
	if !hasToolUseSnapshot {
		t.Errorf("got %v, want tool_use ACTIVITY_SNAPSHOT for approved resume", eventTypes(collected))
	}

	if !sawFunctionResponse {
		t.Error("got no FunctionResponse in session events from resume, want one")
	}
}

func TestBridge_ResumeDenied(t *testing.T) {
	// Trigger an interrupt, then resume with approved:false — the
	// TOOL_CALL_RESULT should carry a denial marker.
	interruptEv := session.NewEvent(context.Background(), "inv-1")
	interruptEv.Author = "test-agent"
	interruptEv.LLMResponse = model.LLMResponse{
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
	interruptEv.LongRunningToolIDs = []string{"fc-1"}

	resumeEv := session.NewEvent(context.Background(), "inv-2")
	resumeEv.Author = "test-agent"
	resumeEv.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "denied"}},
		},
		Partial: false,
	}

	var callCount int32
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			n := atomic.AddInt32(&callCount, 1)
			if n == 1 {
				if !yield(interruptEv, nil) {
					return
				}
				return
			}
			if !yield(resumeEv, nil) {
				return
			}
		}
	})

	store := aguiadk.NewRunStore()
	t.Cleanup(store.Stop)

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:    a,
		AppName:  "testapp",
		UserID:   "user1",
		RunStore: store,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Phase 1: populate runstore via interrupt.
	collectEvents(t, bridgeAgent, defaultInput())

	// Phase 2: resume with denied.
	input := defaultInput()
	input.Resume = []types.ResumeEntry{
		{
			InterruptID: "fc-1",
			Status:      types.ResumeStatusResolved,
			Payload:     map[string]any{"approved": false},
		},
	}

	collected := collectEvents(t, bridgeAgent, input)

	for _, ev := range collected {
		if ev.Type() == events.EventTypeToolCallResult {
			tcre, ok := ev.(*events.ToolCallResultEvent)
			if !ok {
				t.Fatalf("got %T, want *events.ToolCallResultEvent", ev)
			}
			if !strings.Contains(tcre.Content, `"denied":true`) {
				t.Errorf("denied TOOL_CALL_RESULT content = %q, want it to contain %q", tcre.Content, `"denied":true`)
			}
			return
		}
		// Denied calls must NOT emit a tool_use activity snapshot — the
		// tool does not execute.
		if ev.Type() == events.EventTypeActivitySnapshot {
			if ase, ok := ev.(*events.ActivitySnapshotEvent); ok && ase.ActivityType == "tool_use" {
				t.Error("denied resume must not emit a tool_use ACTIVITY_SNAPSHOT")
			}
		}
	}
	t.Fatalf("got %v, want a TOOL_CALL_RESULT event with denial", eventTypes(collected))
}

func TestBridge_ResumeNoPausedRun(t *testing.T) {
	a := testutil.MustNewFakeAgent("test-agent")

	store := aguiadk.NewRunStore()
	t.Cleanup(store.Stop)

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:    a,
		AppName:  "testapp",
		UserID:   "user1",
		RunStore: store,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	input := defaultInput()
	input.Resume = []types.ResumeEntry{
		{
			InterruptID: "fc-1",
			Status:      types.ResumeStatusResolved,
			Payload:     map[string]any{"approved": true},
		},
	}

	collected := collectEvents(t, bridgeAgent, input)

	hasRunError := false
	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunError {
			hasRunError = true
			errEvt, ok := ev.(*events.RunErrorEvent)
			if !ok {
				t.Fatalf("got %T, want *events.RunErrorEvent", ev)
			}
			if !strings.Contains(errEvt.Message, "no paused run") {
				t.Errorf("RunError message = %q, want it to contain %q", errEvt.Message, "no paused run")
			}
		}
	}
	if !hasRunError {
		t.Fatal("got no RUN_ERROR for resume with no paused run, want one")
	}
}

func TestBridge_StreamingToolCall(t *testing.T) {
	// Partial event with name+ID but no args, then final event with args.
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{ID: "fc-1", Name: "get_weather"},
			}},
		},
		Partial: true,
	}
	ev2 := session.NewEvent(context.Background(), "inv-1")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{ID: "fc-1", Name: "get_weather", Args: map[string]any{"city": "SF"}},
			}},
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

	bridgeAgent, err := aguiadk.New(aguiadk.Config{Agent: a, AppName: "testapp", UserID: "user1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	// START on partial (no args yet), ARGS on final, END on final.
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs, // final event carries accumulated args
		events.EventTypeToolCallEnd,
		// No tool_use activity snapshot — no FunctionResponse in this test.
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_StreamingToolCallNeverFinalized(t *testing.T) {
	// A partial tool call with no final event — closeStreamedToolCalls
	// should still emit TOOL_CALL_END on run end.
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{ID: "fc-1", Name: "get_weather"},
			}},
		},
		Partial: true,
	}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{Agent: a, AppName: "testapp", UserID: "user1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	// START on partial (nil args → no ARGS), END from closeStreamedToolCalls.
	// No tool_use activity snapshot — the tool never executed (no final event).
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeToolCallStart,
		events.EventTypeToolCallEnd, // synthesized by closeStreamedToolCalls
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_StreamingToolCallPartialArgs(t *testing.T) {
	// Simulates real ADK streaming: Partial events carry PartialArgs (deltas),
	// not accumulated Args. The final non-Partial event carries the accumulated
	// Args. Verifies TOOL_CALL_ARGS is emitted per PartialArg.StringValue delta
	// (matching the example server), not the accumulated args.
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "get_weather",
					// PartialArgs carries the deltas; Args is nil on Partials.
					PartialArgs: []*genai.PartialArg{
						{JsonPath: "$.city", StringValue: `"San"`},
					},
					WillContinue: new(true),
				},
			}},
		},
		Partial: true,
	}
	ev2 := session.NewEvent(context.Background(), "inv-1")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "get_weather",
					PartialArgs: []*genai.PartialArg{
						{JsonPath: "$.city", StringValue: ` Francisco"`},
					},
					WillContinue: new(false),
				},
			}},
		},
		Partial: true,
	}
	ev3 := session.NewEvent(context.Background(), "inv-1")
	ev3.Author = "test-agent"
	ev3.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				// Final event: accumulated Args, no PartialArgs.
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "get_weather",
					Args: map[string]any{"city": "San Francisco"},
				},
			}},
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
			if !yield(ev3, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{Agent: a, AppName: "testapp", UserID: "user1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	// START on first partial, ARGS per PartialArg delta, END on final. The
	// final accumulated args are NOT re-emitted because partial deltas were
	// already sent (the client reducer appends deltas). No tool_use snapshot
	// (no FunctionResponse).
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs, // delta: `"San"`
		events.EventTypeToolCallArgs, // delta: ` Francisco"`
		events.EventTypeToolCallEnd,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)

	// Verify the ARGS deltas are the PartialArg.StringValue fragments, not the
	// accumulated args. The accumulated args would be `{"city":"San Francisco"}`
	// which is different from the deltas `"San"` and ` Francisco"`.
	var argsDeltas []string
	for _, ev := range collected {
		if ev.Type() == events.EventTypeToolCallArgs {
			if tcae, ok := ev.(*events.ToolCallArgsEvent); ok {
				argsDeltas = append(argsDeltas, tcae.Delta)
			}
		}
	}
	if len(argsDeltas) != 2 {
		t.Fatalf("got %d TOOL_CALL_ARGS events, want 2 (partials only, no full re-send): %v", len(argsDeltas), argsDeltas)
	}
	if argsDeltas[0] != `"San"` {
		t.Errorf("first ARGS delta = %q, want %q", argsDeltas[0], `"San"`)
	}
	if argsDeltas[1] != ` Francisco"` {
		t.Errorf("second ARGS delta = %q, want %q", argsDeltas[1], ` Francisco"`)
	}
}

// TestBridge_StreamingToolCallArgsNoDuplication verifies that concatenating
// all TOOL_CALL_ARGS deltas for a tool call produces valid JSON equal to the
// final accumulated args. The AG-UI client reducer APPENDS each delta to
// function.arguments (default.ts:513), so the bridge must not emit the full
// args JSON after partial deltas — that would duplicate/corrupt the client's
// concatenated args. Bug B2.
func TestBridge_StreamingToolCallArgsNoDuplication(t *testing.T) {
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "get_weather",
					PartialArgs: []*genai.PartialArg{
						{JsonPath: "$.city", StringValue: `{"city":"`},
					},
					WillContinue: new(true),
				},
			}},
		},
		Partial: true,
	}

	ev2 := session.NewEvent(context.Background(), "inv-1")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "get_weather",
					PartialArgs: []*genai.PartialArg{
						{JsonPath: "$.city", StringValue: `San Francisco"}`},
					},
					WillContinue: new(false),
				},
			}},
		},
		Partial: true,
	}

	ev3 := session.NewEvent(context.Background(), "inv-1")
	ev3.Author = "test-agent"
	ev3.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "get_weather",
					Args: map[string]any{"city": "San Francisco"},
				},
			}},
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
			if !yield(ev3, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{Agent: a, AppName: "testapp", UserID: "user1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	// Collect all TOOL_CALL_ARGS deltas for toolCallId "fc-1".
	// The bridge uses the ADK function call ID as the AG-UI tool call ID
	// when fc.ID is non-empty, so we can match directly.
	var argsDeltas []string
	for _, ev := range collected {
		if ev.Type() == events.EventTypeToolCallArgs {
			if tcae, ok := ev.(*events.ToolCallArgsEvent); ok {
				if tcae.ToolCallID == "fc-1" {
					argsDeltas = append(argsDeltas, tcae.Delta)
				}
			}
		}
	}

	if len(argsDeltas) == 0 {
		t.Fatal("got no TOOL_CALL_ARGS event, want at least one")
	}

	// Simulate the AG-UI client reducer: concatenate all deltas.
	concatenated := strings.Join(argsDeltas, "")

	// The concatenated result must be valid JSON matching the final args.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(concatenated), &parsed); err != nil {
		t.Fatalf("concatenated TOOL_CALL_ARGS deltas are not valid JSON: %s\nerror: %v\ndeltas: %v", concatenated, err, argsDeltas)
	}
	if city, ok := parsed["city"].(string); !ok || city != "San Francisco" {
		t.Errorf("concatenated args city = %v, want %q (full: %s)", parsed["city"], "San Francisco", concatenated)
	}
}

func TestBridge_MalformedToolCall_EmptyName(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{ID: "fc-1", Name: "", Args: map[string]any{"x": 1}},
			}},
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

	bridgeAgent, err := aguiadk.New(aguiadk.Config{Agent: a, AppName: "testapp", UserID: "user1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	// Should emit TOOL_CALL_RESULT with error, not TOOL_CALL_START.
	hasErrorResult := false
	for _, ev := range collected {
		if ev.Type() == events.EventTypeToolCallResult {
			tcre, ok := ev.(*events.ToolCallResultEvent)
			if !ok {
				t.Fatalf("got %T, want *events.ToolCallResultEvent", ev)
			}
			if strings.Contains(tcre.Content, "empty function name") {
				hasErrorResult = true
			}
		}
		if ev.Type() == events.EventTypeToolCallStart {
			t.Error("should not emit TOOL_CALL_START for empty-name call")
		}
	}
	if !hasErrorResult {
		t.Error("got no TOOL_CALL_RESULT with empty-name error, want one")
	}
}

func TestBridge_MalformedToolCall_EmptyID(t *testing.T) {
	// Empty ID should get a synthetic ID — the call should still emit
	// START/ARGS/END, not be dropped.
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{Name: "get_weather", Args: map[string]any{"city": "SF"}},
			}},
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

	bridgeAgent, err := aguiadk.New(aguiadk.Config{Agent: a, AppName: "testapp", UserID: "user1"})
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
		// No tool_use activity snapshot — no FunctionResponse in this test.
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)

	// Verify the synthetic ID is non-empty.
	for _, ev := range collected {
		if ev.Type() == events.EventTypeToolCallStart {
			tcse, ok := ev.(*events.ToolCallStartEvent)
			if !ok {
				t.Fatalf("got %T, want *events.ToolCallStartEvent", ev)
			}
			if tcse.ToolCallID == "" {
				t.Error("got empty synthetic ToolCallID, want non-empty")
			}
		}
	}
}

func TestBridge_SuppressedToolMode(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "update_doc",
					Args: map[string]any{"content": "hello"},
				},
			}},
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

	mapper := func(name string, args map[string]any) ([]events.JSONPatchOperation, bool) {
		if name == "update_doc" {
			return []events.JSONPatchOperation{
				{Op: "add", Path: "/doc", Value: args["content"]},
			}, true
		}
		return nil, false
	}

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:              a,
		AppName:            "testapp",
		UserID:             "user1",
		SuppressToolEvents: true,
		ToolToStateMapper:  mapper,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	hasStateDelta := false
	hasToolCallStart := false
	for _, ev := range collected {
		switch ev.Type() {
		case events.EventTypeStateDelta:
			hasStateDelta = true
			sd, ok := ev.(*events.StateDeltaEvent)
			if !ok {
				t.Fatalf("got %T, want *events.StateDeltaEvent", ev)
			}
			if len(sd.Delta) != 1 || sd.Delta[0].Path != "/doc" {
				t.Errorf("StateDelta = %+v, want one op with path /doc", sd.Delta)
			}
		case events.EventTypeToolCallStart, events.EventTypeToolCallArgs, events.EventTypeToolCallEnd:
			hasToolCallStart = true
		}
	}
	if !hasStateDelta {
		t.Error("got no STATE_DELTA event from suppressed tool mode, want one")
	}
	if hasToolCallStart {
		t.Error("got TOOL_CALL_* events in suppressed mode, want none")
	}
}

func TestBridge_SuppressedToolModeMapperReturnsNil(t *testing.T) {
	// When the mapper returns nil for a tool, normal tool call events
	// should be emitted.
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "unmapped_tool",
					Args: map[string]any{"x": 1},
				},
			}},
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

	mapper := func(name string, args map[string]any) ([]events.JSONPatchOperation, bool) {
		return nil, false // no mapping for this tool
	}

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:              a,
		AppName:            "testapp",
		UserID:             "user1",
		SuppressToolEvents: true,
		ToolToStateMapper:  mapper,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	// Should fall back to normal tool call events.
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs,
		events.EventTypeToolCallEnd,
		// No tool_use activity snapshot — no FunctionResponse in this test.
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_SuppressedToolModeMixed(t *testing.T) {
	// Two tools in one run: one suppressed (returns (ops, true)), one not
	// (returns (nil, false)). Verify STATE_DELTA for the suppressed tool,
	// TOOL_CALL_START/ARGS/END for the non-suppressed tool, and TOOL_CALL_RESULT
	// only for the non-suppressed tool (both get a FunctionResponse).
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{
					FunctionCall: &genai.FunctionCall{
						ID:   "fc-1",
						Name: "update_doc",
						Args: map[string]any{"content": "hello"},
					},
				},
				{
					FunctionCall: &genai.FunctionCall{
						ID:   "fc-2",
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
						Name:     "update_doc",
						Response: map[string]any{"ok": true},
					},
				},
				{
					FunctionResponse: &genai.FunctionResponse{
						ID:       "fc-2",
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

	mapper := func(name string, args map[string]any) ([]events.JSONPatchOperation, bool) {
		if name == "update_doc" {
			return []events.JSONPatchOperation{
				{Op: "add", Path: "/doc", Value: args["content"]},
			}, true
		}
		return nil, false
	}

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:              a,
		AppName:            "testapp",
		UserID:             "user1",
		SuppressToolEvents: true,
		ToolToStateMapper:  mapper,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	// Expected: STATE_DELTA for suppressed tool, TOOL_CALL_START/ARGS/END for
	// non-suppressed tool, then ACTIVITY_SNAPSHOT + TOOL_CALL_RESULT only for
	// the non-suppressed tool (fc-2).
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeStateDelta, // suppressed tool (update_doc)
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs,
		events.EventTypeToolCallEnd,
		events.EventTypeActivitySnapshot, // tool_use for get_weather
		events.EventTypeToolCallResult,   // only for get_weather
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)

	// Verify the STATE_DELTA has the right path and the TOOL_CALL_RESULT
	// correlates to the non-suppressed tool.
	var stateDeltaPath string
	var toolCallResultID string
	var toolCallStartID string
	for _, ev := range collected {
		switch ev.Type() {
		case events.EventTypeStateDelta:
			sd, ok := ev.(*events.StateDeltaEvent)
			if !ok {
				t.Fatalf("got %T, want *events.StateDeltaEvent", ev)
			}
			if len(sd.Delta) != 1 {
				t.Errorf("StateDelta len = %d, want 1", len(sd.Delta))
			} else {
				stateDeltaPath = sd.Delta[0].Path
			}
		case events.EventTypeToolCallStart:
			tcse, ok := ev.(*events.ToolCallStartEvent)
			if !ok {
				t.Fatalf("got %T, want *events.ToolCallStartEvent", ev)
			}
			toolCallStartID = tcse.ToolCallID
		case events.EventTypeToolCallResult:
			tcre, ok := ev.(*events.ToolCallResultEvent)
			if !ok {
				t.Fatalf("got %T, want *events.ToolCallResultEvent", ev)
			}
			toolCallResultID = tcre.ToolCallID
		}
	}
	if stateDeltaPath != "/doc" {
		t.Errorf("StateDelta path = %q, want /doc", stateDeltaPath)
	}
	if toolCallStartID != "fc-2" {
		t.Errorf("ToolCallStart ID = %q, want fc-2", toolCallStartID)
	}
	if toolCallResultID != toolCallStartID {
		t.Errorf("ToolCallResult ID = %q, want %q (should match non-suppressed tool)", toolCallResultID, toolCallStartID)
	}
}

func TestBridge_SuppressedToolModeNilOpsSuppress(t *testing.T) {
	// Mapper returns (nil, true) — verify no STATE_DELTA, no TOOL_CALL_*
	// events, tool call silently swallowed.
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "silent_tool",
					Args: map[string]any{"x": 1},
				},
			}},
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

	mapper := func(name string, args map[string]any) ([]events.JSONPatchOperation, bool) {
		return nil, true // suppress with no state delta
	}

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:              a,
		AppName:            "testapp",
		UserID:             "user1",
		SuppressToolEvents: true,
		ToolToStateMapper:  mapper,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	// Expect only RUN_STARTED, STATE_SNAPSHOT, RUN_FINISHED — no tool events
	// and no STATE_DELTA.
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_SuppressedToolModeNonSuppressedResult(t *testing.T) {
	// Mapper returns (nil, false) with a FunctionResponse — verify
	// TOOL_CALL_RESULT IS emitted (the key new behavior after removing the
	// blanket suppressTools check in emitFunctionResponse).
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "normal_tool",
					Args: map[string]any{"x": 1},
				},
			}},
		},
		Partial: false,
	}

	ev2 := session.NewEvent(context.Background(), "inv-2")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionResponse: &genai.FunctionResponse{
					ID:       "fc-1",
					Name:     "normal_tool",
					Response: map[string]any{"result": "ok"},
				},
			}},
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

	mapper := func(name string, args map[string]any) ([]events.JSONPatchOperation, bool) {
		return nil, false // don't suppress — normal tool events
	}

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:              a,
		AppName:            "testapp",
		UserID:             "user1",
		SuppressToolEvents: true,
		ToolToStateMapper:  mapper,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	// Expect full tool call lifecycle including TOOL_CALL_RESULT.
	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs,
		events.EventTypeToolCallEnd,
		events.EventTypeActivitySnapshot, // tool_use
		events.EventTypeToolCallResult,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)

	// Verify TOOL_CALL_RESULT content contains the response.
	for _, ev := range collected {
		if ev.Type() == events.EventTypeToolCallResult {
			tcre, ok := ev.(*events.ToolCallResultEvent)
			if !ok {
				t.Fatalf("got %T, want *events.ToolCallResultEvent", ev)
			}
			if !strings.Contains(tcre.Content, `"result":"ok"`) {
				t.Errorf("TOOL_CALL_RESULT Content = %q, want it to contain %q", tcre.Content, `"result":"ok"`)
			}
		}
	}
}

// --- test helpers ---

func TestBridge_InterleavedReasoningText(t *testing.T) {
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{Text: "Let me think...", Thought: true},
			},
		},
		Partial: false,
	}

	ev2 := session.NewEvent(context.Background(), "inv-1")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Here is my answer."}},
		},
		Partial: false,
	}

	ev3 := session.NewEvent(context.Background(), "inv-1")
	ev3.Author = "test-agent"
	ev3.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{Text: "Wait, reconsidering...", Thought: true},
			},
		},
		Partial: false,
	}

	ev4 := session.NewEvent(context.Background(), "inv-1")
	ev4.Author = "test-agent"
	ev4.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Final answer."}},
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
			if !yield(ev3, nil) {
				return
			}
			if !yield(ev4, nil) {
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
		// thought 1
		events.EventTypeReasoningStart,
		events.EventTypeReasoningMessageStart,
		events.EventTypeReasoningMessageContent,
		events.EventTypeReasoningMessageEnd,
		events.EventTypeReasoningEnd,
		// text 1 (reasoning auto-closed before text opens)
		events.EventTypeTextMessageStart,
		events.EventTypeTextMessageContent,
		events.EventTypeTextMessageEnd,
		// thought 2 (text auto-closed before reasoning opens)
		events.EventTypeReasoningStart,
		events.EventTypeReasoningMessageStart,
		events.EventTypeReasoningMessageContent,
		events.EventTypeReasoningMessageEnd,
		events.EventTypeReasoningEnd,
		// text 2
		events.EventTypeTextMessageStart,
		events.EventTypeTextMessageContent,
		events.EventTypeTextMessageEnd,
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_StepEvents(t *testing.T) {
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "thinking about it"}},
		},
		Partial: true,
	}

	ev2 := session.NewEvent(context.Background(), "inv-1")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{Text: "thinking about it"},
				{
					FunctionCall: &genai.FunctionCall{
						ID:   "fc-1",
						Name: "search",
						Args: map[string]any{"q": "test"},
					},
				},
			},
		},
		Partial: false,
	}

	ev3 := session.NewEvent(context.Background(), "inv-1")
	ev3.Author = "test-agent"
	ev3.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{
					FunctionResponse: &genai.FunctionResponse{
						ID:       "fc-1",
						Name:     "search",
						Response: map[string]any{"result": "found"},
					},
				},
			},
		},
		Partial: false,
	}

	ev4 := session.NewEvent(context.Background(), "inv-1")
	ev4.Author = "test-agent"
	ev4.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "done"}},
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
			if !yield(ev3, nil) {
				return
			}
			if !yield(ev4, nil) {
				return
			}
		}
	})

	stepOn := true
	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:          a,
		AppName:        "testapp",
		UserID:         "user1",
		EmitStepEvents: &stepOn,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	expected := []events.EventType{
		events.EventTypeRunStarted,
		events.EventTypeStateSnapshot,
		// ev1: partial text — first non-partial hasn't arrived yet, no step
		events.EventTypeTextMessageStart,
		events.EventTypeTextMessageContent,
		// ev2: non-partial, has FunctionCall → STEP_STARTED("tools") (first non-partial
		// with FunctionCall goes directly to tools step)
		events.EventTypeStepStarted, // "tools"
		events.EventTypeTextMessageContent,
		events.EventTypeTextMessageEnd,
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs,
		events.EventTypeToolCallEnd,
		// ev3: FunctionResponse → StepFinished("tools") + StepStarted("llm")
		events.EventTypeStepFinished,     // "tools"
		events.EventTypeStepStarted,      // "llm"
		events.EventTypeActivitySnapshot, // tool_use
		events.EventTypeToolCallResult,
		// ev4: text, non-partial
		events.EventTypeTextMessageStart,
		events.EventTypeTextMessageContent,
		events.EventTypeTextMessageEnd,
		events.EventTypeStepFinished, // "llm" (closeOpenStep at run end)
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_MidStreamErrorDuringToolCallStreaming(t *testing.T) {
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{
					FunctionCall: &genai.FunctionCall{
						ID:   "fc-1",
						Name: "search",
						PartialArgs: []*genai.PartialArg{
							{StringValue: `{"q":"te`},
						},
					},
				},
			},
		},
		Partial: true,
	}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev1, nil) {
				return
			}
			// Then yield an error.
			yield(nil, fmt.Errorf("stream interrupted"))
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

	ctx := context.Background()
	var collected []events.Event
	for ev, err := range bridgeAgent.Run(ctx, defaultInput()) {
		if err != nil {
			// Error is expected — but we should still have collected events.
			_ = err
			continue
		}
		if ev != nil {
			collected = append(collected, ev)
		}
	}

	typeSeq := eventTypes(collected)

	// Verify TOOL_CALL_END is emitted before RUN_ERROR.
	hasToolCallStart := false
	hasToolCallEnd := false
	runErrorIdx := -1
	toolCallEndIdx := -1
	for i, et := range typeSeq {
		if et == events.EventTypeToolCallStart {
			hasToolCallStart = true
		}
		if et == events.EventTypeToolCallEnd {
			hasToolCallEnd = true
			toolCallEndIdx = i
		}
		if et == events.EventTypeRunError {
			runErrorIdx = i
		}
	}

	if !hasToolCallStart {
		t.Error("got no TOOL_CALL_START in event stream, want one")
	}
	if !hasToolCallEnd {
		t.Error("got no TOOL_CALL_END before RUN_ERROR, want one (closeStreamedToolCalls on error path)")
	}
	if runErrorIdx == -1 {
		t.Fatal("got no RUN_ERROR event, want one")
	}
	if toolCallEndIdx == -1 || toolCallEndIdx > runErrorIdx {
		t.Errorf("TOOL_CALL_END (idx %d) must come before RUN_ERROR (idx %d)", toolCallEndIdx, runErrorIdx)
	}
}

func TestBridge_CustomEvent(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "hello"}},
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
		CustomEventEmitter: func(emitter *agui.EventEmitter, toolCallCount int) error {
			return emitter.Custom("agent_complete", map[string]any{"toolCalls": toolCallCount})
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	hasCustom := false
	customIdx := -1
	runFinishedIdx := -1
	for i, et := range typeSeq {
		if et == events.EventTypeCustom {
			hasCustom = true
			customIdx = i
		}
		if et == events.EventTypeRunFinished {
			runFinishedIdx = i
		}
	}

	if !hasCustom {
		t.Fatal("got no CUSTOM event in stream, want one")
	}
	if customIdx > runFinishedIdx {
		t.Errorf("CUSTOM (idx %d) must come before RUN_FINISHED (idx %d)", customIdx, runFinishedIdx)
	}

	// Verify the custom event name.
	for _, ev := range collected {
		if ev.Type() == events.EventTypeCustom {
			customEvt, ok := ev.(*events.CustomEvent)
			if !ok {
				t.Fatalf("got %T, want *events.CustomEvent", ev)
			}
			if customEvt.Name != "agent_complete" {
				t.Errorf("custom event name = %q, want %q", customEvt.Name, "agent_complete")
			}
		}
	}
}

func TestBridge_ReasoningEncryptedValue(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{Text: "Let me think...", Thought: true, ThoughtSignature: []byte("encrypted-sig")},
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

	hasEncrypted := false
	for _, et := range typeSeq {
		if et == events.EventTypeReasoningEncryptedValue {
			hasEncrypted = true
		}
	}
	if !hasEncrypted {
		t.Errorf("got %v, want REASONING_ENCRYPTED_VALUE event", typeSeq)
	}
}

func TestBridge_MaxIterationsExceeded(t *testing.T) {
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "first response"}},
		},
		Partial:      false,
		TurnComplete: true,
	}

	ev2 := session.NewEvent(context.Background(), "inv-1")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "second response"}},
		},
		Partial:      false,
		TurnComplete: true,
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
		Agent:         a,
		AppName:       "testapp",
		UserID:        "user1",
		MaxIterations: 1,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	var collected []events.Event
	for ev, err := range bridgeAgent.Run(ctx, defaultInput()) {
		if err != nil {
			// The MaxIterations RUN_ERROR is delivered as an event, not an
			// iterator error; any iterator error here is unexpected.
			t.Fatalf("unexpected iterator error: %v", err)
		}
		if ev != nil {
			collected = append(collected, ev)
		}
	}

	typeSeq := eventTypes(collected)

	hasRunError := false
	for _, et := range typeSeq {
		if et == events.EventTypeRunError {
			hasRunError = true
		}
	}
	if !hasRunError {
		t.Fatalf("got %v, want RUN_ERROR event", typeSeq)
	}

	// Verify the error message contains "did not converge".
	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunError {
			errEvt, ok := ev.(*events.RunErrorEvent)
			if !ok {
				t.Fatalf("got %T, want *events.RunErrorEvent", ev)
			}
			if !strings.Contains(errEvt.Message, "did not converge") {
				t.Errorf("error message = %q, want it to contain %q", errEvt.Message, "did not converge")
			}
		}
	}
}

// TestBridge_MaxIterationsCountsModelTurns proves the MaxIterations counter
// counts completed model turns (TurnComplete events), not all non-partial
// events. Bug B1: the counter incremented on every non-partial event, so a
// single model turn that calls a tool (function-call event + function-response
// event + final-text event) counted as 3 turns.
//
// The fake agent yields 3 non-partial events but only 1 has TurnComplete=true.
// With MaxIterations=1:
//   - Buggy code (counting all non-partial): 3 > 1 → RUN_ERROR (wrong)
//   - Fixed code (counting TurnComplete): 1 > 1 is false → no RUN_ERROR (correct)
//
// In real ADK streaming, the model response with a function call would also
// have TurnComplete=true (stream_aggregator.go sets it when FinishReason != "").
// This test uses synthetic events with TurnComplete=false on the function
// call/response to isolate the counting logic from real ADK behavior.
func TestBridge_MaxIterationsCountsModelTurns(t *testing.T) {
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Partial: false,
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "get_weather",
					Args: map[string]any{"city": "SF"},
				},
			}},
		},
	}

	ev2 := session.NewEvent(context.Background(), "inv-1")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Partial: false,
		Content: &genai.Content{
			Role: "user",
			Parts: []*genai.Part{{
				FunctionResponse: &genai.FunctionResponse{
					ID:       "fc-1",
					Name:     "get_weather",
					Response: map[string]any{"temp": 72},
				},
			}},
		},
	}

	ev3 := session.NewEvent(context.Background(), "inv-1")
	ev3.Author = "test-agent"
	ev3.LLMResponse = model.LLMResponse{
		Partial:      false,
		TurnComplete: true,
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "The weather is 72 degrees"}},
		},
	}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev1, nil) {
				return
			}
			if !yield(ev2, nil) {
				return
			}
			if !yield(ev3, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:         a,
		AppName:       "testapp",
		UserID:        "user1",
		MaxIterations: 1,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())
	typeSeq := eventTypes(collected)

	for _, et := range typeSeq {
		if et == events.EventTypeRunError {
			t.Fatalf("got %v, want no RUN_ERROR (1 model turn within MaxIterations=1)", typeSeq)
		}
	}

	hasRunFinished := false
	for _, et := range typeSeq {
		if et == events.EventTypeRunFinished {
			hasRunFinished = true
		}
	}
	if !hasRunFinished {
		t.Fatalf("got %v, want RUN_FINISHED", typeSeq)
	}
}

func TestBridge_PerRequestApproval(t *testing.T) {
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
		Agent:            a,
		AppName:          "testapp",
		UserID:           "user1",
		ApprovalModeFunc: func(r *http.Request) bool { return true },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Inject an HTTP request into context via WithHTTPRequest.
	input := defaultInput()
	ctx := aguiadk.WithHTTPRequest(context.Background(), &http.Request{})

	var collected []events.Event
	for ev, err := range bridgeAgent.Run(ctx, input) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ev != nil {
			collected = append(collected, ev)
		}
	}

	typeSeq := eventTypes(collected)

	// With auto-approve, the long-running tool should NOT trigger an interrupt.
	// The run should complete normally (RUN_FINISHED without interrupt outcome).
	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunFinished {
			finEvt, ok := ev.(*events.RunFinishedEvent)
			if !ok {
				t.Fatalf("got %T, want *events.RunFinishedEvent", ev)
			}
			if finEvt.Outcome != nil && finEvt.Outcome.Type == events.RunFinishedOutcomeTypeInterrupt {
				t.Errorf("got interrupt, want no interrupt outcome with auto-approve")
			}
		}
	}

	// Verify we got a normal RUN_FINISHED (not an interrupt).
	hasRunFinished := false
	for _, et := range typeSeq {
		if et == events.EventTypeRunFinished {
			hasRunFinished = true
		}
	}
	if !hasRunFinished {
		t.Fatal("got no RUN_FINISHED event, want one")
	}
}

func TestBridge_StatePersistenceAcrossRuns(t *testing.T) {
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "setting state"}},
		},
		Partial: false,
	}
	ev1.Actions.StateDelta = map[string]any{"foo": "bar"}

	ev2 := session.NewEvent(context.Background(), "inv-2")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "reading state"}},
		},
		Partial: false,
	}

	var callCount int32
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			n := atomic.AddInt32(&callCount, 1)
			if n == 1 {
				if !yield(ev1, nil) {
					return
				}
			} else {
				if !yield(ev2, nil) {
					return
				}
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

	// Run 1: emits StateDelta setting foo=bar.
	collectEvents(t, bridgeAgent, defaultInput())

	// Run 2: same thread, should see foo=bar in STATE_SNAPSHOT.
	input2 := defaultInput()
	input2.RunID = "run-2"
	collected2 := collectEvents(t, bridgeAgent, input2)

	var snapshotMap map[string]any
	for _, ev := range collected2 {
		if ev.Type() == events.EventTypeStateSnapshot {
			snapEvt, ok := ev.(*events.StateSnapshotEvent)
			if !ok {
				t.Fatalf("got %T, want *events.StateSnapshotEvent", ev)
			}
			snapshotMap = snapEvt.Snapshot.(map[string]any)
		}
	}
	if snapshotMap == nil {
		t.Fatal("got no STATE_SNAPSHOT event in run 2, want one")
	}
	fooVal, ok := snapshotMap["foo"]
	if !ok {
		t.Fatal("got no 'foo' key in state snapshot from run 2, want one")
	}
	if fooVal != "bar" {
		t.Errorf("state['foo'] = %v, want 'bar'", fooVal)
	}
}

func TestBridge_ConcurrentResume(t *testing.T) {
	interruptEv := session.NewEvent(context.Background(), "inv-1")
	interruptEv.Author = "test-agent"
	interruptEv.LLMResponse = model.LLMResponse{
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
	interruptEv.LongRunningToolIDs = []string{"fc-1"}

	resumeEv := session.NewEvent(context.Background(), "inv-2")
	resumeEv.Author = "test-agent"
	resumeEv.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "resumed"}},
		},
		Partial: false,
	}

	var callCount int32
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			n := atomic.AddInt32(&callCount, 1)
			if n == 1 {
				if !yield(interruptEv, nil) {
					return
				}
				return
			}
			if !yield(resumeEv, nil) {
				return
			}
		}
	})

	store := aguiadk.NewRunStore()
	t.Cleanup(store.Stop)

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:    a,
		AppName:  "testapp",
		UserID:   "user1",
		RunStore: store,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Phase 1: trigger interrupt.
	collectEvents(t, bridgeAgent, defaultInput())

	// Phase 2: resume twice concurrently.
	resumeInput := defaultInput()
	resumeInput.Resume = []types.ResumeEntry{
		{InterruptID: "fc-1", Status: types.ResumeStatusResolved, Payload: map[string]any{"approved": true}},
	}

	var wg sync.WaitGroup
	var errors []string
	var mu sync.Mutex
	collectWithError := func() {
		defer wg.Done()
		ctx := context.Background()
		for ev, err := range bridgeAgent.Run(ctx, resumeInput) {
			if err != nil {
				mu.Lock()
				errors = append(errors, err.Error())
				mu.Unlock()
				return
			}
			if ev != nil && ev.Type() == events.EventTypeRunError {
				if re, ok := ev.(*events.RunErrorEvent); ok {
					mu.Lock()
					errors = append(errors, re.Message)
					mu.Unlock()
				} else {
					mu.Lock()
					errors = append(errors, "run_error")
					mu.Unlock()
				}
			}
		}
	}

	wg.Add(2)
	go collectWithError()
	go collectWithError()
	wg.Wait()

	// At least one should have gotten a "claimed by a concurrent resume" error.
	hasConcurrentError := false
	for _, e := range errors {
		if strings.Contains(e, "concurrent resume") || strings.Contains(e, "no paused run found") {
			hasConcurrentError = true
		}
	}
	if !hasConcurrentError {
		t.Errorf("got %v, want at least one concurrent resume error", errors)
	}
}

func TestBridge_MultimodalAudio(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "received audio"}},
		},
		Partial: false,
	}

	var hasInlineData bool
	var partCount int
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			content := ctx.UserContent()
			if content != nil {
				for _, p := range content.Parts {
					partCount++
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

	// Small valid WAV base64 (minimal header + silence).
	wavBase64 := "UklGRiQAAABXQVZFZm10IBAAAAABAAEARKwAAIhYAQACABAAZGF0YQAAAAA="

	input := types.RunAgentInput{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Messages: []types.Message{
			{
				ID:   "msg-1",
				Role: types.RoleUser,
				Content: []types.InputContent{
					{Type: types.InputContentTypeText, Text: "transcribe this"},
					{
						Type: types.InputContentTypeAudio,
						Source: &types.InputContentSource{
							Type:     types.InputContentSourceTypeData,
							Value:    wavBase64,
							MimeType: "audio/wav",
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
	if !hasInlineData {
		t.Error("got no inline data part for audio content, want one")
	}
}

func TestBridge_MultimodalBinary(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "received binary"}},
		},
		Partial: false,
	}

	var hasInlineData bool
	var partCount int
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			content := ctx.UserContent()
			if content != nil {
				for _, p := range content.Parts {
					partCount++
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

	// Small base64 binary payload.
	binBase64 := "SGVsbG8gV29ybGQ="

	input := types.RunAgentInput{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Messages: []types.Message{
			{
				ID:   "msg-1",
				Role: types.RoleUser,
				Content: []types.InputContent{
					{Type: types.InputContentTypeText, Text: "process this"},
					{
						Type:     types.InputContentTypeBinary,
						Data:     binBase64,
						MimeType: "application/octet-stream",
					},
				},
			},
		},
	}

	collectEvents(t, bridgeAgent, input)

	if partCount != 2 {
		t.Errorf("partCount = %d, want 2", partCount)
	}
	if !hasInlineData {
		t.Error("got no inline data part for binary content, want one")
	}
}

// --- existing test helpers below ---

func TestBridge_MessagesSnapshotOnInterrupt(t *testing.T) {
	// Verifies that MESSAGES_SNAPSHOT is emitted before RUN_FINISHED on the
	// interrupt path when EmitMessagesSnapshot is enabled.
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "I need approval"}},
		},
		Partial: false,
	}
	fcEv := session.NewEvent(context.Background(), "inv-1")
	fcEv.Author = "test-agent"
	fcEv.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "approve",
					Args: map[string]any{"action": "delete"},
				},
			}},
		},
		Partial: false,
	}
	fcEv.LongRunningToolIDs = []string{"fc-1"}

	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev, nil) {
				return
			}
			if !yield(fcEv, nil) {
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

	// Find MESSAGES_SNAPSHOT and verify it appears before RUN_FINISHED.
	var msgSnapIdx, runFinIdx = -1, -1
	for i, ev := range collected {
		if ev.Type() == events.EventTypeMessagesSnapshot {
			msgSnapIdx = i
		}
		if ev.Type() == events.EventTypeRunFinished {
			runFinIdx = i
		}
	}
	if msgSnapIdx == -1 {
		t.Fatal("got no MESSAGES_SNAPSHOT event on interrupt path, want one")
	}
	if runFinIdx == -1 {
		t.Fatal("got no RUN_FINISHED event, want one")
	}
	if msgSnapIdx >= runFinIdx {
		t.Errorf("MESSAGES_SNAPSHOT (index %d) should come before RUN_FINISHED (index %d)", msgSnapIdx, runFinIdx)
	}

	// Verify RUN_FINISHED has interrupt outcome.
	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunFinished {
			finEvt, ok := ev.(*events.RunFinishedEvent)
			if !ok {
				t.Fatalf("got %T, want *events.RunFinishedEvent", ev)
			}
			if finEvt.Outcome == nil || finEvt.Outcome.Type != events.RunFinishedOutcomeTypeInterrupt {
				t.Error("got no interrupt outcome on RUN_FINISHED, want one")
			}
		}
	}
}

func TestBridge_StateStatusTransitions(t *testing.T) {
	// Verifies that STATE_DELTA events with "/status" are emitted at lifecycle
	// transitions when EmitStateStatus is enabled.
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Hello"}},
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
		Agent:           a,
		AppName:         "testapp",
		UserID:          "user1",
		EmitStateStatus: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	var statusDeltas []string
	for _, ev := range collected {
		if ev.Type() == events.EventTypeStateDelta {
			if sd, ok := ev.(*events.StateDeltaEvent); ok {
				for _, op := range sd.Delta {
					if op.Path == "/status" {
						if s, ok := op.Value.(string); ok {
							statusDeltas = append(statusDeltas, s)
						}
					}
				}
			}
		}
	}

	expected := []string{"running", "done"}
	if len(statusDeltas) != len(expected) {
		t.Fatalf("got %d status deltas, want %d: %v", len(statusDeltas), len(expected), statusDeltas)
	}
	for i, want := range expected {
		if statusDeltas[i] != want {
			t.Errorf("statusDelta[%d] = %q, want %q", i, statusDeltas[i], want)
		}
	}
}

func TestBridge_StateStatusOnInterrupt(t *testing.T) {
	// Verifies status transitions: running → awaiting_approval on interrupt.
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "approve",
					Args: map[string]any{"action": "delete"},
				},
			}},
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
		Agent:           a,
		AppName:         "testapp",
		UserID:          "user1",
		EmitStateStatus: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	var statusDeltas []string
	for _, ev := range collected {
		if ev.Type() == events.EventTypeStateDelta {
			if sd, ok := ev.(*events.StateDeltaEvent); ok {
				for _, op := range sd.Delta {
					if op.Path == "/status" {
						if s, ok := op.Value.(string); ok {
							statusDeltas = append(statusDeltas, s)
						}
					}
				}
			}
		}
	}

	expected := []string{"running", "awaiting_approval"}
	if len(statusDeltas) != len(expected) {
		t.Fatalf("got %d status deltas, want %d: %v", len(statusDeltas), len(expected), statusDeltas)
	}
	for i, want := range expected {
		if statusDeltas[i] != want {
			t.Errorf("statusDelta[%d] = %q, want %q", i, statusDeltas[i], want)
		}
	}
}

func TestBridge_StateStatusOnError(t *testing.T) {
	// Verifies status transitions: running → error on agent error.
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			yield(nil, fmt.Errorf("agent crashed"))
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:           a,
		AppName:         "testapp",
		UserID:          "user1",
		EmitStateStatus: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	var statusDeltas []string
	for _, ev := range collected {
		if ev.Type() == events.EventTypeStateDelta {
			if sd, ok := ev.(*events.StateDeltaEvent); ok {
				for _, op := range sd.Delta {
					if op.Path == "/status" {
						if s, ok := op.Value.(string); ok {
							statusDeltas = append(statusDeltas, s)
						}
					}
				}
			}
		}
	}

	expected := []string{"running", "error"}
	if len(statusDeltas) != len(expected) {
		t.Fatalf("got %d status deltas, want %d: %v", len(statusDeltas), len(expected), statusDeltas)
	}
	for i, want := range expected {
		if statusDeltas[i] != want {
			t.Errorf("statusDelta[%d] = %q, want %q", i, statusDeltas[i], want)
		}
	}
}

func TestBridge_ActivityDeltaDuringStreaming(t *testing.T) {
	// Verifies that ACTIVITY_SNAPSHOT and ACTIVITY_DELTA events are emitted
	// during streaming tool calls when EmitActivityDeltas is enabled.
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "search",
					PartialArgs: []*genai.PartialArg{
						{JsonPath: "$.q", StringValue: `"hel`},
					},
					WillContinue: new(true),
				},
			}},
		},
		Partial: true,
	}

	ev2 := session.NewEvent(context.Background(), "inv-1")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "search",
					PartialArgs: []*genai.PartialArg{
						{JsonPath: "$.q", StringValue: `lo"`},
					},
					WillContinue: new(false),
				},
			}},
		},
		Partial: true,
	}

	ev3 := session.NewEvent(context.Background(), "inv-1")
	ev3.Author = "test-agent"
	ev3.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "search",
					Args: map[string]any{"q": "hello"},
				},
			}},
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
			if !yield(ev3, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:              a,
		AppName:            "testapp",
		UserID:             "user1",
		EmitActivityDeltas: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	var activitySnapCount, activityDeltaCount int
	for _, ev := range collected {
		switch ev.Type() {
		case events.EventTypeActivitySnapshot:
			activitySnapCount++
		case events.EventTypeActivityDelta:
			activityDeltaCount++
		}
	}

	if activitySnapCount != 1 {
		t.Errorf("got %d ACTIVITY_SNAPSHOT events, want 1", activitySnapCount)
	}
	if activityDeltaCount != 2 {
		t.Errorf("got %d ACTIVITY_DELTA events, want 2 (one per PartialArg)", activityDeltaCount)
	}
}

func TestBridge_NoActivityDeltaWhenDisabled(t *testing.T) {
	// Verifies that ACTIVITY_DELTA events are NOT emitted when EmitActivityDeltas
	// is disabled (the default).
	ev1 := session.NewEvent(context.Background(), "inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "search",
					PartialArgs: []*genai.PartialArg{
						{JsonPath: "$.q", StringValue: `"hel`},
					},
					WillContinue: new(true),
				},
			}},
		},
		Partial: true,
	}

	ev2 := session.NewEvent(context.Background(), "inv-1")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "search",
					Args: map[string]any{"q": "hello"},
				},
			}},
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

	for _, ev := range collected {
		if ev.Type() == events.EventTypeActivityDelta {
			t.Error("did not expect ACTIVITY_DELTA when EmitActivityDeltas is disabled")
		}
		if ev.Type() == events.EventTypeActivitySnapshot {
			// ACTIVITY_SNAPSHOT is only emitted at FunctionResponse time, not at
			// TOOL_CALL_START time, when EmitActivityDeltas is disabled.
			t.Error("did not expect ACTIVITY_SNAPSHOT during streaming when EmitActivityDeltas is disabled")
		}
	}
}

func TestBridge_EmptyModelStream(t *testing.T) {
	// Verifies that a runner that yields zero events still produces
	// RUN_STARTED and RUN_FINISHED.
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
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
		events.EventTypeRunFinished,
	}
	assertEventSequence(t, typeSeq, expected)
}

func TestBridge_HandBackMode(t *testing.T) {
	// Verifies that ClientToolModeHandBack produces a plain RUN_FINISHED
	// (no interrupt outcome) with MESSAGES_SNAPSHOT when a long-running tool
	// is invoked.
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "client_tool",
					Args: map[string]any{"query": "test"},
				},
			}},
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
		Agent:                a,
		AppName:              "testapp",
		UserID:               "user1",
		EmitMessagesSnapshot: true,
		ClientTools:          &aguiadk.ClientToolConfig{Mode: aguiadk.ClientToolModeHandBack},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	collected := collectEvents(t, bridgeAgent, defaultInput())

	// Verify RUN_FINISHED has NO interrupt outcome.
	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunFinished {
			finEvt, ok := ev.(*events.RunFinishedEvent)
			if !ok {
				t.Fatalf("got %T, want *events.RunFinishedEvent", ev)
			}
			if finEvt.Outcome != nil {
				t.Errorf("got type %q, want nil Outcome on hand-back", finEvt.Outcome.Type)
			}
		}
	}

	// Verify MESSAGES_SNAPSHOT was emitted.
	hasMsgSnapshot := false
	for _, ev := range collected {
		if ev.Type() == events.EventTypeMessagesSnapshot {
			hasMsgSnapshot = true
		}
	}
	if !hasMsgSnapshot {
		t.Error("got no MESSAGES_SNAPSHOT on hand-back, want one")
	}
}

func TestBridge_MultimodalVideo(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "received video"}},
		},
		Partial: false,
	}

	var hasInlineData bool
	var partCount int
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			content := ctx.UserContent()
			if content != nil {
				for _, p := range content.Parts {
					partCount++
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

	// Small valid MP4 base64 (minimal header).
	mp4Base64 := "AAAAIGZ0eXBpc29tAAACAGlzb21pc28yYXZjMW1wNDE="

	input := types.RunAgentInput{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Messages: []types.Message{
			{
				ID:   "msg-1",
				Role: types.RoleUser,
				Content: []types.InputContent{
					{Type: types.InputContentTypeText, Text: "analyze this video"},
					{
						Type: types.InputContentTypeVideo,
						Source: &types.InputContentSource{
							Type:     types.InputContentSourceTypeData,
							Value:    mp4Base64,
							MimeType: "video/mp4",
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
	if !hasInlineData {
		t.Error("got no inline data part for video content, want one")
	}
}

func TestBridge_MultimodalDocument(t *testing.T) {
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "received document"}},
		},
		Partial: false,
	}

	var hasInlineData bool
	var partCount int
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			content := ctx.UserContent()
			if content != nil {
				for _, p := range content.Parts {
					partCount++
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

	// Small valid PDF base64 (minimal header).
	pdfBase64 := "JVBERi0xLjQKJdPr6eEKMSAwIG9iago8PC9UeXBlL0NhdGFsb2cvUGFnZXMgMiAwIFI+PmVuZG9iagoyIDAgb2JqCjw8L1R5cGUvUGFnZXMvS2lkc1szIDAgUl0vQ291bnQgMT4+ZW5kb2JqCjMgMCBvYmoKPDwvVHlwZS9QYWdlL01lZGlhQm94WzAgMCAzIDNdL1BhcmVudCAyIDAgUj4+ZW5kb2JqCnN0cmVhbQp4cmVmCjAgNApzdGFydHhyZWYKODIKJSVFT0YK"

	input := types.RunAgentInput{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Messages: []types.Message{
			{
				ID:   "msg-1",
				Role: types.RoleUser,
				Content: []types.InputContent{
					{Type: types.InputContentTypeText, Text: "read this document"},
					{
						Type: types.InputContentTypeDocument,
						Source: &types.InputContentSource{
							Type:     types.InputContentSourceTypeData,
							Value:    pdfBase64,
							MimeType: "application/pdf",
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
	if !hasInlineData {
		t.Error("got no inline data part for document content, want one")
	}
}

func TestBridge_MultimodalProviderGating(t *testing.T) {
	// Verifies that when Provider is set to a non-openai value, audio/video/
	// document content types fall back to text-only.
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "ok"}},
		},
		Partial: false,
	}

	var hasInlineData bool
	var partCount int
	var textParts int
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			content := ctx.UserContent()
			if content != nil {
				for _, p := range content.Parts {
					partCount++
					if p.InlineData != nil {
						hasInlineData = true
					}
					if p.Text != "" {
						textParts++
					}
				}
			}
			if !yield(ev, nil) {
				return
			}
		}
	})

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:    a,
		AppName:  "testapp",
		UserID:   "user1",
		Provider: "anthropic",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	wavBase64 := "UklGRiQAAABXQVZFZm10IBAAAAABAAEARKwAAIhYAQACABAAZGF0YQAAAAA="

	input := types.RunAgentInput{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Messages: []types.Message{
			{
				ID:   "msg-1",
				Role: types.RoleUser,
				Content: []types.InputContent{
					{Type: types.InputContentTypeText, Text: "transcribe this"},
					{
						Type: types.InputContentTypeAudio,
						Source: &types.InputContentSource{
							Type:     types.InputContentSourceTypeData,
							Value:    wavBase64,
							MimeType: "audio/wav",
						},
						Text: "[audio attachment]",
					},
				},
			},
		},
	}

	collectEvents(t, bridgeAgent, input)

	if hasInlineData {
		t.Error("got inline data part for audio with non-openai provider, want none")
	}
	if textParts != 2 {
		t.Errorf("got %d text parts, want 2 (user text + fallback text)", textParts)
	}
}

func TestBridge_MultimodalProviderOpenAI(t *testing.T) {
	// Verifies that when Provider is "openai", audio content is forwarded
	// as inline data (no gating).
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "ok"}},
		},
		Partial: false,
	}

	var hasInlineData bool
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			content := ctx.UserContent()
			if content != nil {
				for _, p := range content.Parts {
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
		Agent:    a,
		AppName:  "testapp",
		UserID:   "user1",
		Provider: "openai",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	wavBase64 := "UklGRiQAAABXQVZFZm10IBAAAAABAAEARKwAAIhYAQACABAAZGF0YQAAAAA="

	input := types.RunAgentInput{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Messages: []types.Message{
			{
				ID:   "msg-1",
				Role: types.RoleUser,
				Content: []types.InputContent{
					{Type: types.InputContentTypeText, Text: "transcribe this"},
					{
						Type: types.InputContentTypeAudio,
						Source: &types.InputContentSource{
							Type:     types.InputContentSourceTypeData,
							Value:    wavBase64,
							MimeType: "audio/wav",
						},
					},
				},
			},
		},
	}

	collectEvents(t, bridgeAgent, input)

	if !hasInlineData {
		t.Error("got no inline data part for audio with openai provider, want one")
	}
}

func TestBridge_HandBackPreset(t *testing.T) {
	cfg := aguiadk.HandBackPreset(aguiadk.Config{})
	if cfg.ClientTools == nil || cfg.ClientTools.Mode != aguiadk.ClientToolModeHandBack {
		t.Errorf("ClientToolMode = %v, want ClientToolModeHandBack", cfg.ClientTools.Mode)
	}
	if !cfg.EmitMessagesSnapshot {
		t.Errorf("EmitMessagesSnapshot = %v, want true", cfg.EmitMessagesSnapshot)
	}
	if cfg.EmitStateSnapshot == nil || !*cfg.EmitStateSnapshot {
		t.Errorf("EmitStateSnapshot = %v, want true", cfg.EmitStateSnapshot)
	}
}

func TestBridge_StopReleasesLazyRunStore(t *testing.T) {
	a := testutil.MustNewFakeAgent("test-agent")
	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   a,
		AppName: "testapp",
		UserID:  "user1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Stop should be safe to call even if no RunStore was created.
	aguiadk.Stop(bridgeAgent)

	// Stop on a non-bridge agent should be a no-op (no panic).
	aguiadk.Stop(nil)
}

func TestBridge_StopDoesNotReleaseProvidedRunStore(t *testing.T) {
	store := aguiadk.NewRunStore()
	t.Cleanup(store.Stop)
	a := testutil.MustNewFakeAgent("test-agent")
	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:    a,
		AppName:  "testapp",
		UserID:   "user1",
		RunStore: store,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Stop should NOT stop a caller-provided RunStore — caller owns it.
	aguiadk.Stop(bridgeAgent)

	// Verify the store is still usable (not stopped).
	key := aguiadk.RunKey("t", "r")
	store.Save(key, &aguiadk.PausedRun{ThreadID: "t", RunID: "r"})
	if _, ok := store.Load(key); !ok {
		t.Error("got error using caller-provided RunStore after Stop, want usable")
	}
}

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

// TestBridge_EarlyStreamTerminationReleasesGoroutine verifies that breaking
// out of the bridge.Run iterator promptly cancels the background runInternal
// goroutine instead of leaking it. The fake agent blocks until the
// run context is cancelled; if cancellation did not propagate, the goroutine
// would leak and the runExited channel would never close, hanging the test.
func TestBridge_EarlyStreamTerminationReleasesGoroutine(t *testing.T) {
	runExited := make(chan struct{})
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			defer close(runExited)
			// Block until the run context is cancelled. The bridge must
			// propagate consumer-side cancellation to this context.
			<-ctx.Done()
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

	ctx, cancel := context.WithCancel(context.Background())
	// Start iterating and break out after the first event (RUN_STARTED).
	seen := 0
	for ev, err := range bridgeAgent.Run(ctx, defaultInput()) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		seen++
		_ = ev
		if seen >= 1 {
			break // stop consuming — simulates client disconnect
		}
	}
	cancel()

	// The runInternal goroutine should observe the cancellation and exit.
	// The primary mechanism is the runExited channel (closed by the agent
	// goroutine via defer). The 30-second timeout is a safety net for
	// detecting a leaked goroutine, not the primary synchronization.
	select {
	case <-runExited:
		// success: goroutine exited promptly
	case <-time.After(30 * time.Second):
		t.Fatal("runInternal goroutine leaked: did not exit within 30s of consumer disconnect")
	}
}

// TestBridge_SubagentLifecycle verifies that events authored by a non-root
// sub-agent trigger SUBAGENT_STARTED and SUBAGENT_FINISHED lifecycle events,
// and that stream events emitted while the sub-agent is active carry
// subagentRunId in their JSON payload.
func TestBridge_SubagentLifecycle(t *testing.T) {
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			// Sub-agent "researcher" emits a text message.
			ev1 := session.NewEvent(context.Background(), "inv-1")
			ev1.Author = "researcher"
			ev1.LLMResponse = model.LLMResponse{
				Partial: true,
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "Research"}},
				},
			}
			if !yield(ev1, nil) {
				return
			}

			ev2 := session.NewEvent(context.Background(), "inv-1")
			ev2.Author = "researcher"
			ev2.LLMResponse = model.LLMResponse{
				Partial:      false,
				TurnComplete: true,
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "Research complete"}},
				},
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

	var subagentStarts, subagentFinishes int
	var textContentJSON []string
	for _, ev := range collected {
		switch ev.Type() {
		case "SUBAGENT_STARTED":
			subagentStarts++
		case "SUBAGENT_FINISHED":
			subagentFinishes++
		case events.EventTypeTextMessageContent:
			data, _ := ev.ToJSON()
			textContentJSON = append(textContentJSON, string(data))
		}
	}

	if subagentStarts != 1 {
		t.Errorf("SUBAGENT_STARTED count = %d, want 1", subagentStarts)
	}
	if subagentFinishes != 1 {
		t.Errorf("SUBAGENT_FINISHED count = %d, want 1", subagentFinishes)
	}

	// Verify that TEXT_MESSAGE_CONTENT events carry subagentRunId.
	if len(textContentJSON) == 0 {
		t.Fatal("got no TEXT_MESSAGE_CONTENT event, want at least one")
	}
	for _, j := range textContentJSON {
		if !strings.Contains(j, `"subagentRunId"`) {
			t.Errorf("TEXT_MESSAGE_CONTENT JSON missing subagentRunId: %s", j)
		}
	}
}

// TestBridge_SubagentClosedOnRunnerError verifies that a SUBAGENT_STARTED
// emitted by a sub-agent is balanced by a SUBAGENT_FINISHED before RUN_ERROR
// when the runner yields an error mid-turn. Without closeOpenSubagent() on the
// error path, the lifecycle is left unbalanced.
func TestBridge_SubagentClosedOnRunnerError(t *testing.T) {
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			// Sub-agent "researcher" starts streaming.
			ev1 := session.NewEvent(context.Background(), "inv-1")
			ev1.Author = "researcher"
			ev1.LLMResponse = model.LLMResponse{
				Partial: true,
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "Researching"}},
				},
			}
			if !yield(ev1, nil) {
				return
			}
			// Runner error while sub-agent is active.
			if !yield(nil, fmt.Errorf("runner crashed")) {
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

	ctx := context.Background()
	var collected []events.Event
	for ev, err := range bridgeAgent.Run(ctx, defaultInput()) {
		if err != nil {
			// The runner error ("runner crashed") is expected; the
			// SUBAGENT_FINISHED and RUN_ERROR events are emitted before it
			// as proper events, so continue collecting those.
			continue
		}
		if ev != nil {
			collected = append(collected, ev)
		}
	}

	typeSeq := eventTypes(collected)
	var subagentStarts, subagentFinishes int
	for _, et := range typeSeq {
		switch et {
		case "SUBAGENT_STARTED":
			subagentStarts++
		case "SUBAGENT_FINISHED":
			subagentFinishes++
		}
	}
	if subagentStarts != 1 {
		t.Fatalf("SUBAGENT_STARTED count = %d, want 1", subagentStarts)
	}
	if subagentFinishes != 1 {
		t.Fatalf("SUBAGENT_FINISHED count = %d, want 1 (lifecycle must be balanced before RUN_ERROR)", subagentFinishes)
	}
}

// TestBridge_TokenUsageOnRunnerError verifies that token usage collected from
// prior non-partial events is emitted on RUN_ERROR when the runner yields an
// error. Bug B4: the runner error path used RunErrorWithOptions, dropping
// collected usage. RunErrorWithUsage preserves it (and falls back to
// RunErrorWithOptions when no usage was collected).
func TestBridge_TokenUsageOnRunnerError(t *testing.T) {
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			// First event: a non-partial model response with usage metadata.
			ev := session.NewEvent(context.Background(), "inv-1")
			ev.Author = "test-agent"
			ev.LLMResponse = model.LLMResponse{
				Partial:      false,
				TurnComplete: true,
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "partial answer"}},
				},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 5,
					TotalTokenCount:      15,
				},
			}
			if !yield(ev, nil) {
				return
			}
			// Then a runner error.
			if !yield(nil, fmt.Errorf("model overloaded")) {
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

	ctx := context.Background()
	var collected []events.Event
	for ev, err := range bridgeAgent.Run(ctx, defaultInput()) {
		if err != nil {
			// The runner error is expected; continue collecting events
			// emitted before it (the usage-bearing RUN_ERROR).
			continue
		}
		if ev != nil {
			collected = append(collected, ev)
		}
	}

	// Find the RUN_ERROR event and verify it carries usage via a typed
	// assertion (matching TestBridge_TokenUsageReporting's style).
	var errorEv events.Event
	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunError {
			errorEv = ev
			break
		}
	}
	if errorEv == nil {
		t.Fatal("got no RUN_ERROR event, want one")
	}

	usageEv, ok := errorEv.(*agui.RunErrorWithUsageEvent)
	if !ok {
		t.Fatalf("got %T, want *agui.RunErrorWithUsageEvent", errorEv)
	}
	if len(usageEv.Usage) == 0 {
		t.Fatal("got empty usage on RUN_ERROR, want non-empty")
	}
	u := usageEv.Usage[0]
	if u.InputTokens == nil || *u.InputTokens != 10 {
		t.Errorf("usage inputTokens = %v, want 10", u.InputTokens)
	}
	if u.OutputTokens == nil || *u.OutputTokens != 5 {
		t.Errorf("usage outputTokens = %v, want 5", u.OutputTokens)
	}
	if u.TotalTokens == nil || *u.TotalTokens != 15 {
		t.Errorf("usage totalTokens = %v, want 15", u.TotalTokens)
	}
}

// TestBridge_TokenUsageReporting verifies that UsageMetadata from ADK events
// is collected and emitted on RUN_FINISHED via the usage field.
func TestBridge_TokenUsageReporting(t *testing.T) {
	a := testutil.MustNewFakeAgent("test-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			ev := session.NewEvent(context.Background(), "inv-1")
			ev.Author = "test-agent"
			ev.LLMResponse = model.LLMResponse{
				Partial: false,
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "Hello"}},
				},
				TurnComplete: true,
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 20,
					TotalTokenCount:      30,
				},
				ModelVersion: "gemini-2.5-flash",
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

	collected := collectEvents(t, bridgeAgent, defaultInput())

	// Find the RUN_FINISHED event and verify it carries usage.
	var finishedEv events.Event
	for _, ev := range collected {
		if ev.Type() == events.EventTypeRunFinished {
			finishedEv = ev
			break
		}
	}
	if finishedEv == nil {
		t.Fatal("got no RUN_FINISHED event, want one")
	}

	usageEv, ok := finishedEv.(*agui.RunFinishedWithUsageEvent)
	if !ok {
		t.Fatalf("got %T, want *agui.RunFinishedWithUsageEvent", finishedEv)
	}
	if len(usageEv.Usage) == 0 {
		t.Fatal("got empty usage on RUN_FINISHED, want non-empty")
	}
	u := usageEv.Usage[0]
	if u.Provider != "google" {
		t.Errorf("usage provider = %q, want google", u.Provider)
	}
	if u.Model != "gemini-2.5-flash" {
		t.Errorf("usage model = %q, want gemini-2.5-flash", u.Model)
	}
	if u.InputTokens == nil || *u.InputTokens != 10 {
		t.Errorf("usage inputTokens = %v, want 10", u.InputTokens)
	}
	if u.OutputTokens == nil || *u.OutputTokens != 20 {
		t.Errorf("usage outputTokens = %v, want 20", u.OutputTokens)
	}
	if u.TotalTokens == nil || *u.TotalTokens != 30 {
		t.Errorf("usage totalTokens = %v, want 30", u.TotalTokens)
	}
}
