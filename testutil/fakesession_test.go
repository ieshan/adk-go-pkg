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
	"testing"

	"google.golang.org/adk/session"
)

func TestFakeSession_Builder(t *testing.T) {
	s := NewFakeSession().
		WithID("s-1").
		WithAppName("my-app").
		WithUserID("u-1")

	if s.ID() != "s-1" {
		t.Errorf("ID() = %q, want %q", s.ID(), "s-1")
	}
	if s.AppName() != "my-app" {
		t.Errorf("AppName() = %q, want %q", s.AppName(), "my-app")
	}
	if s.UserID() != "u-1" {
		t.Errorf("UserID() = %q, want %q", s.UserID(), "u-1")
	}
}

func TestFakeSession_State(t *testing.T) {
	s := NewFakeSession().WithState(map[string]any{"key1": "val1"})

	val, err := s.State().Get("key1")
	if err != nil || val != "val1" {
		t.Errorf("Get(key1) = %v, %v; want val1, nil", val, err)
	}

	_, err = s.State().Get("nonexistent")
	if err != session.ErrStateKeyNotExist {
		t.Errorf("Get(nonexistent) error = %v, want ErrStateKeyNotExist", err)
	}

	if err := s.State().Set("key2", 42); err != nil {
		t.Errorf("Set(key2, 42) error = %v", err)
	}
	val, _ = s.State().Get("key2")
	if val != 42 {
		t.Errorf("Get(key2) = %v, want 42", val)
	}
}

func TestFakeSession_Events(t *testing.T) {
	e1 := NewTextEvent("user", "hello")
	e2 := NewTextEvent("model", "hi there")
	s := NewFakeSession().WithEvents(e1, e2)

	if s.Events().Len() != 2 {
		t.Errorf("Len() = %d, want 2", s.Events().Len())
	}
	if s.Events().At(0).Author != "user" {
		t.Errorf("At(0).Author = %q, want %q", s.Events().At(0).Author, "user")
	}
	if s.Events().At(99) != nil {
		t.Error("At(99) should be nil")
	}

	// AddEvent
	e3 := NewTextEvent("user", "more")
	s.AddEvent(e3)
	if s.Events().Len() != 3 {
		t.Errorf("after AddEvent, Len() = %d, want 3", s.Events().Len())
	}
}

func TestFakeState_All(t *testing.T) {
	s := NewFakeStateWithData(map[string]any{"a": 1, "b": 2})
	count := 0
	for k, v := range s.All() {
		count++
		if k == "a" && v != 1 {
			t.Errorf("All() a = %v, want 1", v)
		}
		if k == "b" && v != 2 {
			t.Errorf("All() b = %v, want 2", v)
		}
	}
	if count != 2 {
		t.Errorf("All() yielded %d items, want 2", count)
	}
}

func TestFakeEvents_All(t *testing.T) {
	e1 := NewTextEvent("user", "a")
	e2 := NewTextEvent("model", "b")
	fe := NewFakeEvents([]*session.Event{e1, e2})

	count := 0
	for e := range fe.All() {
		count++
		if e == nil {
			t.Error("All() yielded nil event")
		}
	}
	if count != 2 {
		t.Errorf("All() yielded %d events, want 2", count)
	}
}
