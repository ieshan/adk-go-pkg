package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// responsesStreamResult captures both values yielded by the iter.Seq2 returned
// from parseResponsesStream so tests can collect the full sequence.
type responsesStreamResult struct {
	resp *model.LLMResponse
	err  error
}

// collectResponsesStream drives the iterator returned by parseResponsesStream
// to completion and returns all yielded pairs as a slice.
func collectResponsesStream(ctx context.Context, body *strings.Reader) []responsesStreamResult {
	var results []responsesStreamResult
	for resp, err := range parseResponsesStream(ctx, body) {
		results = append(results, responsesStreamResult{resp: resp, err: err})
	}
	return results
}

// sseLines joins lines into an SSE body with blank-line separators.
func sseLines(lines ...string) string {
	var out []string
	for _, l := range lines {
		out = append(out, l, "")
	}
	return strings.Join(out, "\n")
}

func TestParseResponsesStream_TextDelta(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		`data: {"type":"response.output_text.delta","delta":" world"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello world"}]}]}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	// Should have 2 text partials + 1 final.
	if len(results) < 2 {
		t.Fatalf("got %d results, want at least 2", len(results))
	}

	// Verify text partials.
	textCount := 0
	for _, r := range results[:len(results)-1] {
		if r.err != nil {
			t.Errorf("unexpected error: %v", r.err)
		}
		if r.resp != nil && r.resp.Partial && r.resp.Content != nil && len(r.resp.Content.Parts) > 0 {
			if r.resp.Content.Parts[0].Text != "" {
				textCount++
			}
		}
	}
	if textCount != 2 {
		t.Errorf("got %d text partials, want 2", textCount)
	}

	// Verify final response.
	if len(results) == 0 {
		t.Fatal("got no results, want at least one")
	}
	last := results[len(results)-1]
	if last.err != nil {
		t.Fatalf("last error: %v", last.err)
	}
	if last.resp == nil || !last.resp.TurnComplete {
		t.Errorf("last: got %+v, want TurnComplete=true", last.resp)
	}
}

func TestParseResponsesStream_ReasoningDelta(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.reasoning_text.delta","delta":"Thinking..."}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Answer"}]}]}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	// First result should be a reasoning partial with Thought=true.
	if len(results) < 2 {
		t.Fatalf("got %d results, want at least 2", len(results))
	}
	r := results[0]
	if r.err != nil {
		t.Errorf("unexpected error: %v", r.err)
	}
	if r.resp == nil || !r.resp.Partial {
		t.Errorf("first result: got %+v, want Partial=true", r.resp)
	}
	if r.resp.Content == nil || len(r.resp.Content.Parts) == 0 {
		t.Fatal("no content parts")
	}
	if r.resp.Content.Parts[0].Text != "Thinking..." {
		t.Errorf("text: got %q, want %q", r.resp.Content.Parts[0].Text, "Thinking...")
	}
	if !r.resp.Content.Parts[0].Thought {
		t.Errorf("thought: got false, want true")
	}
}

func TestParseResponsesStream_ReasoningSummaryDelta(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.reasoning_summary_text.delta","delta":"Summary..."}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Answer"}]}]}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	if len(results) < 2 {
		t.Fatalf("got %d results, want at least 2", len(results))
	}
	r := results[0]
	if r.resp == nil || r.resp.Content == nil || len(r.resp.Content.Parts) == 0 {
		t.Fatal("no content")
	}
	if !r.resp.Content.Parts[0].Thought {
		t.Errorf("thought: got false, want true")
	}
}

func TestParseResponsesStream_FunctionCallArgs(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.output_item.added","item_id":"item_1","item":{"type":"function_call","call_id":"call_abc","name":"get_weather"}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"{\"loc"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"ation\":\"NYC\"}"}`,
		`data: {"type":"response.function_call_arguments.done","item_id":"item_1","arguments":"{\"location\":\"NYC\"}"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"function_call","call_id":"call_abc","name":"get_weather","arguments":"{\"location\":\"NYC\"}"}]}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	// Find the FunctionCall partial.
	var foundFC bool
	for _, r := range results {
		if r.err != nil {
			t.Errorf("unexpected error: %v", r.err)
		}
		if r.resp == nil || r.resp.Content == nil {
			continue
		}
		for _, p := range r.resp.Content.Parts {
			if p.FunctionCall != nil && r.resp.Partial {
				foundFC = true
				fc := p.FunctionCall
				if fc.ID != "call_abc" {
					t.Errorf("ID: got %q, want %q", fc.ID, "call_abc")
				}
				if fc.Name != "get_weather" {
					t.Errorf("Name: got %q, want %q", fc.Name, "get_weather")
				}
				if fc.Args["location"] != "NYC" {
					t.Errorf("Args[location]: got %v, want %q", fc.Args["location"], "NYC")
				}
			}
		}
	}
	if !foundFC {
		t.Errorf("got no FunctionCall partial, want one")
	}

	// Verify final response.
	if len(results) == 0 {
		t.Fatal("got no results, want at least one")
	}
	last := results[len(results)-1]
	if last.resp == nil || !last.resp.TurnComplete {
		t.Errorf("last: got %+v, want TurnComplete=true", last.resp)
	}
}

func TestParseResponsesStream_FunctionCallArgsBuffered(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.output_item.added","item_id":"item_1","item":{"type":"function_call","call_id":"call_1","name":"fn"}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"{\"a\":"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"1}"}`,
		`data: {"type":"response.function_call_arguments.done","item_id":"item_1"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"function_call","call_id":"call_1","name":"fn","arguments":"{\"a\":1}"}]}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	// Find the FunctionCall partial — args should come from buffered deltas.
	for _, r := range results {
		if r.resp == nil || r.resp.Content == nil {
			continue
		}
		for _, p := range r.resp.Content.Parts {
			if p.FunctionCall != nil && r.resp.Partial {
				if p.FunctionCall.Args["a"] != float64(1) {
					t.Errorf("Args[a]: got %v, want 1", p.FunctionCall.Args["a"])
				}
			}
		}
	}
}

func TestParseResponsesStream_OutputItemAdded_TracksMappings(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.output_item.added","item_id":"item_1","item":{"type":"function_call","call_id":"call_x","name":"my_fn"}}`,
		`data: {"type":"response.function_call_arguments.done","item_id":"item_1","arguments":"{}"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"function_call","call_id":"call_x","name":"my_fn","arguments":"{}"}]}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	for _, r := range results {
		if r.err != nil {
			t.Errorf("unexpected error: %v", r.err)
		}
		if r.resp == nil || r.resp.Content == nil {
			continue
		}
		for _, p := range r.resp.Content.Parts {
			if p.FunctionCall != nil && r.resp.Partial {
				if p.FunctionCall.ID != "call_x" {
					t.Errorf("ID: got %q, want %q", p.FunctionCall.ID, "call_x")
				}
				if p.FunctionCall.Name != "my_fn" {
					t.Errorf("Name: got %q, want %q", p.FunctionCall.Name, "my_fn")
				}
			}
		}
	}
}

func TestParseResponsesStream_ResponseCompleted(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi"}]}],"usage":{"input_tokens":5,"output_tokens":1,"total_tokens":6}}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	if len(results) == 0 {
		t.Fatal("got no results, want at least one")
	}
	last := results[len(results)-1]
	if last.err != nil {
		t.Fatalf("last error: %v", last.err)
	}
	if last.resp == nil || !last.resp.TurnComplete {
		t.Errorf("last: got %+v, want TurnComplete=true", last.resp)
	}
	if last.resp.FinishReason != genai.FinishReasonStop {
		t.Errorf("finish reason: got %v, want %v", last.resp.FinishReason, genai.FinishReasonStop)
	}
	if last.resp.UsageMetadata == nil {
		t.Fatal("got nil UsageMetadata, want non-nil")
	}
	if last.resp.UsageMetadata.TotalTokenCount != 6 {
		t.Errorf("total tokens: got %d, want 6", last.resp.UsageMetadata.TotalTokenCount)
	}
	if last.resp.CustomMetadata["openai_response_id"] != "resp_1" {
		t.Errorf("openai_response_id: got %v, want %q", last.resp.CustomMetadata["openai_response_id"], "resp_1")
	}
}

func TestParseResponsesStream_ResponseIncomplete(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.output_text.delta","delta":"partial"}`,
		`data: {"type":"response.incomplete","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}],"incomplete_details":{"reason":"max_output_tokens"}}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	if len(results) == 0 {
		t.Fatal("got no results, want at least one")
	}
	last := results[len(results)-1]
	if last.resp == nil || !last.resp.TurnComplete {
		t.Errorf("last: got %+v, want TurnComplete=true", last.resp)
	}
	if last.resp.FinishReason != genai.FinishReasonMaxTokens {
		t.Errorf("finish reason: got %v, want %v", last.resp.FinishReason, genai.FinishReasonMaxTokens)
	}
}

func TestParseResponsesStream_ResponseFailed(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.failed","response":{"error":{"message":"rate limit exceeded"}}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].err == nil {
		t.Fatal("got nil error, want error")
	}
	if !strings.Contains(results[0].err.Error(), "rate limit exceeded") {
		t.Errorf("error: got %v, want to contain 'rate limit exceeded'", results[0].err)
	}
}

func TestParseResponsesStream_ResponseFailed_EmptyMessage(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.failed","response":{}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].err == nil {
		t.Fatal("got nil error, want error")
	}
	if !strings.Contains(results[0].err.Error(), "response failed") {
		t.Errorf("error: got %v, want to contain 'response failed'", results[0].err)
	}
}

func TestParseResponsesStream_ErrorEvent(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"error","message":"something went wrong"}`,
	))

	results := collectResponsesStream(context.Background(), body)

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].err == nil {
		t.Fatal("got nil error, want error")
	}
	if !strings.Contains(results[0].err.Error(), "something went wrong") {
		t.Errorf("error: got %v, want to contain 'something went wrong'", results[0].err)
	}
}

