package aguiadk

import (
	"context"
	"sync"
	"time"

	"google.golang.org/adk/session"
)

// SessionManagerConfig controls session lifecycle.
type SessionManagerConfig struct {
	Service         session.Service
	SessionTimeout  time.Duration // Default: 20 minutes
	CleanupInterval time.Duration // Default: 5 minutes
}

// SessionManager maps AG-UI thread IDs to ADK session IDs.
// It is safe for concurrent use.
type SessionManager struct {
	cfg     SessionManagerConfig
	mu      sync.RWMutex
	threads map[string]threadEntry // threadID -> entry
	done    chan struct{}
	wg      sync.WaitGroup
}

type threadEntry struct {
	sessionID string
	appName   string
	userID    string
	lastUsed  time.Time
}

// NewSessionManager creates a SessionManager and starts a background cleanup
// goroutine. Call Stop to release resources.
func NewSessionManager(cfg SessionManagerConfig) *SessionManager {
	if cfg.SessionTimeout == 0 {
		cfg.SessionTimeout = 20 * time.Minute
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = 5 * time.Minute
	}
	sm := &SessionManager{
		cfg:     cfg,
		threads: make(map[string]threadEntry),
		done:    make(chan struct{}),
	}
	sm.wg.Add(1)
	go sm.cleanupLoop()
	return sm
}

// Resolve returns the ADK session for the given AG-UI thread. If no session
// exists for the thread, a new one is created via the configured Service.
func (m *SessionManager) Resolve(ctx context.Context, threadID, appName, userID string) (session.Session, error) {
	// Fast path: read lock
	m.mu.RLock()
	entry, ok := m.threads[threadID]
	m.mu.RUnlock()

	if ok {
		// Update last used time
		m.mu.Lock()
		entry.lastUsed = time.Now()
		m.threads[threadID] = entry
		m.mu.Unlock()

		resp, err := m.cfg.Service.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: entry.sessionID,
		})
		if err == nil {
			return resp.Session, nil
		}
		// If get failed, fall through to create new session
	}

	// Slow path: write lock, double-check, create
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if entry, ok := m.threads[threadID]; ok {
		resp, err := m.cfg.Service.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: entry.sessionID,
		})
		if err == nil {
			entry.lastUsed = time.Now()
			m.threads[threadID] = entry
			return resp.Session, nil
		}
	}

	// Create new session
	createResp, err := m.cfg.Service.Create(ctx, &session.CreateRequest{
		AppName: appName,
		UserID:  userID,
		State: map[string]any{
			"_ag_ui_thread_id": threadID,
			"_ag_ui_app_name":  appName,
			"_ag_ui_user_id":   userID,
		},
	})
	if err != nil {
		return nil, err
	}

	m.threads[threadID] = threadEntry{
		sessionID: createResp.Session.ID(),
		appName:   appName,
		userID:    userID,
		lastUsed:  time.Now(),
	}

	return createResp.Session, nil
}

// Stop signals the background cleanup goroutine to exit and waits for it
// to finish (with a 5-second timeout).
func (m *SessionManager) Stop() {
	close(m.done)
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func (m *SessionManager) cleanupLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

func (m *SessionManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for threadID, entry := range m.threads {
		if now.Sub(entry.lastUsed) > m.cfg.SessionTimeout {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			// Best-effort cleanup — delete errors are non-fatal since sessions
			// will naturally expire and be recreated on next Resolve.
			_ = m.cfg.Service.Delete(ctx, &session.DeleteRequest{
				AppName:   entry.appName,
				UserID:    entry.userID,
				SessionID: entry.sessionID,
			})
			cancel()
			delete(m.threads, threadID)
		}
	}
}
