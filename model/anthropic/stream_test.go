package anthropic

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// buildSSE constructs a minimal Anthropic-style SSE stream from data lines.
// Each entry is a JSON payload prefixed with "data: ".
func buildSSE(lines ...string) string {
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString("data: ")
		sb.WriteString(l)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// TestParseStream_TextOnly verifies the core streaming lifecycle for a single
// text block.
func TestParseStream_TextOnly(t *testing.T) {
	stream := buildSSE(
		`{"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-opus-4","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	)

	ctx := context.Background()
	var partials []string
	var final *model.LLMResponse
	for resp, err := range parseStream(ctx, strings.NewReader(stream)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Partial {
			if resp.Content != nil && len(resp.Content.Parts) > 0 {
				partials = append(partials, resp.Content.Parts[0].Text)
			}
		}
		if resp.TurnComplete {
			final = resp
		}
	}

	if len(partials) != 2 {
		t.Fatalf("expected 2 partials, got %d: %v", len(partials), partials)
	}
	if partials[0] != "Hello" {
		t.Errorf("partial[0]: got %q, want %q", partials[0], "Hello")
	}
	if partials[1] != " world" {
		t.Errorf("partial[1]: got %q, want %q", partials[1], " world")
	}
	if final == nil {
		t.Fatal("expected final TurnComplete response")
	}
	if !final.TurnComplete {
		t.Error("expected TurnComplete=true")
	}
	if final.FinishReason != genai.FinishReasonStop {
		t.Errorf("finish_reason: got %v, want %v", final.FinishReason, genai.FinishReasonStop)
	}
	if final.Content == nil || len(final.Content.Parts) != 1 {
		t.Fatal("expected 1 part in final content")
	}
	if final.Content.Parts[0].Text != "Hello world" {
		t.Errorf("final text: got %q, want %q", final.Content.Parts[0].Text, "Hello world")
	}
}

// TestParseStream_ToolUse verifies that tool_use blocks with partial JSON
// deltas accumulate correctly and parse into FunctionCall parts.
func TestParseStream_ToolUse(t *testing.T) {
	stream := buildSSE(
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01X","name":"get_weather","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"location\":\"Paris"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":25}}`,
		`{"type":"message_stop"}`,
	)

	ctx := context.Background()
	var final *model.LLMResponse
	for resp, err := range parseStream(ctx, strings.NewReader(stream)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.TurnComplete {
			final = resp
		}
	}

	if final == nil {
		t.Fatal("expected final TurnComplete response")
	}
	if final.Content == nil || len(final.Content.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(final.Content.Parts))
	}
	fc := final.Content.Parts[0].FunctionCall
	if fc == nil {
		t.Fatal("expected FunctionCall part")
	}
	if fc.ID != "toolu_01X" {
		t.Errorf("ID: got %q, want %q", fc.ID, "toolu_01X")
	}
	if fc.Name != "get_weather" {
		t.Errorf("Name: got %q, want %q", fc.Name, "get_weather")
	}
	if fc.Args["location"] != "Paris" {
		t.Errorf("Args.location: got %v, want %q", fc.Args["location"], "Paris")
	}
}

// TestParseStream_MultipleBlocks verifies index tracking across different block
// types (text followed by tool_use).
func TestParseStream_MultipleBlocks(t *testing.T) {
	stream := buildSSE(
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"The weather is"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_02Y","name":"get_weather","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":10}}`,
		`{"type":"message_stop"}`,
	)

	ctx := context.Background()
	var final *model.LLMResponse
	for resp, err := range parseStream(ctx, strings.NewReader(stream)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.TurnComplete {
			final = resp
		}
	}

	if final == nil {
		t.Fatal("expected final TurnComplete response")
	}
	if final.Content == nil || len(final.Content.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(final.Content.Parts))
	}
	if final.Content.Parts[0].Text != "The weather is" {
		t.Errorf("text: got %q, want %q", final.Content.Parts[0].Text, "The weather is")
	}
	if final.Content.Parts[1].FunctionCall == nil {
		t.Error("expected FunctionCall on second part")
	}
}