func TestParseResponsesStream_ErrorEvent_EmptyMessage(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"error"}`,
	))

	results := collectResponsesStream(context.Background(), body)

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].err == nil {
		t.Fatal("got nil error, want error")
	}
	if !strings.Contains(results[0].err.Error(), "stream error") {
		t.Errorf("error: got %v, want to contain 'stream error'", results[0].err)
	}
}

func TestParseResponsesStream_NoTerminalEvent(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
	))

	results := collectResponsesStream(context.Background(), body)

	// Should have 1 text partial, no final response.
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].resp == nil || !results[0].resp.Partial {
		t.Errorf("result: got %+v, want Partial=true", results[0].resp)
	}
	if results[0].resp.TurnComplete {
		t.Errorf("result: got TurnComplete=true, want false (no terminal event)")
	}
}

func TestParseResponsesStream_ContextCancellation(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.output_text.delta","delta":"a"}`,
		`data: {"type":"response.output_text.delta","delta":"b"}`,
		`data: {"type":"response.output_text.delta","delta":"c"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"abc"}]}]}}`,
	))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var results []responsesStreamResult
	for resp, err := range parseResponsesStream(ctx, body) {
		results = append(results, responsesStreamResult{resp: resp, err: err})
		cancel()
	}

	if len(results) == 0 {
		t.Fatal("got no results, want at least one")
	}

	foundInterrupted := false
	for _, r := range results {
		if r.resp != nil && r.resp.Interrupted {
			foundInterrupted = true
			break
		}
	}
	if !foundInterrupted {
		t.Errorf("got no Interrupted=true result, want one after cancellation")
	}
}

