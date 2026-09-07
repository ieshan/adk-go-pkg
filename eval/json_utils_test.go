package eval_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
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
	t.Cleanup(func() { _ = root.Close() })

	es, err := eval.LoadEvalSetFromFile(root, "test_eval_set.json")
	if err != nil {
		t.Fatalf("LoadEvalSetFromFile failed: %v", err)
	}
	if es.EvalSetID != "test-set" {
		t.Errorf("EvalSetID = %q, want %q", es.EvalSetID, "test-set")
	}
	if len(es.EvalCases) != 1 {
		t.Errorf("len(EvalCases) = %d, want 1", len(es.EvalCases))
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
	t.Cleanup(func() { _ = root.Close() })

	es, err := eval.LoadEvalSetFromFile(root, "old_eval_set.json")
	if err != nil {
		t.Fatalf("LoadEvalSetFromFile failed: %v", err)
	}
	if len(es.EvalCases) != 1 {
		t.Errorf("len(EvalCases) = %d, want 1", len(es.EvalCases))
	}
	if len(es.EvalCases[0].Conversation) != 1 {
		t.Errorf("len(Conversation) = %d, want 1", len(es.EvalCases[0].Conversation))
	}
}

func TestLoadEvalSetFromFile_NotFound(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	_, err = eval.LoadEvalSetFromFile(root, "nonexistent/eval_set.json")
	if err == nil {
		t.Error("got nil error, want error for non-existent file")
	}
}

func TestMarshalEvalSet_RoundTrip(t *testing.T) {
	original := &eval.EvalSet{
		EvalSetID: "round-trip",
		Name:      "Round Trip",
		EvalCases: []eval.EvalCase{
			{
				EvalID: "c1",
				Conversation: []eval.Invocation{{
					UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
				}},
			},
		},
	}

	data, err := eval.MarshalEvalSet(original)
	if err != nil {
		t.Fatalf("MarshalEvalSet failed: %v", err)
	}

	es, err := eval.UnmarshalEvalSet(data)
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
	_, err := eval.UnmarshalEvalSet([]byte(`{invalid json`))
	if err == nil {
		t.Error("got nil error, want error for invalid JSON")
	}
}

// FuzzLoadEvalCase verifies that eval.UnmarshalEvalSet never panics on
// arbitrary byte input. Valid JSON should parse without error; invalid input
// should return an error (no panic).
func FuzzLoadEvalCase(f *testing.F) {
	// Seed: valid JSON eval set.
	f.Add([]byte(`{"evalSetId":"s","name":"n","evalCases":[]}`))
	// Seed: malformed JSON.
	f.Add([]byte(`invalid json`))
	// Seed: empty bytes.
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		es, err := eval.UnmarshalEvalSet(data)
		// The function must not panic — reaching here is the primary assertion.
		// Both error and non-error outcomes are acceptable as long as no panic
		// occurred.
		if err != nil {
			return
		}
		_ = es
	})
}
