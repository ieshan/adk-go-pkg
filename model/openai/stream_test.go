package openai

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/adk/model"
)

// streamResult captures both values yielded by the iter.Seq2 returned from
// parseStream so tests can collect the full sequence before making assertions.
type streamResult struct {
	resp *model.LLMResponse
	err  error
}

// collectStream drives the iterator returned by parseStream to completion and
// returns all yielded pairs as a slice.
func collectStream(ctx context.Context, body *strings.Reader) []streamResult {
	var results []streamResult
	for resp, err := range parseStream(ctx, body) {
		results = append(results, streamResult{resp: resp, err: err})
	}
	return results
}

// TestParseStream_SimpleText verifies that text-content deltas across multiple
// SSE chunks are yielded as partial LLMResponse values, and that the final
// data: [DONE] sentinel produces a response with TurnComplete: true.
func TestParseStream_SimpleText(t *testing.T) {
	body := strings.NewReader(strings.Join([]string{
		`data: {"id":"1","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}`,
		``,
		`data: {"id":"1","choices":[{"index":0,"delta":{"content":" world"}}]}`,
		``,
		`data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n"))

	results := collectStream(context.Background(), body)

	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	for i, r := range results {
		if r.err != nil {
			t.Errorf("result[%d]: unexpected error: %v", i, r.err)
		}
	}

	// Verify at least one result carries text content and has Partial=true.
	textPartialCount := 0
	for _, r := range results {
		if r.resp == nil || r.resp.Content == nil {
			continue
		}
		for _, p := range r.resp.Content.Parts {
			if p.Text != "" {
				if !r.resp.Partial {
					t.Errorf("text-bearing chunk should have Partial=true")
				}
				textPartialCount++
			}
		}
	}
	if textPartialCount == 0 {
		t.Error("expected at least one chunk with text content and Partial=true")
	}

	// The last result must have TurnComplete=true.
	last := results[len(results)-1]
	if last.err != nil {
		t.Fatalf("last result has unexpected error: %v", last.err)
	}
	if last.resp == nil || !last.resp.TurnComplete {
		t.Errorf("last result: expected TurnComplete=true, got %+v", last.resp)
	}
}

// TestParseStream_ToolCalls verifies that tool_call deltas sent across multiple
// chunks are accumulated by index and the DONE sentinel causes a response
// containing FunctionCall Parts with fully assembled arguments.
func TestParseStream_ToolCalls(t *testing.T) {
	body := strings.NewReader(strings.Join([]string{
		`data: {"id":"1","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":""}}]}}]}`,
		``,
		`data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":"}}]}}]}`,
		``,
		`data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"test\"}"}}]}}]}`,
		``,
		`data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n"))

	results := collectStream(context.Background(), body)

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	for i, r := range results {
		if r.err != nil {
			t.Errorf("result[%d]: unexpected error: %v", i, r.err)
		}
	}

	// The last result must be TurnComplete.
	last := results[len(results)-1]
	if last.resp == nil || !last.resp.TurnComplete {
		t.Errorf("last result: expected TurnComplete=true, got %+v", last.resp)
	}

	// Scan all results for a FunctionCall Part with the assembled arguments.
	var foundFC bool
	for _, r := range results {
		if r.resp == nil || r.resp.Content == nil {
			continue
		}
		for _, p := range r.resp.Content.Parts {
			if p.FunctionCall == nil {
				continue
			}
			foundFC = true
			fc := p.FunctionCall
			if fc.Name != "search" {
				t.Errorf("FunctionCall.Name: got %q, want %q", fc.Name, "search")
			}
			if fc.ID != "call_1" {
				t.Errorf("FunctionCall.ID: got %q, want %q", fc.ID, "call_1")
			}
			q, ok := fc.Args["q"]
			if !ok {
				t.Errorf("FunctionCall.Args missing key %q; full args=%v", "q", fc.Args)
			} else if q != "test" {
				t.Errorf("FunctionCall.Args[q]: got %v, want %q", q, "test")
			}
		}
	}
	if !foundFC {
		t.Error("expected at least one FunctionCall Part across all results")
	}
}

// TestParseStream_Done verifies that a stream containing only data: [DONE]
// yields exactly one result with TurnComplete: true and no error.
func TestParseStream_Done(t *testing.T) {
	body := strings.NewReader("data: [DONE]\n")

	results := collectStream(context.Background(), body)

	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result, got %d", len(results))
	}
	r := results[0]
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if r.resp == nil || !r.resp.TurnComplete {
		t.Errorf("expected TurnComplete=true, got %+v", r.resp)
	}
}

// TestParseStream_EmptyLines verifies that blank lines interspersed between SSE
// data lines are silently skipped and do not produce spurious results.
func TestParseStream_EmptyLines(t *testing.T) {
	body := strings.NewReader(strings.Join([]string{
		``,
		``,
		`data: {"id":"1","choices":[{"index":0,"delta":{"content":"hi"}}]}`,
		``,
		``,
		`data: [DONE]`,
		``,
	}, "\n"))

	results := collectStream(context.Background(), body)

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	for i, r := range results {
		if r.err != nil {
			t.Errorf("result[%d]: unexpected error: %v", i, r.err)
		}
	}

	last := results[len(results)-1]
	if last.resp == nil || !last.resp.TurnComplete {
		t.Errorf("last result: expected TurnComplete=true")
	}
}

// TestParseStream_ContextCancellation verifies that when the context is
// cancelled during iteration the iterator yields a result with Interrupted: true.
func TestParseStream_ContextCancellation(t *testing.T) {
	body := strings.NewReader(strings.Join([]string{
		`data: {"id":"1","choices":[{"index":0,"delta":{"content":"a"}}]}`,
		`data: {"id":"1","choices":[{"index":0,"delta":{"content":"b"}}]}`,
		`data: {"id":"1","choices":[{"index":0,"delta":{"content":"c"}}]}`,
		`data: [DONE]`,
		``,
	}, "\n"))

	ctx, cancel := context.WithCancel(context.Background())

	var results []streamResult
	for resp, err := range parseStream(ctx, body) {
		results = append(results, streamResult{resp: resp, err: err})
		// Cancel after receiving the first yield so the next iteration detects it.
		cancel()
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result before cancellation")
	}

	foundInterrupted := false
	for _, r := range results {
		if r.resp != nil && r.resp.Interrupted {
			foundInterrupted = true
			break
		}
	}
	if !foundInterrupted {
		t.Errorf("expected an Interrupted=true result after context cancellation; got %d result(s)", len(results))
	}
}

// TestParseStream_MalformedJSON verifies that a data line with invalid JSON
// causes the iterator to yield a non-nil error.
func TestParseStream_MalformedJSON(t *testing.T) {
	body := strings.NewReader("data: {invalid\n")

	results := collectStream(context.Background(), body)

	if len(results) == 0 {
		t.Fatal("expected at least one result containing an error")
	}

	foundErr := false
	for _, r := range results {
		if r.err != nil {
			foundErr = true
			break
		}
	}
	if !foundErr {
		t.Errorf("expected an error result for malformed JSON, but none was yielded")
	}
}
