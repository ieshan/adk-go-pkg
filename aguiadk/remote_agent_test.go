package aguiadk

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"

	"google.golang.org/genai"
)

func TestNewRemoteAgent_ADKExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "event: RUN_STARTED\ndata: {\"type\":\"RUN_STARTED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: TEXT_MESSAGE_START\ndata: {\"type\":\"TEXT_MESSAGE_START\",\"messageId\":\"m1\",\"role\":\"assistant\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: TEXT_MESSAGE_CONTENT\ndata: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"messageId\":\"m1\",\"delta\":\"Hello from remote!\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: TEXT_MESSAGE_END\ndata: {\"type\":\"TEXT_MESSAGE_END\",\"messageId\":\"m1\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: RUN_FINISHED\ndata: {\"type\":\"RUN_FINISHED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	remAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "remote_assistant",
		Description: "Remote AG-UI agent",
		Endpoint:    server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create remote agent: %v", err)
	}

	ic := testutil.NewFakeInvocationContext().
		WithUserContent(genai.NewContentFromText("hi", "user")).
		WithContext(context.Background())
	var textReceived string
	for ev, err := range remAgent.Run(ic) {
		if err != nil {
			t.Fatalf("agent run error: %v", err)
		}
		if ev.Content != nil {
			for _, p := range ev.Content.Parts {
				if p.Text != "" {
					textReceived += p.Text
				}
			}
		}
	}

	if textReceived != "Hello from remote!" {
		t.Fatalf("expected 'Hello from remote!', got %q", textReceived)
	}
}

func TestNewRemoteAgent_MissingName(t *testing.T) {
	_, err := NewRemoteAgent(RemoteAgentConfig{Endpoint: "http://localhost"})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestNewRemoteAgent_MissingEndpoint(t *testing.T) {
	_, err := NewRemoteAgent(RemoteAgentConfig{Name: "test"})
	if err == nil {
		t.Fatal("expected error for missing endpoint")
	}
}
