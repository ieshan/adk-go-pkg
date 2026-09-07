package eval_test

import (
	"encoding/json"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"google.golang.org/genai"
)

func TestInvocationUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name: "valid invocation with user content",
			json: `{
				"userContent": {"role": "user", "parts": [{"text": "hello"}]},
				"finalResponse": {"role": "model", "parts": [{"text": "hi"}]}
			}`,
			wantErr: false,
		},
		{
			name:    "empty json",
			json:    `{}`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var inv eval.Invocation
			err := json.Unmarshal([]byte(tt.json), &inv)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvalCaseUnmarshalJSON_XORValidation(t *testing.T) {
	userContent := `"userContent":{"role":"user","parts":[{"text":"hi"}]}`
	scenario := `"conversationScenario":{"startingPrompt":"test"}`

	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name:    "both conversation and scenario",
			json:    `{"evalId":"e1","conversation":[{"` + userContent + `}],` + scenario + `}`,
			wantErr: true,
		},
		{
			name:    "neither conversation nor scenario",
			json:    `{"evalId":"e1"}`,
			wantErr: true,
		},
		{
			name:    "only conversation",
			json:    `{"evalId":"e1","conversation":[{` + userContent + `}]}`,
			wantErr: false,
		},
		{
			name:    "only scenario",
			json:    `{"evalId":"e1",` + scenario + `}`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ec eval.EvalCase
			err := json.Unmarshal([]byte(tt.json), &ec)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvalSetJSONRoundTrip(t *testing.T) {
	original := &eval.EvalSet{
		EvalSetID: "test-set",
		Name:      "Test Set",
		EvalCases: []eval.EvalCase{
			{
				EvalID: "case-1",
				Conversation: []eval.Invocation{
					{
						UserContent:   &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hello"}}},
						FinalResponse: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "world"}}},
					},
				},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded eval.EvalSet
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.EvalSetID != original.EvalSetID {
		t.Errorf("EvalSetID = %q, want %q", decoded.EvalSetID, original.EvalSetID)
	}
	if len(decoded.EvalCases) != 1 {
		t.Errorf("len(EvalCases) = %d, want 1", len(decoded.EvalCases))
	}
	if decoded.EvalCases[0].EvalID != "case-1" {
		t.Errorf("EvalID = %q, want %q", decoded.EvalCases[0].EvalID, "case-1")
	}
}

func TestNewEvalSet(t *testing.T) {
	es := eval.NewEvalSet("my-set")
	if es.EvalSetID != "my-set" {
		t.Errorf("EvalSetID = %q, want %q", es.EvalSetID, "my-set")
	}
	if es.Name != "my-set" {
		t.Errorf("Name = %q, want %q", es.Name, "my-set")
	}
	if len(es.EvalCases) != 0 {
		t.Errorf("len(EvalCases) = %d, want 0", len(es.EvalCases))
	}
	if es.CreationTimestamp <= 0 {
		t.Error("CreationTimestamp should be positive")
	}
}

func TestSessionStateTypeAlias(t *testing.T) {
	var s = eval.SessionState(map[string]any{"key": "value"})
	if s["key"] != "value" {
		t.Error("SessionState map not working")
	}
}
