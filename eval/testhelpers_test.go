package eval_test

import (
	"context"

	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// textToContent creates a genai.Content from a text string with "user" role.
// This is a local copy of the unexported eval.textToContent helper for use
// in black-box tests.
func textToContent(text string) *genai.Content {
	return &genai.Content{
		Parts: []*genai.Part{{Text: text}},
		Role:  "user",
	}
}

// fakeAgentRunner implements eval.AgentRunner for testing.
// This is a local copy for use in black-box tests; the original lives in
// agent_evaluator_test.go (package eval, white-box).
type fakeAgentRunner struct {
	events []*session.Event
	err    error
}

func (f *fakeAgentRunner) RunForSession(ctx context.Context, sessionID, userID, appName string, userContent *genai.Content) ([]*session.Event, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events, nil
}
