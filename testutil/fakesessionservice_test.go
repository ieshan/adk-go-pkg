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
	"testing"

	"google.golang.org/adk/session"
)

func TestFakeSessionService_CreateAndGet(t *testing.T) {
	svc := NewFakeSessionService()
	ctx := context.Background()

	// Create
	createResp, err := svc.Create(ctx, &session.CreateRequest{
		AppName:   "app",
		UserID:    "user1",
		SessionID: "sess1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if createResp.Session.ID() != "sess1" {
		t.Errorf("Create() session ID = %q, want %q", createResp.Session.ID(), "sess1")
	}

	// Get
	getResp, err := svc.Get(ctx, &session.GetRequest{
		AppName: "app", UserID: "user1", SessionID: "sess1",
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if getResp.Session.ID() != "sess1" {
		t.Errorf("Get() session ID = %q, want %q", getResp.Session.ID(), "sess1")
	}
}

func TestFakeSessionService_CreateDuplicate(t *testing.T) {
	svc := NewFakeSessionService()
	ctx := context.Background()

	svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "user1", SessionID: "sess1",
	})
	_, err := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "user1", SessionID: "sess1",
	})
	if err == nil {
		t.Error("Create() duplicate should return error")
	}
}

func TestFakeSessionService_GetNotFound(t *testing.T) {
	svc := NewFakeSessionService()
	_, err := svc.Get(context.Background(), &session.GetRequest{
		AppName: "app", UserID: "user1", SessionID: "missing",
	})
	if err == nil {
		t.Error("Get() missing session should return error")
	}
}

func TestFakeSessionService_Delete(t *testing.T) {
	svc := NewFakeSessionService()
	ctx := context.Background()

	svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "user1", SessionID: "sess1",
	})

	err := svc.Delete(ctx, &session.DeleteRequest{
		AppName: "app", UserID: "user1", SessionID: "sess1",
	})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = svc.Get(ctx, &session.GetRequest{
		AppName: "app", UserID: "user1", SessionID: "sess1",
	})
	if err == nil {
		t.Error("Get() after Delete should fail")
	}
}

func TestFakeSessionService_List(t *testing.T) {
	svc := NewFakeSessionService()
	ctx := context.Background()

	svc.Create(ctx, &session.CreateRequest{AppName: "app", UserID: "user1", SessionID: "s1"})
	svc.Create(ctx, &session.CreateRequest{AppName: "app", UserID: "user1", SessionID: "s2"})
	svc.Create(ctx, &session.CreateRequest{AppName: "app", UserID: "user2", SessionID: "s3"})

	listResp, err := svc.List(ctx, &session.ListRequest{
		AppName: "app", UserID: "user1",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listResp.Sessions) != 2 {
		t.Errorf("List() for user1 returned %d sessions, want 2", len(listResp.Sessions))
	}
}

func TestFakeSessionService_AppendEvent(t *testing.T) {
	svc := NewFakeSessionService()
	ctx := context.Background()

	createResp, _ := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "user1", SessionID: "sess1",
	})

	event := NewTextEvent("model", "hello")
	err := svc.AppendEvent(ctx, createResp.Session, event)
	if err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	if svc.AppendEventCount() != 1 {
		t.Errorf("AppendEventCount() = %d, want 1", svc.AppendEventCount())
	}

	// Verify event was added to the session.
	fs := svc.GetSession("app", "user1", "sess1")
	if fs.Events().Len() != 1 {
		t.Errorf("session events after AppendEvent = %d, want 1", fs.Events().Len())
	}
}

func TestFakeSessionService_AppendEventTempKeyRemoval(t *testing.T) {
	svc := NewFakeSessionService()
	ctx := context.Background()

	createResp, _ := svc.Create(ctx, &session.CreateRequest{
		AppName: "app", UserID: "user1", SessionID: "sess1",
	})

	event := NewTextEvent("model", "hello")
	event.Actions.StateDelta = map[string]any{
		"temp:cache": "will be removed",
		"persistent": "kept",
	}
	err := svc.AppendEvent(ctx, createResp.Session, event)
	if err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	// temp: prefixed keys should be removed from StateDelta.
	if _, ok := event.Actions.StateDelta["temp:cache"]; ok {
		t.Error("temp: prefixed key should be removed from StateDelta")
	}
	if _, ok := event.Actions.StateDelta["persistent"]; !ok {
		t.Error("persistent key should remain in StateDelta")
	}
}

func TestFakeSessionService_CallTracking(t *testing.T) {
	svc := NewFakeSessionService()
	ctx := context.Background()

	svc.Create(ctx, &session.CreateRequest{AppName: "app", UserID: "user1", SessionID: "s1"})

	if svc.CreateCount() != 1 {
		t.Errorf("CreateCount() = %d, want 1", svc.CreateCount())
	}
}

func TestFakeSessionService_PreloadSession(t *testing.T) {
	svc := NewFakeSessionService()
	fs := NewFakeSession().WithID("pre-seed").WithAppName("app").WithUserID("user1")
	svc.PreloadSession(fs)

	getResp, err := svc.Get(context.Background(), &session.GetRequest{
		AppName: "app", UserID: "user1", SessionID: "pre-seed",
	})
	if err != nil {
		t.Fatalf("Get() preloaded error = %v", err)
	}
	if getResp.Session.ID() != "pre-seed" {
		t.Errorf("Get() preloaded ID = %q, want %q", getResp.Session.ID(), "pre-seed")
	}
}
