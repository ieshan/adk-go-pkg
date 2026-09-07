package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/memory"
	"google.golang.org/adk/v2/session"
)

var errSearchFailed = errors.New("search failed")
var errAddFailed = errors.New("add failed")

func TestFakeMemoryService_AddSession(t *testing.T) {
	svc := testutil.NewFakeMemoryService()
	sess := testutil.NewFakeSession().WithAppName("app").WithUserID("user")

	err := svc.AddSessionToMemory(context.Background(), sess)
	if err != nil {
		t.Fatalf("AddSessionToMemory() error = %v", err)
	}
	if svc.AddSessionCount() != 1 {
		t.Errorf("AddSessionCount() = %d, want 1", svc.AddSessionCount())
	}
}

func TestFakeMemoryService_SearchPreloaded(t *testing.T) {
	svc := testutil.NewFakeMemoryService()
	svc.PreloadMemory("user1", "app1",
		testutil.NewMemoryEntry("e1", "hello world", "model"),
		testutil.NewMemoryEntry("e2", "goodbye world", "model"),
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
	svc := testutil.NewFakeMemoryService()
	resp, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{
		Query:   "hello",
		UserID:  "user1",
		AppName: "app1",
	})
	if err != nil {
		t.Fatalf("SearchMemory() error = %v", err)
	}
	if len(resp.Memories) != 0 {
		t.Errorf("got %d entries, want 0", len(resp.Memories))
	}
}

func TestFakeMemoryService_WithSearchFunc(t *testing.T) {
	svc := testutil.NewFakeMemoryService().WithSearchFunc(
		func(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
			return nil, errSearchFailed
		},
	)

	_, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{
		Query: "test", UserID: "u", AppName: "a",
	})
	if !errors.Is(err, errSearchFailed) {
		t.Errorf("SearchMemory() error = %v, want errSearchFailed", err)
	}
}

func TestFakeMemoryService_WithAddSessionFunc(t *testing.T) {
	svc := testutil.NewFakeMemoryService().WithAddSessionFunc(
		func(ctx context.Context, s session.Session) error {
			return errAddFailed
		},
	)

	sess := testutil.NewFakeSession()
	err := svc.AddSessionToMemory(context.Background(), sess)
	if !errors.Is(err, errAddFailed) {
		t.Errorf("AddSessionToMemory() error = %v, want errAddFailed", err)
	}
}

func TestFakeMemoryService_CallTracking(t *testing.T) {
	svc := testutil.NewFakeMemoryService()
	if _, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{
		Query: "q1", UserID: "u", AppName: "a",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{
		Query: "q2", UserID: "u", AppName: "a",
	}); err != nil {
		t.Fatal(err)
	}

	if svc.SearchCount() != 2 {
		t.Errorf("SearchCount() = %d, want 2", svc.SearchCount())
	}
	lastSearch := svc.LastSearch()
	if lastSearch == nil {
		t.Fatal("nil search")
	}
	if lastSearch.Query != "q2" {
		t.Errorf("LastSearch().Query = %q, want %q", lastSearch.Query, "q2")
	}
}
