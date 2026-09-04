package eval

import (
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/genai"
)

func TestLoadEvalSetFromFile_NewFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test_eval_set.json")

	data := `{
		"evalSetId": "test-set",
		"name": "Test Set",
		"evalCases": [
			{
				"evalId": "case-1",
				"conversation": [
					{
						"userContent": {"role": "user", "parts": [{"text": "hello"}]},
						"finalResponse": {"role": "model", "parts": [{"text": "world"}]}
					}
				]
			}
		]
	}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	es, err := LoadEvalSetFromFile(root, "test_eval_set.json")
	if err != nil {
		t.Fatalf("LoadEvalSetFromFile failed: %v", err)
	}
	if es.EvalSetID != "test-set" {
		t.Errorf("EvalSetID = %q, want %q", es.EvalSetID, "test-set")
	}
	if len(es.EvalCases) != 1 {
		t.Fatalf("len(EvalCases) = %d, want 1", len(es.EvalCases))
	}
	if es.EvalCases[0].EvalID != "case-1" {
		t.Errorf("EvalID = %q, want %q", es.EvalCases[0].EvalID, "case-1")
	}
}

func TestLoadEvalSetFromFile_OldFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old_eval_set.json")

	data := `[
		{
			"query": "what is the weather",
			"reference": "it is sunny",
			"expected_tool_use": [
				{"toolName": "get_weather", "toolInput": {"city": "SF"}}
			]
		}
	]`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	es, err := LoadEvalSetFromFile(root, "old_eval_set.json")
	if err != nil {
		t.Fatalf("LoadEvalSetFromFile failed: %v", err)
	}
	if len(es.EvalCases) != 1 {
		t.Fatalf("len(EvalCases) = %d, want 1", len(es.EvalCases))
	}
	if len(es.EvalCases[0].Conversation) != 1 {
		t.Fatalf("len(Conversation) = %d, want 1", len(es.EvalCases[0].Conversation))
	}
}

func TestLoadEvalSetFromFile_NotFound(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	_, err = LoadEvalSetFromFile(root, "nonexistent/eval_set.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestMarshalEvalSet_RoundTrip(t *testing.T) {
	original := &EvalSet{
		EvalSetID: "round-trip",
		Name:      "Round Trip",
		EvalCases: []EvalCase{
			{
				EvalID: "c1",
				Conversation: []Invocation{{
					UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
				}},
			},
		},
	}

	data, err := MarshalEvalSet(original)
	if err != nil {
		t.Fatalf("MarshalEvalSet failed: %v", err)
	}

	es, err := UnmarshalEvalSet(data)
	if err != nil {
		t.Fatalf("UnmarshalEvalSet failed: %v", err)
	}
	if es.EvalSetID != "round-trip" {
		t.Errorf("EvalSetID = %q, want %q", es.EvalSetID, "round-trip")
	}
	if len(es.EvalCases) != 1 {
		t.Errorf("len(EvalCases) = %d, want 1", len(es.EvalCases))
	}
}

func TestUnmarshalEvalSet_InvalidJSON(t *testing.T) {
	_, err := UnmarshalEvalSet([]byte(`{invalid json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