func TestParseResponsesStream_UnknownEventSkipped(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.queued"}`,
		`data: {"type":"response.steer"}`,
		`data: {"type":"some_unknown_event","delta":"skip me"}`,
		`data: {"type":"response.output_text.delta","delta":"text"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"text"}]}]}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	// Unknown events should be skipped — only text partial + final.
	for _, r := range results {
		if r.err != nil {
			t.Errorf("unexpected error: %v", r.err)
		}
	}
	// Should have at least the text partial and the final.
	if len(results) < 2 {
		t.Errorf("got %d results, want at least 2", len(results))
	}
}

func TestParseResponsesStream_EmptyDeltaSkipped(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.output_text.delta","delta":""}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}]}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	// Empty delta should not produce a partial — only the final response.
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (empty delta skipped)", len(results))
	}
	if results[0].resp == nil || !results[0].resp.TurnComplete {
		t.Errorf("result: got %+v, want TurnComplete=true", results[0].resp)
	}
}

func TestParseResponsesStream_MalformedJSON(t *testing.T) {
	t.Parallel()
	body := strings.NewReader("data: {invalid\n")

	results := collectResponsesStream(context.Background(), body)

	if len(results) == 0 {
		t.Fatal("got no results, want at least one error")
	}
	foundErr := false
	for _, r := range results {
		if r.err != nil {
			foundErr = true
			break
		}
	}
	if !foundErr {
		t.Errorf("got no error result for malformed JSON, want one")
	}
}

func TestParseResponsesStream_ResponseCreated(t *testing.T) {
	t.Parallel()
	body := strings.NewReader(sseLines(
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-4o"}}`,
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}]}}`,
	))

	results := collectResponsesStream(context.Background(), body)

	// response.created should not produce a yield — only text partial + final.
	for _, r := range results {
		if r.err != nil {
			t.Errorf("unexpected error: %v", r.err)
		}
	}
	// Should have text partial + final = 2 results.
	if len(results) != 2 {
		t.Errorf("got %d results, want 2 (created event should not yield)", len(results))
	}
}

// --- HTTP integration streaming tests ---

func TestGenerateResponses_Streaming(t *testing.T) {
	t.Parallel()
	sseBody := sseLines(
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.output_text.delta","delta":" there"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi there"}]}]}}`,
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("got %s, want /responses", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseBody)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
		API:     APIResponses,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}}},
	}

	resps, errs := collectResponses(m, context.Background(), req, true)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(resps) < 2 {
		t.Fatalf("got %d responses, want at least 2", len(resps))
	}

	// Verify at least one partial.
	foundPartial := false
	for _, resp := range resps {
		if resp.Partial && resp.Content != nil && len(resp.Content.Parts) > 0 && resp.Content.Parts[0].Text != "" {
			foundPartial = true
			break
		}
	}
	if !foundPartial {
		t.Errorf("got no partial text responses, want at least one")
	}

	// Verify last is TurnComplete.
	last := resps[len(resps)-1]
	if !last.TurnComplete {
		t.Errorf("last: got TurnComplete=false, want true")
	}
}

