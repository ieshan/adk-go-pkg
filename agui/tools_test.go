package agui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	// Brief pause so Wait registers before Submit.
	time.Sleep(50 * time.Millisecond)

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

	// Submit after a short delay.
	time.Sleep(100 * time.Millisecond)
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
		t.Fatalf("expected timeout error, got result %q", result)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

func TestToolResultHandler_ContextCancel(t *testing.T) {
	h := agui.NewToolResultHandler()
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := h.Wait(ctx, "call-cancel", 5*time.Second)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got: %v", err)
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

	// Let all goroutines register.
	time.Sleep(100 * time.Millisecond)

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
		t.Fatal("expected error for non-pending tool call")
	}
	if !strings.Contains(err.Error(), "no pending tool call") {
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
	time.Sleep(50 * time.Millisecond)

	body := `{"toolCallId":"tc-1","content":"result-data"}`
	req := httptest.NewRequest(http.MethodPost, "/tool-result", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	endpoint.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
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
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestToolResultEndpoint_MethodNotAllowed(t *testing.T) {
	h := agui.NewToolResultHandler()
	endpoint := agui.ToolResultEndpoint(h)

	req := httptest.NewRequest(http.MethodGet, "/tool-result", nil)
	rec := httptest.NewRecorder()

	endpoint.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
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
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
