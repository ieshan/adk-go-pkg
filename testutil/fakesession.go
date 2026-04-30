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
	"iter"
	"sync"
	"time"

	"google.golang.org/adk/session"
)

// Compile-time interface checks.
var _ session.Session = (*FakeSession)(nil)
var _ session.State = (*FakeState)(nil)
var _ session.Events = (*FakeEvents)(nil)

// FakeSession implements session.Session for testing.
// Thread-safe.
type FakeSession struct {
	mu         sync.RWMutex
	idVal      string
	appNameVal string
	userIDVal  string
	stateVal   *FakeState
	eventsVal  *FakeEvents
	lastUpdate time.Time
}

// NewFakeSession creates a FakeSession with sensible defaults.
func NewFakeSession() *FakeSession {
	return &FakeSession{
		idVal:      "test-session",
		appNameVal: "test-app",
		userIDVal:  "test-user",
		stateVal:   NewFakeState(),
		eventsVal:  NewFakeEvents(nil),
		lastUpdate: time.Now(),
	}
}

// WithID sets the session ID (builder pattern).
func (f *FakeSession) WithID(id string) *FakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idVal = id
	return f
}

// WithAppName sets the app name (builder pattern).
func (f *FakeSession) WithAppName(name string) *FakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.appNameVal = name
	return f
}

// WithUserID sets the user ID (builder pattern).
func (f *FakeSession) WithUserID(id string) *FakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.userIDVal = id
	return f
}

// WithState sets the initial state data (builder pattern).
func (f *FakeSession) WithState(data map[string]any) *FakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stateVal = NewFakeStateWithData(data)
	return f
}

// WithEvents adds events (builder pattern).
func (f *FakeSession) WithEvents(events ...*session.Event) *FakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.eventsVal = NewFakeEvents(events)
	return f
}

// WithLastUpdateTime sets the last update time (builder pattern).
func (f *FakeSession) WithLastUpdateTime(t time.Time) *FakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastUpdate = t
	return f
}

// ID implements session.Session.
func (f *FakeSession) ID() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.idVal
}

// AppName implements session.Session.
func (f *FakeSession) AppName() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.appNameVal
}

// UserID implements session.Session.
func (f *FakeSession) UserID() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.userIDVal
}

// State implements session.Session.
func (f *FakeSession) State() session.State {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.stateVal
}

// Events implements session.Session.
func (f *FakeSession) Events() session.Events {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.eventsVal
}

// LastUpdateTime implements session.Session.
func (f *FakeSession) LastUpdateTime() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastUpdate
}

// AddEvent appends an event and updates lastUpdateTime.
func (f *FakeSession) AddEvent(e *session.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.eventsVal.Add(e)
	f.lastUpdate = time.Now()
}

// ---------------------------------------------------------------------------
// FakeState
// ---------------------------------------------------------------------------

// FakeState implements session.State for testing.
// Thread-safe.
type FakeState struct {
	mu   sync.RWMutex
	Data map[string]any
}

// NewFakeState creates a FakeState with an empty data map.
func NewFakeState() *FakeState {
	return NewFakeStateWithData(make(map[string]any))
}

// NewFakeStateWithData creates a FakeState with the given initial data.
// The data map is copied; subsequent mutations to the input map do not affect
// the FakeState.
func NewFakeStateWithData(data map[string]any) *FakeState {
	if data == nil {
		data = make(map[string]any)
	}
	copied := make(map[string]any, len(data))
	for k, v := range data {
		copied[k] = v
	}
	return &FakeState{Data: copied}
}

// Get implements session.State.
// Returns session.ErrStateKeyNotExist if the key does not exist.
func (f *FakeState) Get(key string) (any, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	val, ok := f.Data[key]
	if !ok {
		return nil, session.ErrStateKeyNotExist
	}
	return val, nil
}

// Set implements session.State.
func (f *FakeState) Set(key string, val any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Data[key] = val
	return nil
}

// All implements session.State.
func (f *FakeState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		f.mu.RLock()
		defer f.mu.RUnlock()
		for k, v := range f.Data {
			if !yield(k, v) {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// FakeEvents
// ---------------------------------------------------------------------------

// FakeEvents implements session.Events for testing.
type FakeEvents struct {
	mu     sync.RWMutex
	events []*session.Event
}

// NewFakeEvents creates a FakeEvents with the given events.
func NewFakeEvents(events []*session.Event) *FakeEvents {
	if events == nil {
		events = []*session.Event{}
	}
	return &FakeEvents{events: events}
}

// All implements session.Events.
func (f *FakeEvents) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		f.mu.RLock()
		defer f.mu.RUnlock()
		for _, e := range f.events {
			if !yield(e) {
				return
			}
		}
	}
}

// Len implements session.Events.
func (f *FakeEvents) Len() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.events)
}

// At implements session.Events.
func (f *FakeEvents) At(i int) *session.Event {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if i < 0 || i >= len(f.events) {
		return nil
	}
	return f.events[i]
}

// Add appends an event.
func (f *FakeEvents) Add(e *session.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
}
