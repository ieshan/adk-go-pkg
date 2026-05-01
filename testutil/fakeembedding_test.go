package testutil

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestFakeEmbedding_Basic(t *testing.T) {
	f := NewFakeEmbedding()

	vec, err := f.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed() unexpected error: %v", err)
	}

	if len(vec) != 1536 {
		t.Errorf("expected 1536 dimensions, got %d", len(vec))
	}

	// Verify dimension accessor
	if f.Dimension() != 1536 {
		t.Errorf("Dimension() = %d, want 1536", f.Dimension())
	}
}

func TestFakeEmbedding_Deterministic(t *testing.T) {
	f := NewFakeEmbedding()
	ctx := context.Background()

	vec1, err := f.Embed(ctx, "test text")
	if err != nil {
		t.Fatalf("first Embed() error: %v", err)
	}

	vec2, err := f.Embed(ctx, "test text")
	if err != nil {
		t.Fatalf("second Embed() error: %v", err)
	}

	// Same input should produce same output
	for i := range vec1 {
		if vec1[i] != vec2[i] {
			t.Errorf("embeddings differ at index %d: %v vs %v", i, vec1[i], vec2[i])
			break
		}
	}

	// Different input should produce different output
	vec3, err := f.Embed(ctx, "different text")
	if err != nil {
		t.Fatalf("third Embed() error: %v", err)
	}

	different := false
	for i := range vec1 {
		if vec1[i] != vec3[i] {
			different = true
			break
		}
	}
	if !different {
		t.Error("different texts produced identical embeddings")
	}
}

func TestFakeEmbedding_CustomDimension(t *testing.T) {
	f := NewFakeEmbedding().WithDimension(768)

	vec, err := f.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}

	if len(vec) != 768 {
		t.Errorf("expected 768 dimensions, got %d", len(vec))
	}

	if f.Dimension() != 768 {
		t.Errorf("Dimension() = %d, want 768", f.Dimension())
	}
}

func TestFakeEmbedding_Precomputed(t *testing.T) {
	f := NewFakeEmbedding()
	precomputed := []float32{0.1, 0.2, 0.3, 0.4, 0.5}

	f.WithPrecomputedEmbedding("known text", precomputed)

	vec, err := f.Embed(context.Background(), "known text")
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}

	if len(vec) != len(precomputed) {
		t.Fatalf("expected %d dimensions, got %d", len(precomputed), len(vec))
	}

	for i := range precomputed {
		if vec[i] != precomputed[i] {
			t.Errorf("vec[%d] = %v, want %v", i, vec[i], precomputed[i])
		}
	}
}

func TestFakeEmbedding_PrecomputedCopy(t *testing.T) {
	f := NewFakeEmbedding()
	precomputed := []float32{0.1, 0.2, 0.3}

	f.WithPrecomputedEmbedding("test", precomputed)

	// Modify original after setting
	precomputed[0] = 999

	vec, err := f.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}

	// Should still return original values, not modified
	if vec[0] != 0.1 {
		t.Errorf("precomputed embedding was not copied, got %v", vec[0])
	}
}

func TestFakeEmbedding_CallRecording(t *testing.T) {
	f := NewFakeEmbedding()
	ctx := context.Background()

	f.Embed(ctx, "first")
	f.Embed(ctx, "second")
	f.Embed(ctx, "third")

	if f.CallCount() != 3 {
		t.Errorf("CallCount() = %d, want 3", f.CallCount())
	}

	calls := f.Calls()
	if len(calls) != 3 {
		t.Errorf("len(Calls()) = %d, want 3", len(calls))
	}

	expected := []string{"first", "second", "third"}
	for i, exp := range expected {
		if calls[i] != exp {
			t.Errorf("Calls()[%d] = %q, want %q", i, calls[i], exp)
		}
	}

	last := f.LastCall()
	if last != "third" {
		t.Errorf("LastCall() = %q, want %q", last, "third")
	}
}

func TestFakeEmbedding_LastCallEmpty(t *testing.T) {
	f := NewFakeEmbedding()

	if f.LastCall() != "" {
		t.Errorf("LastCall() = %q, want empty string", f.LastCall())
	}
}

func TestFakeEmbedding_Reset(t *testing.T) {
	f := NewFakeEmbedding()
	ctx := context.Background()

	f.Embed(ctx, "test")
	f.SetError(errors.New("boom"))

	f.Reset()

	if f.CallCount() != 0 {
		t.Errorf("after Reset, CallCount() = %d, want 0", f.CallCount())
	}

	if f.LastCall() != "" {
		t.Errorf("after Reset, LastCall() = %q, want empty", f.LastCall())
	}

	// Should work normally after reset
	vec, err := f.Embed(ctx, "after reset")
	if err != nil {
		t.Errorf("after Reset, Embed() error: %v", err)
	}
	if len(vec) != 1536 {
		t.Errorf("after Reset, expected 1536 dimensions, got %d", len(vec))
	}
}

func TestFakeEmbedding_ErrorInjection(t *testing.T) {
	f := NewFakeEmbedding()
	ctx := context.Background()

	testErr := errors.New("embedding service unavailable")
	f.SetError(testErr)

	_, err := f.Embed(ctx, "test")
	if err != testErr {
		t.Errorf("expected error %v, got %v", testErr, err)
	}

	// Error should still record the call
	if f.CallCount() != 1 {
		t.Errorf("CallCount() = %d, want 1", f.CallCount())
	}

	// Clear error and verify it works again
	f.ClearError()

	vec, err := f.Embed(ctx, "after clear")
	if err != nil {
		t.Fatalf("after ClearError, Embed() error: %v", err)
	}
	if len(vec) != 1536 {
		t.Errorf("expected 1536 dimensions, got %d", len(vec))
	}
}

func TestFakeEmbedding_ContextCancellation(t *testing.T) {
	f := NewFakeEmbedding()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := f.Embed(ctx, "test")
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestFakeEmbedding_AsFunc(t *testing.T) {
	f := NewFakeEmbedding()
	fn := f.AsFunc()

	// Verify it returns the correct function type
	vec, err := fn(context.Background(), "test")
	if err != nil {
		t.Fatalf("AsFunc() returned function error: %v", err)
	}
	if len(vec) != 1536 {
		t.Errorf("expected 1536 dimensions, got %d", len(vec))
	}

	// Verify call is recorded on the FakeEmbedding
	if f.CallCount() != 1 {
		t.Errorf("CallCount() = %d, want 1", f.CallCount())
	}
}

func TestFakeEmbedding_VectorRange(t *testing.T) {
	f := NewFakeEmbedding()
	ctx := context.Background()

	vec, err := f.Embed(ctx, "test text for range validation")
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}

	// All values should be in [-1, 1] range
	for i, v := range vec {
		if v < -1.0 || v > 1.0 {
			t.Errorf("vec[%d] = %v, outside [-1, 1] range", i, v)
		}
		// Check for NaN
		if math.IsNaN(float64(v)) {
			t.Errorf("vec[%d] is NaN", i)
		}
	}
}

func TestFakeEmbedding_ThreadSafety(t *testing.T) {
	f := NewFakeEmbedding()
	ctx := context.Background()

	// Run concurrent embeddings
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 10; j++ {
				_, err := f.Embed(ctx, "concurrent test")
				if err != nil {
					t.Errorf("concurrent Embed() error: %v", err)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have 100 calls recorded
	if f.CallCount() != 100 {
		t.Errorf("CallCount() = %d, want 100", f.CallCount())
	}
}
