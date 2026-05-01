package testutil

import (
	"context"
	"errors"
	"testing"

	"iter"

	"google.golang.org/adk/model"
)

func TestFakeLLM_Name(t *testing.T) {
	f := NewFakeLLM()
	if got := f.Name(); got != "fake-llm" {
		t.Errorf("Name() = %q, want %q", got, "fake-llm")
	}

	f.WithName("custom-model")
	if got := f.Name(); got != "custom-model" {
		t.Errorf("WithName() = %q, want %q", got, "custom-model")
	}
}

func TestFakeLLM_NonStreaming(t *testing.T) {
	resp1 := NewTextResponse("hello")
	resp2 := NewTextResponse("world")
	f := NewFakeLLM(resp1, resp2)

	req := NewLLMRequest(NewUserContent("hi"))

	// First call should return resp1.
	got := collectLLMResponses(t, f.GenerateContent(context.Background(), req, false))
	if len(got) != 1 {
		t.Fatalf("expected 1 response, got %d", len(got))
	}
	if got[0].Content.Parts[0].Text != "hello" {
		t.Errorf("first call text = %q, want %q", got[0].Content.Parts[0].Text, "hello")
	}
	if !got[0].TurnComplete {
		t.Error("first call: TurnComplete should be true")
	}

	// Second call should return resp2.
	got = collectLLMResponses(t, f.GenerateContent(context.Background(), req, false))
	if len(got) != 1 {
		t.Fatalf("expected 1 response, got %d", len(got))
	}
	if got[0].Content.Parts[0].Text != "world" {
		t.Errorf("second call text = %q, want %q", got[0].Content.Parts[0].Text, "world")
	}

	// Third call should repeat the last response.
	got = collectLLMResponses(t, f.GenerateContent(context.Background(), req, false))
	if got[0].Content.Parts[0].Text != "world" {
		t.Errorf("third call text = %q, want %q (repeated)", got[0].Content.Parts[0].Text, "world")
	}
}

func TestFakeLLM_Streaming(t *testing.T) {
	resp1 := NewTextResponse("chunk1")
	resp2 := NewTextResponse("chunk2")
	resp3 := NewTextResponse("chunk3")
	f := NewFakeLLM(resp1, resp2, resp3)

	req := NewLLMRequest(NewUserContent("hi"))
	got := collectLLMResponses(t, f.GenerateContent(context.Background(), req, true))

	if len(got) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(got))
	}

	// First two should be partial.
	if !got[0].Partial {
		t.Error("response 0: Partial should be true")
	}
	if got[0].TurnComplete {
		t.Error("response 0: TurnComplete should be false")
	}
	if !got[1].Partial {
		t.Error("response 1: Partial should be true")
	}
	if got[1].TurnComplete {
		t.Error("response 1: TurnComplete should be false")
	}

	// Last should be final.
	if got[2].Partial {
		t.Error("response 2: Partial should be false")
	}
	if !got[2].TurnComplete {
		t.Error("response 2: TurnComplete should be true")
	}
}

func TestFakeLLM_SingleResponseStreaming(t *testing.T) {
	f := NewFakeLLM(NewTextResponse("only"))
	req := NewLLMRequest(NewUserContent("hi"))
	got := collectLLMResponses(t, f.GenerateContent(context.Background(), req, true))

	if len(got) != 1 {
		t.Fatalf("expected 1 response, got %d", len(got))
	}
	if got[0].Partial {
		t.Error("single response: Partial should be false")
	}
	if !got[0].TurnComplete {
		t.Error("single response: TurnComplete should be true")
	}
}

