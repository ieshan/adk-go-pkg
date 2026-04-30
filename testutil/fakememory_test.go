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
	"errors"
	"testing"

	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
)

func TestFakeMemoryService_AddSession(t *testing.T) {
	svc := NewFakeMemoryService()
	sess := NewFakeSession().WithAppName("app").WithUserID("user")

	err := svc.AddSessionToMemory(context.Background(), sess)
	if err != nil {
		t.Fatalf("AddSessionToMemory() error = %v", err)
	}
	if svc.AddSessionCount() != 1 {
		t.Errorf("AddSessionCount() = %d, want 1", svc.AddSessionCount())
	}
}

func TestFakeMemoryService_SearchPreloaded(t *testing.T) {
	svc := NewFakeMemoryService()
	svc.PreloadMemory("user1", "app1",
		NewMemoryEntry("e1", "hello world", "model"),
		NewMemoryEntry("e2", "goodbye world", "model"),
	)

	resp, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{
		Query:   "hello",
		UserID:  "user1",
		AppName: "app1",
	})
	if err != nil {
		t.Fatalf("SearchMemory() error = %v", err)
	}
	if len(resp.Memories) != 2 {
		t.Errorf("SearchMemory() returned %d entries, want 2", len(resp.Memories))
	}
}

func TestFakeMemoryService_SearchEmpty(t *testing.T) {
	svc := NewFakeMemoryService()
	resp, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{
		Query:   "hello",
		UserID:  "user1",
		AppName: "app1",
	})
	if err != nil {
		t.Fatalf("SearchMemory() error = %v", err)
	}
	if len(resp.Memories) != 0 {
		t.Errorf("SearchMemory() empty should return 0 entries, got %d", len(resp.Memories))
	}
}

func TestFakeMemoryService_WithSearchFunc(t *testing.T) {
	svc := NewFakeMemoryService().WithSearchFunc(
		func(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
			return nil, errors.New("search failed")
		},
	)

	_, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{
		Query: "test", UserID: "u", AppName: "a",
	})
	if err == nil || err.Error() != "search failed" {
		t.Errorf("SearchMemory() error = %v, want 'search failed'", err)
	}
}

func TestFakeMemoryService_WithAddSessionFunc(t *testing.T) {
	svc := NewFakeMemoryService().WithAddSessionFunc(
		func(ctx context.Context, s session.Session) error {
			return errors.New("add failed")
		},
	)

	sess := NewFakeSession()
	err := svc.AddSessionToMemory(context.Background(), sess)
	if err == nil || err.Error() != "add failed" {
		t.Errorf("AddSessionToMemory() error = %v, want 'add failed'", err)
	}
}

func TestFakeMemoryService_CallTracking(t *testing.T) {
	svc := NewFakeMemoryService()
	svc.SearchMemory(context.Background(), &memory.SearchRequest{
		Query: "q1", UserID: "u", AppName: "a",
	})
	svc.SearchMemory(context.Background(), &memory.SearchRequest{
		Query: "q2", UserID: "u", AppName: "a",
	})

	if svc.SearchCount() != 2 {
		t.Errorf("SearchCount() = %d, want 2", svc.SearchCount())
	}
	if svc.LastSearch().Query != "q2" {
		t.Errorf("LastSearch().Query = %q, want %q", svc.LastSearch().Query, "q2")
	}
}
