package aguiadk_test

import (
	"context"
	"iter"
	"net/http"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"github.com/ieshan/adk-go-pkg/aguiadk"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// makeAgent creates a simple ADK agent that yields the given events.
func makeAgent(name string, events []*session.Event) agent.Agent {
	a, err := agent.New(agent.Config{
		Name:        name,
		Description: "test agent",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				for _, ev := range events {
					if !yield(ev, nil) {
						return
					}
				}
			}
		},
	})
	if err != nil {
		panic(err)
	}
	return a
}

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
		a := makeAgent("test", nil)
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
		a := makeAgent("test", nil)
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
		a := makeAgent("test", nil)
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
	ev := session.NewEvent("inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Hello, world!"}},
		},
		Partial: false,
	}

	a := makeAgent("test-agent", []*session.Event{ev})

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
	ev1 := session.NewEvent("inv-1")
	ev1.Author = "test-agent"
	ev1.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Hello"}},
		},
		Partial: true,
	}

	ev2 := session.NewEvent("inv-1")
	ev2.Author = "test-agent"
	ev2.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Hello, world!"}},
		},
		Partial: false,
	}

	a := makeAgent("test-agent", []*session.Event{ev1, ev2})

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
	ev := session.NewEvent("inv-1")
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

	a := makeAgent("test-agent", []*session.Event{ev})

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

func TestBridge_ThoughtParts(t *testing.T) {
	ev := session.NewEvent("inv-1")
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

	a := makeAgent("test-agent", []*session.Event{ev})

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
		a := makeAgent("test-agent", nil)
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
		a := makeAgent("test-agent", nil)
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
	ev := session.NewEvent("inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "done"}},
		},
		Partial: false,
	}
	ev.Actions.StateDelta["key1"] = "value1"

	a := makeAgent("test-agent", []*session.Event{ev})

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
	ev := session.NewEvent("inv-1")
	ev.Author = "test-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Hi"}},
		},
		Partial: false,
	}

	a := makeAgent("test-agent", []*session.Event{ev})

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
	ev := session.NewEvent("inv-1")
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

	a := makeAgent("test-agent", []*session.Event{ev})

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
	a := makeAgent("test-agent", nil)

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