func TestFakeLLM_ErrorInjection(t *testing.T) {
	f := NewFakeLLM(NewTextResponse("ok"))
	f.SetError(errors.New("boom"))

	req := NewLLMRequest(NewUserContent("hi"))
	var gotErr error
	for _, err := range f.GenerateContent(context.Background(), req, false) {
		if err != nil {
			gotErr = err
		}
	}
	if gotErr == nil || gotErr.Error() != "boom" {
		t.Errorf("expected error 'boom', got %v", gotErr)
	}

	// Error persists until cleared.
	f.ClearError()
	got := collectLLMResponses(t, f.GenerateContent(context.Background(), req, false))
	if len(got) != 1 {
		t.Fatalf("expected 1 response after ClearError, got %d", len(got))
	}
}

func TestFakeLLM_CallRecording(t *testing.T) {
	f := NewFakeLLM(NewTextResponse("ok"))

	req1 := NewLLMRequest(NewUserContent("first"))
	req2 := NewLLMRequest(NewUserContent("second"))

	collectLLMResponses(t, f.GenerateContent(context.Background(), req1, false))
	collectLLMResponses(t, f.GenerateContent(context.Background(), req2, false))

	if f.CallCount() != 2 {
		t.Errorf("CallCount() = %d, want 2", f.CallCount())
	}

	last := f.LastCall()
	if last.Contents[0].Parts[0].Text != "second" {
		t.Errorf("LastCall text = %q, want %q", last.Contents[0].Parts[0].Text, "second")
	}

	at0 := f.CallsAt(0)
	if at0.Contents[0].Parts[0].Text != "first" {
		t.Errorf("CallsAt(0) text = %q, want %q", at0.Contents[0].Parts[0].Text, "first")
	}

	if f.CallsAt(-1) != nil || f.CallsAt(99) != nil {
		t.Error("CallsAt out of range should return nil")
	}
}

func TestFakeLLM_AddResponse(t *testing.T) {
	f := NewFakeLLM(NewTextResponse("first"))
	f.AddResponse(NewTextResponse("second"))

	req := NewLLMRequest(NewUserContent("hi"))

	collectLLMResponses(t, f.GenerateContent(context.Background(), req, false))
	got := collectLLMResponses(t, f.GenerateContent(context.Background(), req, false))
	if got[0].Content.Parts[0].Text != "second" {
		t.Errorf("second call text = %q, want %q", got[0].Content.Parts[0].Text, "second")
	}
}

func TestFakeLLM_Reset(t *testing.T) {
	f := NewFakeLLM(NewTextResponse("ok"))
	req := NewLLMRequest(NewUserContent("hi"))
	collectLLMResponses(t, f.GenerateContent(context.Background(), req, false))

	f.Reset()
	if f.CallCount() != 0 {
		t.Errorf("after Reset, CallCount() = %d, want 0", f.CallCount())
	}
	if f.LastCall() != nil {
		t.Error("after Reset, LastCall() should be nil")
	}
}

func TestFakeLLM_ContextCancellation(t *testing.T) {
	f := NewFakeLLM(NewTextResponse("a"), NewTextResponse("b"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := NewLLMRequest(NewUserContent("hi"))
	got := collectLLMResponses(t, f.GenerateContent(ctx, req, true))
	// With a cancelled context, no responses should be yielded.
	if len(got) != 0 {
		t.Errorf("expected 0 responses with cancelled context, got %d", len(got))
	}
}

func TestFakeLLM_DefaultResponse(t *testing.T) {
	f := NewFakeLLM() // no responses
	req := NewLLMRequest(NewUserContent("hi"))
	got := collectLLMResponses(t, f.GenerateContent(context.Background(), req, false))
	if len(got) != 1 {
		t.Fatalf("expected 1 default response, got %d", len(got))
	}
}

// collectLLMResponses collects all LLMResponses from an iterator.
func collectLLMResponses(t *testing.T, seq iter.Seq2[*model.LLMResponse, error]) []*model.LLMResponse {
	t.Helper()
	var resps []*model.LLMResponse
	for resp, err := range seq {
		if err != nil {
			t.Fatalf("unexpected error from FakeLLM: %v", err)
		}
		resps = append(resps, resp)
	}
	return resps
}
