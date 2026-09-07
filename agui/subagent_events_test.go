package agui_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ieshan/adk-go-pkg/agui"
)

func TestSubagentLifecycleEvents_Serialization(t *testing.T) {
	start := agui.NewSubagentStartedEvent("sub_run_1", "researcher", agui.WithSubagentDescription("Researches topics"))
	data, err := json.Marshal(start)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["type"] != "SUBAGENT_STARTED" || m["subagentRunId"] != "sub_run_1" || m["name"] != "researcher" {
		t.Fatalf("unexpected json: %s", string(data))
	}
	if m["description"] != "Researches topics" {
		t.Errorf("description = %v, want 'Researches topics'", m["description"])
	}
}

func TestSubagentFinishedEvent_Serialization(t *testing.T) {
	ev := agui.NewSubagentFinishedEvent("sub_run_1", agui.WithSubagentName("researcher"), agui.WithSubagentSuccessOutcome())
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["type"] != "SUBAGENT_FINISHED" || m["subagentRunId"] != "sub_run_1" {
		t.Fatalf("unexpected json: %s", string(data))
	}
	outcome := m["outcome"].(map[string]any)
	if outcome["type"] != "success" {
		t.Errorf("outcome.type = %v, want success", outcome["type"])
	}
}

func TestSubagentErrorEvent_Serialization(t *testing.T) {
	ev := agui.NewSubagentErrorEvent("sub_run_1", "tool failed", agui.WithSubagentErrorName("researcher"), agui.WithSubagentErrorCode("TOOL_ERROR"))
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["type"] != "SUBAGENT_ERROR" || m["subagentRunId"] != "sub_run_1" || m["message"] != "tool failed" {
		t.Fatalf("unexpected json: %s", string(data))
	}
	if m["name"] != "researcher" {
		t.Errorf("name = %v, want researcher", m["name"])
	}
	if m["code"] != "TOOL_ERROR" {
		t.Errorf("code = %v, want TOOL_ERROR", m["code"])
	}
}

func TestSubagentEvents_Validate(t *testing.T) {
	if err := agui.NewSubagentStartedEvent("", "x").Validate(); err == nil {
		t.Error("got nil error, want error for empty subagentRunId")
	}
	if err := agui.NewSubagentStartedEvent("id", "").Validate(); err == nil {
		t.Error("got nil error, want error for empty name")
	}
	if err := agui.NewSubagentFinishedEvent("").Validate(); err == nil {
		t.Error("got nil error, want error for empty subagentRunId")
	}
	if err := agui.NewSubagentErrorEvent("", "msg").Validate(); err == nil {
		t.Error("got nil error, want error for empty subagentRunId")
	}
	if err := agui.NewSubagentErrorEvent("id", "").Validate(); err == nil {
		t.Error("got nil error, want error for empty message")
	}
}

func TestSubagentFinishedEvent_WithResult(t *testing.T) {
	ev := agui.NewSubagentFinishedEvent("sub_1",
		agui.WithSubagentName("researcher"),
		agui.WithSubagentResult("task completed"),
		agui.WithSubagentSuccessOutcome(),
	)
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["result"] != "task completed" {
		t.Errorf("result = %v, want 'task completed'", m["result"])
	}
}

func TestForSubagent_AttributedEvents(t *testing.T) {
	ch := make(chan events.Event, 4)
	base := agui.NewEventEmitter(ch)
	sub := base.ForSubagent("sub_run_1")

	// Emit through the sub-agent wrapper.
	if err := sub.TextMessageStart("m1", new("assistant")); err != nil {
		t.Fatalf("TextMessageStart: %v", err)
	}
	if err := sub.TextMessageContent("m1", "hello"); err != nil {
		t.Fatalf("TextMessageContent: %v", err)
	}
	close(ch)

	var contentJSON string
	for ev := range ch {
		if ev.Type() == events.EventTypeTextMessageContent {
			data, _ := ev.ToJSON()
			contentJSON = string(data)
		}
	}
	if contentJSON == "" {
		t.Fatal("got no TEXT_MESSAGE_CONTENT event, want one")
	}
	if !strings.Contains(contentJSON, `"subagentRunId":"sub_run_1"`) {
		t.Errorf("TEXT_MESSAGE_CONTENT JSON missing subagentRunId: %s", contentJSON)
	}
}

func TestForSubagent_BaseEmitterUntouched(t *testing.T) {
	ch := make(chan events.Event, 2)
	base := agui.NewEventEmitter(ch)
	sub := base.ForSubagent("sub_run_1")

	// Emit through the base emitter — should NOT carry subagentRunId.
	if err := base.TextMessageContent("m1", "hello"); err != nil {
		t.Fatalf("base TextMessageContent: %v", err)
	}
	// Emit through sub — should carry subagentRunId.
	if err := sub.TextMessageContent("m2", "world"); err != nil {
		t.Fatalf("sub TextMessageContent: %v", err)
	}
	close(ch)

	var baseJSON, subJSON string
	for ev := range ch {
		data, _ := ev.ToJSON()
		switch ev.Type() {
		case events.EventTypeTextMessageContent:
			s := string(data)
			if strings.Contains(s, `"m1"`) {
				baseJSON = s
			}
			if strings.Contains(s, `"m2"`) {
				subJSON = s
			}
		}
	}
	if strings.Contains(baseJSON, `"subagentRunId"`) {
		t.Errorf("base emitter event should not carry subagentRunId: %s", baseJSON)
	}
	if !strings.Contains(subJSON, `"subagentRunId":"sub_run_1"`) {
		t.Errorf("sub emitter event should carry subagentRunId: %s", subJSON)
	}
}
