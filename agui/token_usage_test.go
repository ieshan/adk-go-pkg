package agui_test

import (
	"encoding/json"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ieshan/adk-go-pkg/agui"
)

func TestTokenUsage_Aggregation(t *testing.T) {
	in1, out1, tot1 := int64(10), int64(20), int64(30)
	in2, out2, tot2 := int64(5), int64(15), int64(20)

	u1 := events.TokenUsage{Provider: "google", Model: "gemini-2.5-flash", InputTokens: &in1, OutputTokens: &out1, TotalTokens: &tot1}
	u2 := events.TokenUsage{Provider: "google", Model: "gemini-2.5-flash", InputTokens: &in2, OutputTokens: &out2, TotalTokens: &tot2}

	merged := agui.AggregateTokenUsage([]events.TokenUsage{u1, u2})
	if len(merged) != 1 {
		t.Fatalf("got %d aggregated entries, want 1", len(merged))
	}
	if *merged[0].TotalTokens != 50 || *merged[0].InputTokens != 15 || *merged[0].OutputTokens != 35 {
		t.Errorf("aggregated counts mismatch: %+v", merged[0])
	}
}

func TestTokenUsage_Aggregation_DistinctKeys(t *testing.T) {
	in1, out1, tot1 := int64(10), int64(20), int64(30)
	in2, out2, tot2 := int64(5), int64(15), int64(20)

	entries := []events.TokenUsage{
		{Provider: "google", Model: "gemini-2.5-flash", InputTokens: &in1, OutputTokens: &out1, TotalTokens: &tot1},
		{Provider: "openai", Model: "gpt-4o", InputTokens: &in2, OutputTokens: &out2, TotalTokens: &tot2},
	}
	merged := agui.AggregateTokenUsage(entries)
	if len(merged) != 2 {
		t.Fatalf("got %d aggregated entries, want 2", len(merged))
	}
}

func TestTokenUsage_Aggregation_NilCounts(t *testing.T) {
	entries := []events.TokenUsage{
		{Provider: "google", Model: "gemini-2.5-flash"},
		{Provider: "google", Model: "gemini-2.5-flash", InputTokens: new(int64(7))},
	}
	merged := agui.AggregateTokenUsage(entries)
	if len(merged) != 1 {
		t.Fatalf("got %d aggregated entries, want 1", len(merged))
	}
	if merged[0].InputTokens == nil || *merged[0].InputTokens != 7 {
		t.Fatalf("got %+v, want InputTokens=7", merged[0].InputTokens)
	}
	if merged[0].OutputTokens != nil {
		t.Fatalf("got %v, want OutputTokens nil", *merged[0].OutputTokens)
	}
}

// TestEventEmitter_RunFinishedWithUsage verifies that RunFinishedWithUsage
// emits a canonical *events.RunFinishedEvent with the Usage field populated
// when usage is non-empty, and falls back to a plain RunFinishedEvent (no
// usage field) when usage is empty.
func TestEventEmitter_RunFinishedWithUsage(t *testing.T) {
	t.Run("with usage populates Usage field", func(t *testing.T) {
		ch := make(chan events.Event, 1)
		emitter := agui.NewEventEmitter(ch)
		in, out, tot := int64(10), int64(20), int64(30)
		usage := []events.TokenUsage{{
			Provider:     "google",
			Model:        "gemini-2.5-flash",
			InputTokens:  &in,
			OutputTokens: &out,
			TotalTokens:  &tot,
		}}
		if err := emitter.RunFinishedWithUsage("thread-1", "run-1", usage); err != nil {
			t.Fatalf("RunFinishedWithUsage: %v", err)
		}
		close(ch)

		ev := <-ch
		finishedEv, ok := ev.(*events.RunFinishedEvent)
		if !ok {
			t.Fatalf("got %T, want *events.RunFinishedEvent", ev)
		}
		if len(finishedEv.Usage) != 1 {
			t.Fatalf("Usage len = %d, want 1", len(finishedEv.Usage))
		}
		u := finishedEv.Usage[0]
		if u.Provider != "google" || u.Model != "gemini-2.5-flash" {
			t.Errorf("usage[0] = %+v", u)
		}
		if u.TotalTokens == nil || *u.TotalTokens != 30 {
			t.Errorf("totalTokens = %v, want 30", u.TotalTokens)
		}
	})

	t.Run("empty usage falls back to plain event", func(t *testing.T) {
		ch := make(chan events.Event, 1)
		emitter := agui.NewEventEmitter(ch)
		if err := emitter.RunFinishedWithUsage("thread-1", "run-1", nil); err != nil {
			t.Fatalf("RunFinishedWithUsage: %v", err)
		}
		close(ch)

		ev := <-ch
		finishedEv, ok := ev.(*events.RunFinishedEvent)
		if !ok {
			t.Fatalf("got %T, want *events.RunFinishedEvent", ev)
		}
		if len(finishedEv.Usage) != 0 {
			t.Errorf("Usage len = %d, want 0 for empty usage", len(finishedEv.Usage))
		}
		// Wire format must omit the usage field entirely.
		data, _ := json.Marshal(finishedEv)
		if string(data) == "" {
			t.Fatal("empty json")
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, has := m["usage"]; has {
			t.Errorf("usage field present, want absent for empty usage: %s", string(data))
		}
	})
}

// TestEventEmitter_RunErrorWithUsage verifies that RunErrorWithUsage emits a
// canonical *events.RunErrorEvent with the Usage field populated when usage
// is non-empty, and falls back to a plain RunErrorEvent when usage is empty.
func TestEventEmitter_RunErrorWithUsage(t *testing.T) {
	t.Run("with usage populates Usage field", func(t *testing.T) {
		ch := make(chan events.Event, 1)
		emitter := agui.NewEventEmitter(ch)
		tot := int64(5)
		usage := []events.TokenUsage{{
			Provider:    "openai",
			Model:       "gpt-4o",
			TotalTokens: &tot,
		}}
		if err := emitter.RunErrorWithUsage("boom", usage); err != nil {
			t.Fatalf("RunErrorWithUsage: %v", err)
		}
		close(ch)

		ev := <-ch
		errorEv, ok := ev.(*events.RunErrorEvent)
		if !ok {
			t.Fatalf("got %T, want *events.RunErrorEvent", ev)
		}
		if errorEv.Message != "boom" {
			t.Errorf("message = %q, want boom", errorEv.Message)
		}
		if len(errorEv.Usage) != 1 {
			t.Fatalf("Usage len = %d, want 1", len(errorEv.Usage))
		}
		if errorEv.Usage[0].Provider != "openai" {
			t.Errorf("usage[0].Provider = %q, want openai", errorEv.Usage[0].Provider)
		}
	})

	t.Run("empty usage falls back to plain event", func(t *testing.T) {
		ch := make(chan events.Event, 1)
		emitter := agui.NewEventEmitter(ch)
		if err := emitter.RunErrorWithUsage("boom", nil); err != nil {
			t.Fatalf("RunErrorWithUsage: %v", err)
		}
		close(ch)

		ev := <-ch
		errorEv, ok := ev.(*events.RunErrorEvent)
		if !ok {
			t.Fatalf("got %T, want *events.RunErrorEvent", ev)
		}
		if len(errorEv.Usage) != 0 {
			t.Errorf("Usage len = %d, want 0 for empty usage", len(errorEv.Usage))
		}
	})
}
