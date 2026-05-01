package testutil

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sync"
)

// FakeEmbeddingFunc is the function signature for embedding providers.
// It matches the embedding function type used in adk-go-memory and other
// packages that require text-to-vector conversion.
type FakeEmbeddingFunc func(ctx context.Context, text string) ([]float32, error)

// FakeEmbedding implements a deterministic embedding function for testing.
// It generates reproducible 1536-dimensional float32 vectors from text using
// SHA-256 hashing. This is suitable for testing semantic search, memory
// systems, and any code that requires embeddings without calling external APIs.
//
// The embedding function is thread-safe and records all calls for assertions.
//
// Example usage:
//
//	fake := NewFakeEmbedding()
//	vec, err := fake.Embed(ctx, "hello world")
//	if err != nil {
//	    t.Fatal(err)
//	}
//	if len(vec) != 1536 {
//	    t.Errorf("expected 1536 dimensions, got %d", len(vec))
//	}
//
//	// Use in a memory kit configuration:
//	kit, err := memory.New(memory.KitConfig{
//	    Storage:       storage,
//	    EmbeddingFunc: fake.Embed,
//	})
//
// Thread-safe.
type FakeEmbedding struct {
	mu         sync.RWMutex
	dimension  int
	err        error
	calls      []string
	embeddings map[string][]float32 // pre-configured embeddings for specific texts
}

// NewFakeEmbedding creates a FakeEmbedding with default 1536 dimensions.
// The dimension matches OpenAI's text-embedding-ada-002 and text-embedding-3-* models.
func NewFakeEmbedding() *FakeEmbedding {
	return &FakeEmbedding{
		dimension:  1536,
		embeddings: make(map[string][]float32),
	}
}

// WithDimension sets a custom vector dimension (builder pattern).
// Default is 1536. Some models use different dimensions (e.g., 768, 384).
func (f *FakeEmbedding) WithDimension(dim int) *FakeEmbedding {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dimension = dim
	return f
}

// WithPrecomputedEmbedding configures a specific embedding for a text (builder pattern).
// When Embed is called with this exact text, the precomputed vector is returned.
// The embedding is copied defensively to prevent external modifications.
func (f *FakeEmbedding) WithPrecomputedEmbedding(text string, embedding []float32) *FakeEmbedding {
	f.mu.Lock()
	defer f.mu.Unlock()
	copied := make([]float32, len(embedding))
	copy(copied, embedding)
	f.embeddings[text] = copied
	return f
}

// Embed generates a deterministic embedding vector for the given text.
// Returns a preconfigured embedding if one was set for this exact text,
// otherwise generates a deterministic vector from the text using SHA-256.
//
// If SetError was called, returns the configured error instead.
func (f *FakeEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	f.mu.Lock()
	if f.err != nil {
		err := f.err
		f.calls = append(f.calls, text)
		f.mu.Unlock()
		return nil, err
	}

	// Check for preconfigured embedding
	if precomputed, ok := f.embeddings[text]; ok {
		f.calls = append(f.calls, text)
		f.mu.Unlock()
		result := make([]float32, len(precomputed))
		copy(result, precomputed)
		return result, nil
	}

	f.calls = append(f.calls, text)
	dim := f.dimension
	f.mu.Unlock()

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Generate deterministic embedding from text
	return generateDeterministicEmbedding(text, dim), nil
}

// FakeEmbed generates a deterministic 1536-dim float32 vector from text.
// This is a convenience function for simple test cases that don't need the full
// FakeEmbedding struct capabilities. Uses SHA-256 hash for reproducibility.
//
// Example:
//
//	obs := &adapter.Observation{
//	    Embedding: testutil.FakeEmbed("test content"),
//	}
func FakeEmbed(text string) []float32 {
	return generateDeterministicEmbedding(text, 1536)
}

// generateDeterministicEmbedding creates a reproducible float32 vector from text.
// Uses SHA-256 hash of the text to seed the vector values.
func generateDeterministicEmbedding(text string, dim int) []float32 {
	hash := sha256.Sum256([]byte(text))
	vec := make([]float32, dim)

	// Use hash bytes to generate vector components
	for i := 0; i < dim; i++ {
		// Get 4 bytes for each float32
		idx := (i * 4) % len(hash)
		val := binary.BigEndian.Uint32(hash[idx:])
		// Normalize to [-1, 1] range
		vec[i] = (float32(val)/float32(math.MaxUint32))*2 - 1
	}

	return vec
}

// SetError configures the fake to return an error on subsequent calls.
// The error persists until ClearError is called.
func (f *FakeEmbedding) SetError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// ClearError removes the configured error, allowing normal embedding generation.
func (f *FakeEmbedding) ClearError() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = nil
}

// CallCount returns the number of Embed calls made.
func (f *FakeEmbedding) CallCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.calls)
}

// Calls returns a copy of all texts that were embedded.
func (f *FakeEmbedding) Calls() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make([]string, len(f.calls))
	copy(result, f.calls)
	return result
}

// LastCall returns the most recent text that was embedded, or empty string if none.
func (f *FakeEmbedding) LastCall() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.calls) == 0 {
		return ""
	}
	return f.calls[len(f.calls)-1]
}

// Reset clears all call history and errors. Preconfigured embeddings are kept.
func (f *FakeEmbedding) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
	f.err = nil
}

// Dimension returns the configured vector dimension.
func (f *FakeEmbedding) Dimension() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.dimension
}

// AsFunc returns the Embed method as a FakeEmbeddingFunc function type.
// This allows using FakeEmbedding directly where an embedding function is required.
//
// Example:
//
//	fake := testutil.NewFakeEmbedding()
//	kit, err := memory.New(memory.KitConfig{
//	    Storage:       storage,
//	    EmbeddingFunc: fake.AsFunc(),
//	})
func (f *FakeEmbedding) AsFunc() FakeEmbeddingFunc {
	return f.Embed
}