// TestParseStream_Interrupted verifies that context cancellation yields
// Interrupted=true.
func TestParseStream_Interrupted(t *testing.T) {
	stream := buildSSE(
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
	)
	// The stream never reaches message_stop, so we rely on context cancellation.
	// Since parseStream checks ctx.Err() before each line, we need a context
	// that is already cancelled. However, the scanner will still read lines.
	// A better approach: use a slow reader or cancelled context. Since the
	// scanner reads all lines from memory immediately, ctx.Err() will be checked
	// on the first iteration and return Interrupted.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var gotInterrupted bool
	for resp, err := range parseStream(ctx, strings.NewReader(stream)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Interrupted {
			gotInterrupted = true
		}
	}
	if !gotInterrupted {
		t.Error("expected Interrupted=true on cancelled context")
	}
}

// TestParseStream_MalformedJSON verifies that malformed SSE data yields an
// error without panicking.
func TestParseStream_MalformedJSON(t *testing.T) {
	stream := buildSSE(
		`{"type":"content_block_start","index":0,"content_block":`, // truncated JSON
	)

	ctx := context.Background()
	var gotError bool
	for _, err := range parseStream(ctx, strings.NewReader(stream)) {
		if err != nil {
			gotError = true
		}
	}
	if !gotError {
		t.Error("expected error for malformed JSON")
	}
}

// TestParseStream_Thinking verifies that thinking blocks stream correctly and
// that the signature from content_block_start is preserved.
func TestParseStream_Thinking(t *testing.T) {
	stream := buildSSE(
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":"sigABC123"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"I should"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" check the weather"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}`,
		`{"type":"message_stop"}`,
	)

	ctx := context.Background()
	var final *model.LLMResponse
	for resp, err := range parseStream(ctx, strings.NewReader(stream)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.TurnComplete {
			final = resp
		}
	}

	if final == nil {
		t.Fatal("expected final TurnComplete response")
	}
	if final.Content == nil || len(final.Content.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(final.Content.Parts))
	}
	part := final.Content.Parts[0]
	if !part.Thought {
		t.Error("expected Thought=true")
	}
	if part.Text != "I should check the weather" {
		t.Errorf("thinking text: got %q, want %q", part.Text, "I should check the weather")
	}
	if string(part.ThoughtSignature) != "sigABC123" {
		t.Errorf("signature: got %q, want %q", part.ThoughtSignature, "sigABC123")
	}
}

// TestParseStream_UsageAndCache verifies that message_delta usage and cache
// tokens are captured into UsageMetadata and CustomMetadata.
func TestParseStream_UsageAndCache(t *testing.T) {
	stream := buildSSE(
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":15,"cache_creation_input_tokens":100,"cache_read_input_tokens":200}}`,
		`{"type":"message_stop"}`,
	)

	ctx := context.Background()
	var final *model.LLMResponse
	for resp, err := range parseStream(ctx, strings.NewReader(stream)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.TurnComplete {
			final = resp
		}
	}

	if final == nil {
		t.Fatal("expected final TurnComplete response")
	}
	if final.UsageMetadata == nil {
		t.Fatal("expected non-nil UsageMetadata")
	}
	if final.UsageMetadata.CandidatesTokenCount != 15 {
		t.Errorf("output_tokens: got %d, want 15", final.UsageMetadata.CandidatesTokenCount)
	}
	if final.UsageMetadata.CachedContentTokenCount != 200 {
		t.Errorf("cache_read: got %d, want 200", final.UsageMetadata.CachedContentTokenCount)
	}
	if final.CustomMetadata == nil {
		t.Fatal("expected non-nil CustomMetadata")
	}
	if final.CustomMetadata["cache_creation_input_tokens"] != int32(100) {
		t.Errorf("cache_creation: got %v, want 100", final.CustomMetadata["cache_creation_input_tokens"])
	}
	if final.CustomMetadata["cache_read_input_tokens"] != int32(200) {
		t.Errorf("cache_read: got %v, want 200", final.CustomMetadata["cache_read_input_tokens"])
	}
}
