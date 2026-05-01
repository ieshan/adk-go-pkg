package testutil

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/adk/session"
)

// Compile-time interface check.
var _ session.Service = (*FakeSessionService)(nil)

// FakeSessionService implements session.Service for testing.
// It wraps an in-memory store and records all calls for assertions.
//
// Thread-safe.
type FakeSessionService struct {
	mu       sync.RWMutex
	sessions map[string]*FakeSession // key: "appName/userID/sessionID"
	creates  []*session.CreateRequest
	gets     []*session.GetRequest
	appends  []*session.Event
	deletes  []*session.DeleteRequest
}

// NewFakeSessionService creates a FakeSessionService.
func NewFakeSessionService() *FakeSessionService {
	return &FakeSessionService{
		sessions: make(map[string]*FakeSession),
	}
}

// sessionKey builds the map key for a session.
func sessionKey(appName, userID, sessionID string) string {
	return appName + "/" + userID + "/" + sessionID
}

// PreloadSession adds a preconfigured session.
func (f *FakeSessionService) PreloadSession(sess session.Session) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := sessionKey(sess.AppName(), sess.UserID(), sess.ID())
	fs, ok := sess.(*FakeSession)
	if !ok {
		// Wrap non-FakeSession in a new FakeSession with the same data.
		fs = NewFakeSession().
			WithID(sess.ID()).
			WithAppName(sess.AppName()).
			WithUserID(sess.UserID())
	}
	f.sessions[key] = fs
}

// Create implements session.Service.
func (f *FakeSessionService) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.creates = append(f.creates, req)

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("session-%d", len(f.sessions)+1)
	}

	key := sessionKey(req.AppName, req.UserID, sessionID)
	if _, exists := f.sessions[key]; exists {
		return nil, fmt.Errorf("session already exists: %s", key)
	}

	stateData := req.State
	if stateData == nil {
		stateData = make(map[string]any)
	}

	fs := NewFakeSession().
		WithID(sessionID).
		WithAppName(req.AppName).
		WithUserID(req.UserID).
		WithState(stateData)

	f.sessions[key] = fs
	return &session.CreateResponse{Session: fs}, nil
}

// Get implements session.Service.
func (f *FakeSessionService) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.gets = append(f.gets, req)

	key := sessionKey(req.AppName, req.UserID, req.SessionID)
	fs, ok := f.sessions[key]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", key)
	}

	return &session.GetResponse{Session: fs}, nil
}

// List implements session.Service.
func (f *FakeSessionService) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	prefix := req.AppName + "/" + req.UserID + "/"
	var sessions []session.Session
	for key, fs := range f.sessions {
		if strings.HasPrefix(key, prefix) {
			sessions = append(sessions, fs)
		}
	}

	return &session.ListResponse{Sessions: sessions}, nil
}

// Delete implements session.Service.
func (f *FakeSessionService) Delete(ctx context.Context, req *session.DeleteRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.deletes = append(f.deletes, req)

	key := sessionKey(req.AppName, req.UserID, req.SessionID)
	delete(f.sessions, key)
	return nil
}

// AppendEvent implements session.Service.
// It appends the event to the session and removes temp: prefixed state keys.
func (f *FakeSessionService) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.appends = append(f.appends, event)

	key := sessionKey(sess.AppName(), sess.UserID(), sess.ID())
	fs, ok := f.sessions[key]
	if !ok {
		return fmt.Errorf("session not found: %s", key)
	}

	// Remove temporary state keys from the event's StateDelta.
	if event.Actions.StateDelta != nil {
		for k := range event.Actions.StateDelta {
			if strings.HasPrefix(k, "temp:") {
				delete(event.Actions.StateDelta, k)
			}
		}
	}

	fs.AddEvent(event)
	return nil
}

// CreateCount returns the number of Create calls.
func (f *FakeSessionService) CreateCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.creates)
}

// AppendEventCount returns the number of AppendEvent calls.
func (f *FakeSessionService) AppendEventCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.appends)
}

// LastAppendedEvent returns the most recently appended event, or nil.
func (f *FakeSessionService) LastAppendedEvent() *session.Event {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.appends) == 0 {
		return nil
	}
	return f.appends[len(f.appends)-1]
}

// GetSession returns the internal FakeSession for a given key, or nil.
func (f *FakeSessionService) GetSession(appName, userID, sessionID string) *FakeSession {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.sessions[sessionKey(appName, userID, sessionID)]
}
