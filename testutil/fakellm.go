// Copyright 2025 ieshan
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutil

import (
	"context"
	"iter"
	"sync"

	"google.golang.org/adk/model"
)

// Compile-time interface check.
var _ model.LLM = (*FakeLLM)(nil)

// FakeLLM implements model.LLM for deterministic testing.
// It returns preconfigured responses in order and captures all requests for
// assertions. When the response queue is exhausted, the last response is
// repeated.
//
// Streaming behavior:
//   - When stream=true: yields each queued response with Partial=true except
//     the final response which has TurnComplete=true and Partial=false.
//   - When stream=false: yields a single response with TurnComplete=true.
//
// Thread-safe.
type FakeLLM struct {
	mu        sync.RWMutex
	name      string
	responses []model.LLMResponse
	calls     []*model.LLMRequest
	err       error
	callIndex int
}

// NewFakeLLM creates a FakeLLM with preconfigured responses.
// If no responses are provided, a default empty text response is added.
func NewFakeLLM(responses ...model.LLMResponse) *FakeLLM {
	if len(responses) == 0 {
		responses = []model.LLMResponse{NewTextResponse("")}
	}
	return &FakeLLM{
		name:      "fake-llm",
		responses: responses,
	}
}

// WithName sets the model name (builder pattern). Defaults to "fake-llm".
func (f *FakeLLM) WithName(name string) *FakeLLM {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.name = name
	return f
}

// Name implements model.LLM.
func (f *FakeLLM) Name() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.name
}

// GenerateContent implements model.LLM.
//
// When stream=true: yields each queued response with Partial=true except the
// last which has TurnComplete=true and Partial=false.
// When stream=false: yields a single response with TurnComplete=true.
//
// If SetError was called, yields (nil, error) instead.
// Each call records the request in Calls.
func (f *FakeLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		f.mu.Lock()
		if f.err != nil {
			err := f.err
			f.calls = append(f.calls, req)
			f.mu.Unlock()
			yield(nil, err)
			return
		}

		f.calls = append(f.calls, req)
		idx := f.callIndex
		if idx >= len(f.responses) {
			idx = len(f.responses) - 1
		}
		f.callIndex++
		responses := make([]model.LLMResponse, len(f.responses))
		copy(responses, f.responses)
		f.mu.Unlock()

		if stream {
			for i, resp := range responses {
				select {
				case <-ctx.Done():
					return
				default:
				}

				r := resp // copy
				if i < len(responses)-1 {
					r.Partial = true
					r.TurnComplete = false
				} else {
					r.Partial = false
					r.TurnComplete = true
				}
				if !yield(&r, nil) {
					return
				}
			}
			return
		}

		// Non-streaming: return the response at the current call index.
		r := responses[idx] // copy
		r.Partial = false
		r.TurnComplete = true
		yield(&r, nil)
	}
}

// AddResponse appends a response to the queue.
func (f *FakeLLM) AddResponse(resp model.LLMResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses = append(f.responses, resp)
}

// SetError configures the fake to return an error on the next call.
// After the error is returned, subsequent calls will also return the error
// until ClearError is called.
func (f *FakeLLM) SetError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// ClearError removes the configured error, allowing normal responses again.
func (f *FakeLLM) ClearError() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = nil
}

// CallCount returns the number of calls made to GenerateContent.
func (f *FakeLLM) CallCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.calls)
}

// LastCall returns the most recent call (or nil if none).
func (f *FakeLLM) LastCall() *model.LLMRequest {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1]
}

// CallsAt returns the call at index i (or nil if out of range).
func (f *FakeLLM) CallsAt(i int) *model.LLMRequest {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if i < 0 || i >= len(f.calls) {
		return nil
	}
	return f.calls[i]
}

// Reset clears all calls and resets the call index. Responses are kept.
func (f *FakeLLM) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
	f.callIndex = 0
	f.err = nil
}
