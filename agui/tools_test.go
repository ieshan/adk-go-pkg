package agui_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ieshan/adk-go-pkg/agui"
)

func TestToolResultHandler_SubmitThenWait(t *testing.T) {
	h := agui.NewToolResultHandler()
	ctx := context.Background()

	var result string
	var err error
	done := make(chan struct{})

	// Start Wait in a goroutine first (it registers the channel).
	go func() {
		result, err = h.Wait(ctx, "call-1", 2*time.Second)
		close(done)
	}()

	// Deterministically wait for Wait to register its pending entry before
	// calling SubmitResult, instead of relying on a fixed sleep.
	for !h.HasPendingToolCall("call-1") {
		runtime.Gosched()
	}

	if submitErr := h.SubmitResult("call-1", "hello"); submitErr != nil {
		t.Fatalf("SubmitResult: %v", submitErr)
	}

	<-done
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if result != "hello" {
		t.Fatalf("got %q, want %q", result, "hello")
	}
}

func TestToolResultHandler_WaitThenSubmit(t *testing.T) {
	h := agui.NewToolResultHandler()
	ctx := context.Background()

	var result string
	var err error
	done := make(chan struct{})

	go func() {
		result, err = h.Wait(ctx, "call-2", 2*time.Second)
		close(done)
	}()

	// Deterministically wait for Wait to register before submitting.
	for !h.HasPendingToolCall("call-2") {
		runtime.Gosched()
	}
	if submitErr := h.SubmitResult("call-2", "world"); submitErr != nil {
		t.Fatalf("SubmitResult: %v", submitErr)
	}

	<-done
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if result != "world" {
		t.Fatalf("got %q, want %q", result, "world")
	}
}

func TestToolResultHandler_Timeout(t *testing.T) {
	h := agui.NewToolResultHandler()
	ctx := context.Background()

	result, err := h.Wait(ctx, "call-timeout", 50*time.Millisecond)
	if err == nil {
		t.Fatalf("got result %q, want timeout error", result)
	}
	if !errors.Is(err, agui.ErrToolCallTimeout) {
		t.Fatalf("got %v, want timeout error", err)
	}
}

func TestToolResultHandler_ContextCancel(t *testing.T) {
	h := agui.NewToolResultHandler()
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// Deterministically wait for Wait to register before cancelling,
		// so we test the "cancel while blocked" path rather than
		// "cancel before Wait runs".
		for !h.HasPendingToolCall("call-cancel") {
			runtime.Gosched()
		}
		cancel()
	}()

	_, err := h.Wait(ctx, "call-cancel", 5*time.Second)
	if err != context.Canceled {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestToolResultHandler_ConcurrentCalls(t *testing.T) {
	h := agui.NewToolResultHandler()
	ctx := context.Background()
	const n = 10

	var wg sync.WaitGroup
	results := make([]string, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := strings.Repeat("x", i+1) // unique IDs
			results[i], errs[i] = h.Wait(ctx, id, 2*time.Second)
		}(i)
	}

	// Deterministically wait for all goroutines to register their pending
	// entries before submitting results.
	for i := 0; i < n; i++ {
		id := strings.Repeat("x", i+1)
		for !h.HasPendingToolCall(id) {
			runtime.Gosched()
		}
	}

	for i := 0; i < n; i++ {
		id := strings.Repeat("x", i+1)
		if err := h.SubmitResult(id, id); err != nil {
			t.Fatalf("SubmitResult(%q): %v", id, err)
		}
	}

	wg.Wait()
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("call %d error: %v", i, errs[i])
		}
		expected := strings.Repeat("x", i+1)
		if results[i] != expected {
			t.Errorf("call %d: got %q, want %q", i, results[i], expected)
		}
	}
}

func TestToolResultHandler_SubmitNoWaiter(t *testing.T) {
	h := agui.NewToolResultHandler()
	err := h.SubmitResult("nonexistent", "data")
	if err == nil {
		t.Fatal("got nil error, want error for non-pending tool call")
	}
	if !errors.Is(err, agui.ErrNoPendingToolCall) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestToolResultEndpoint_Success(t *testing.T) {
	h := agui.NewToolResultHandler()
	endpoint := agui.ToolResultEndpoint(h)

	// Start a waiter so Submit succeeds.
	done := make(chan struct{})
	go func() {
		_, _ = h.Wait(context.Background(), "tc-1", 2*time.Second)
		close(done)
	}()
	// Deterministically wait for Wait to register before serving the request.
	for !h.HasPendingToolCall("tc-1") {
		runtime.Gosched()
	}

	body := `{"toolCallId":"tc-1","content":"result-data"}`
	req := httptest.NewRequest(http.MethodPost, "/tool-result", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	endpoint.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s, want 200", rec.Code, rec.Body.String())
	}
	<-done
}

func TestToolResultEndpoint_InvalidJSON(t *testing.T) {
	h := agui.NewToolResultHandler()
	endpoint := agui.ToolResultEndpoint(h)

	req := httptest.NewRequest(http.MethodPost, "/tool-result", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()

	endpoint.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestToolResultEndpoint_MethodNotAllowed(t *testing.T) {
	h := agui.NewToolResultHandler()
	endpoint := agui.ToolResultEndpoint(h)

	req := httptest.NewRequest(http.MethodGet, "/tool-result", nil)
	rec := httptest.NewRecorder()

	endpoint.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d, want 405", rec.Code)
	}
}

func TestToolResultEndpoint_MissingToolCallId(t *testing.T) {
	h := agui.NewToolResultHandler()
	endpoint := agui.ToolResultEndpoint(h)

	body := `{"toolCallId":"","content":"data"}`
	req := httptest.NewRequest(http.MethodPost, "/tool-result", strings.NewReader(body))
	rec := httptest.NewRecorder()

	endpoint.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}