func TestGenerateResponses_StreamingError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad"}`, http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
		API:     APIResponses,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}

	_, errs := collectResponses(m, context.Background(), req, true)
	if len(errs) == 0 {
		t.Fatal("got no errors, want at least one")
	}
	var httpErr *HTTPError
	if !errors.As(errs[0], &httpErr) {
		t.Errorf("got %T, want *HTTPError", errs[0])
	}
}

// --- Fuzz test ---

func FuzzParseResponsesStream(f *testing.F) {
	// Seed: valid text stream.
	f.Add(sseLines(
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}]}}`,
	))
	// Seed: valid function call stream.
	f.Add(sseLines(
		`data: {"type":"response.output_item.added","item_id":"i1","item":{"type":"function_call","call_id":"c1","name":"fn"}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"i1","delta":"{}"}`,
		`data: {"type":"response.function_call_arguments.done","item_id":"i1","arguments":"{}"}`,
		`data: {"type":"response.completed","response":{"id":"r1","model":"m","output":[{"type":"function_call","call_id":"c1","name":"fn","arguments":"{}"}]}}`,
	))
	// Seed: valid reasoning stream.
	f.Add(sseLines(
		`data: {"type":"response.reasoning_text.delta","delta":"thinking"}`,
		`data: {"type":"response.completed","response":{"id":"r1","model":"m","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}]}}`,
	))
	// Seed: malformed JSON.
	f.Add("data: {invalid\n")
	// Seed: empty input.
	f.Add("")
	// Seed: partial event.
	f.Add("data: {")

	f.Fuzz(func(t *testing.T, input string) {
		ctx := context.Background()
		for resp, err := range parseResponsesStream(ctx, strings.NewReader(input)) {
			_ = resp
			_ = err
		}
	})
}
