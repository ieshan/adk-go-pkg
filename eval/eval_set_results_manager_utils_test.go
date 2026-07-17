package eval

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCreateEvalSetResult(t *testing.T) {
	results := []EvalCaseResult{
		{EvalID: "case-1", FinalEvalStatus: EvalStatusPassed},
	}
	result := CreateEvalSetResult("app", "test-set", results)

	if result.EvalSetID != "test-set" {
		t.Errorf("EvalSetID = %q, want test-set", result.EvalSetID)
	}
	if result.EvalSetResultName != "app_test-set" {
		t.Errorf("EvalSetResultName = %q, want app_test-set", result.EvalSetResultName)
	}
	if len(result.EvalCaseResults) != 1 {
		t.Errorf("len(EvalCaseResults) = %d, want 1", len(result.EvalCaseResults))
	}
	if result.CreationTimestamp <= 0 {
		t.Errorf("CreationTimestamp = %v, want positive", result.CreationTimestamp)
	}
	// Verify timestamp is recent.
	now := float64(time.Now().Unix())
	if now-result.CreationTimestamp > 5 {
		t.Errorf("CreationTimestamp = %v, too far from now %v", result.CreationTimestamp, now)
	}
}

func TestSanitizeEvalSetResultName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"simple_name", "simple_name"},
		{"path/to/result", "path_to_result"},
		{"back\\slash", "back_slash"},
		{"both/slash\\and", "both_slash_and"},
		{"no_changes", "no_changes"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeEvalSetResultName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeEvalSetResultName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseEvalSetResultJSON_Valid(t *testing.T) {
	data := []byte(`{
		"evalSetResultId": "result-1",
		"evalSetId": "test-set",
		"evalCaseResults": [
			{"evalId": "case-1", "finalEvalStatus": "PASSED"}
		],
		"creationTimestamp": 1234567890
	}`)

	result, err := ParseEvalSetResultJSON(data)
	if err != nil {
		t.Fatalf("ParseEvalSetResultJSON failed: %v", err)
	}
	if result.EvalSetResultID != "result-1" {
		t.Errorf("EvalSetResultID = %q, want result-1", result.EvalSetResultID)
	}
	if len(result.EvalCaseResults) != 1 {
		t.Errorf("len(EvalCaseResults) = %d, want 1", len(result.EvalCaseResults))
	}
}

func TestParseEvalSetResultJSON_DoubleEncoded(t *testing.T) {
	inner := `{"evalSetResultId":"result-1","evalSetId":"test-set","evalCaseResults":[{"evalId":"case-1","finalEvalStatus":"PASSED"}],"creationTimestamp":1234567890}`
	encoded, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	result, err := ParseEvalSetResultJSON(encoded)
	if err != nil {
		t.Fatalf("ParseEvalSetResultJSON failed: %v", err)
	}
	if result.EvalSetResultID != "result-1" {
		t.Errorf("EvalSetResultID = %q, want result-1", result.EvalSetResultID)
	}
}

func TestParseEvalSetResultJSON_Invalid(t *testing.T) {
	_, err := ParseEvalSetResultJSON([]byte("not json at all"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
