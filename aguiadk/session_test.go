package aguiadk_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/session"
)

func newTestManager(t *testing.T) (*aguiadk.SessionManager, session.Service) {
	t.Helper()
	svc := session.InMemoryService()
	sm := aguiadk.NewSessionManager(aguiadk.SessionManagerConfig{
		Service:         svc,
		SessionTimeout:  1 * time.Minute,
		CleanupInterval: 10 * time.Second,
	})
	t.Cleanup(sm.Stop)
	return sm, svc
}

func TestSessionManager_ResolveCreatesNew(t *testing.T) {
	sm, _ := newTestManager(t)
	ctx := context.Background()

	s1, err := sm.Resolve(ctx, "thread-1", "myapp", "user-1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if s1.ID() == "" {
		t.Fatal("expected non-empty session ID")
	}

	// Second resolve with the same thread should return the same session.
	s2, err := sm.Resolve(ctx, "thread-1", "myapp", "user-1")
	if err != nil {
		t.Fatalf("Resolve (second): %v", err)
	}
	if s1.ID() != s2.ID() {
		t.Fatalf("expected same session ID, got %q and %q", s1.ID(), s2.ID())
	}
}

func TestSessionManager_DifferentThreads(t *testing.T) {
	sm, _ := newTestManager(t)
	ctx := context.Background()

	s1, err := sm.Resolve(ctx, "thread-a", "myapp", "user-1")
	if err != nil {
		t.Fatalf("Resolve thread-a: %v", err)
	}

	s2, err := sm.Resolve(ctx, "thread-b", "myapp", "user-1")
	if err != nil {
		t.Fatalf("Resolve thread-b: %v", err)
	}

	if s1.ID() == s2.ID() {
		t.Fatalf("different threads should get different sessions, both got %q", s1.ID())
	}
}

func TestSessionManager_Concurrent(t *testing.T) {
	sm, _ := newTestManager(t)
	ctx := context.Background()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			// All goroutines resolve the same thread; should converge to one session.
			_, err := sm.Resolve(ctx, "shared-thread", "myapp", "user-1")
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent Resolve failed: %v", err)
	}
}

func TestSessionManager_Stop(t *testing.T) {
	svc := session.InMemoryService()
	sm := aguiadk.NewSessionManager(aguiadk.SessionManagerConfig{
		Service:         svc,
		SessionTimeout:  1 * time.Minute,
		CleanupInterval: 10 * time.Millisecond,
	})

	// Stop should not panic even if called quickly.
	sm.Stop()

	// Give the goroutine a moment to exit.
	time.Sleep(50 * time.Millisecond)
}
