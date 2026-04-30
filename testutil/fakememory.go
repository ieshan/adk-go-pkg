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
	"sync"

	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
)

// Compile-time interface check.
var _ memory.Service = (*FakeMemoryService)(nil)

// FakeMemoryService implements memory.Service for testing.
// It allows preconfigured search results and records all calls.
//
// Thread-safe.
type FakeMemoryService struct {
	mu           sync.RWMutex
	sessions     []session.Session
	searchFn     func(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error)
	addSessionFn func(ctx context.Context, s session.Session) error
	searches     []*memory.SearchRequest
	preloaded    map[string][]memory.Entry // key: "userID/appName" -> entries
}

// NewFakeMemoryService creates a FakeMemoryService.
func NewFakeMemoryService() *FakeMemoryService {
	return &FakeMemoryService{
		preloaded: make(map[string][]memory.Entry),
	}
}

// WithSearchFunc configures custom search behavior (builder pattern).
// If not set, SearchMemory returns preloaded entries or an empty response.
func (f *FakeMemoryService) WithSearchFunc(fn func(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error)) *FakeMemoryService {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchFn = fn
	return f
}

// WithAddSessionFunc configures custom AddSession behavior (builder pattern).
func (f *FakeMemoryService) WithAddSessionFunc(fn func(ctx context.Context, s session.Session) error) *FakeMemoryService {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addSessionFn = fn
	return f
}

// PreloadMemory adds preconfigured memory entries for search results.
func (f *FakeMemoryService) PreloadMemory(userID, appName string, entries ...memory.Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := userID + "/" + appName
	f.preloaded[key] = append(f.preloaded[key], entries...)
}

// AddSessionToMemory implements memory.Service.
func (f *FakeMemoryService) AddSessionToMemory(ctx context.Context, s session.Session) error {
	f.mu.Lock()
	fn := f.addSessionFn
	f.sessions = append(f.sessions, s)
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, s)
	}
	return nil
}

// SearchMemory implements memory.Service.
func (f *FakeMemoryService) SearchMemory(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
	f.mu.Lock()
	fn := f.searchFn
	f.searches = append(f.searches, req)
	key := req.UserID + "/" + req.AppName
	entries, hasPreloaded := f.preloaded[key]
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, req)
	}

	if hasPreloaded {
		return &memory.SearchResponse{Memories: entries}, nil
	}

	return &memory.SearchResponse{}, nil
}

// AddSessionCount returns the number of AddSessionToMemory calls.
func (f *FakeMemoryService) AddSessionCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.sessions)
}

// SearchCount returns the number of SearchMemory calls.
func (f *FakeMemoryService) SearchCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.searches)
}

// LastSearch returns the most recent SearchRequest, or nil.
func (f *FakeMemoryService) LastSearch() *memory.SearchRequest {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.searches) == 0 {
		return nil
	}
	return f.searches[len(f.searches)-1]
}
