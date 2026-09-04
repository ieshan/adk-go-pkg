package agui

import (
	"encoding/json"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

func TestTokenUsage_Aggregation(t *testing.T) {
	in1, out1, tot1 := int64(10), int64(20), int64(30)
	in2, out2, tot2 := int64(5), int64(15), int64(20)

	u1 := TokenUsage{Provider: "google", Model: "gemini-2.5-flash", InputTokens: &in1, OutputTokens: &out1, TotalTokens: &tot1}
	u2 := TokenUsage{Provider: "google", Model: "gemini-2.5-flash", InputTokens: &in2, OutputTokens: &out2, TotalTokens: &tot2}

	merged := AggregateTokenUsage([]TokenUsage{u1, u2})
	if len(merged) != 1 {
		t.Fatalf("expected 1 aggregated entry, got %d", len(merged))
	}
	if *merged[0].TotalTokens != 50 || *merged[0].InputTokens != 15 || *merged[0].OutputTokens != 35 {
		t.Fatalf("aggregated counts mismatch: %+v", merged[0])
	}
}

func TestTokenUsage_Aggregation_DistinctKeys(t *testing.T) {
	in1, out1, tot1 := int64(10), int64(20), int64(30)
	in2, out2, tot2 := int64(5), int64(15), int64(20)

	entries := []TokenUsage{
		{Provider: "google", Model: "gemini-2.5-flash", InputTokens: &in1, OutputTokens: &out1, TotalTokens: &tot1},
		{Provider: "openai", Model: "gpt-4o", InputTokens: &in2, OutputTokens: &out2, TotalTokens: &tot2},
	}
	merged := AggregateTokenUsage(entries)
	if len(merged) != 2 {
		t.Fatalf("expected 2 aggregated entries, got %d", len(merged))
	}
}

func TestTokenUsage_Aggregation_NilCounts(t *testing.T) {
	entries := []TokenUsage{
		{Provider: "google", Model: "gemini-2.5-flash"},
		{Provider: "google", Model: "gemini-2.5-flash", InputTokens: new(int64(7))},
	}
	merged := AggregateTokenUsage(entries)
	if len(merged) != 1 {
		t.Fatalf("expected 1 aggregated entry, got %d", len(merged))
	}
	if merged[0].InputTokens == nil || *merged[0].InputTokens != 7 {
		t.Fatalf("expected InputTokens=7, got %+v", merged[0].InputTokens)
	}
	if merged[0].OutputTokens != nil {
		t.Fatalf("expected OutputTokens nil, got %v", *merged[0].OutputTokens)
	}
}

func TestRunFinishedWithUsageEvent_Serialization(t *testing.T) {
	in, out, tot := int64(10), int64(20), int64(30)
	ev := &RunFinishedWithUsageEvent{
		RunFinishedEvent: nil, // will be set below
		Usage: []TokenUsage{{
			Provider:     "google",
			Model:        "gemini-2.5-flash",
			InputTokens:  &in,
			OutputTokens: &out,
			TotalTokens:  &tot,
		}},
	}
	// Use a real base event so JSON has the required type/threadId/runId.
	ev.RunFinishedEvent = events.NewRunFinishedEvent("thread-1", "run-1")
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["type"] != "RUN_FINISHED" {
		t.Errorf("type = %v, want RUN_FINISHED", m["type"])
	}
	usage, ok := m["usage"].([]any)
	if !ok || len(usage) != 1 {
		t.Fatalf("usage = %+v, want 1 entry", m["usage"])
	}
	first := usage[0].(map[string]any)
	if first["provider"] != "google" || first["model"] != "gemini-2.5-flash" {
		t.Errorf("usage[0] = %+v", first)
	}
	if first["totalTokens"].(float64) != 30 {
		t.Errorf("totalTokens = %v, want 30", first["totalTokens"])
	}
}

func TestRunErrorWithUsageEvent_Serialization(t *testing.T) {
	tot := int64(5)
	ev := &RunErrorWithUsageEvent{
		RunErrorEvent: events.NewRunErrorEvent("boom"),
		Usage: []TokenUsage{{
			Provider:    "openai",
			Model:       "gpt-4o",
			TotalTokens: &tot,
		}},
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["type"] != "RUN_ERROR" {
		t.Errorf("type = %v, want RUN_ERROR", m["type"])
	}
	if m["message"] != "boom" {
		t.Errorf("message = %v, want boom", m["message"])
	}
	if _, ok := m["usage"]; !ok {
		t.Error("expected usage field present")
	}
}
