package eval_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestRubricContent_Basic(t *testing.T) {
	rc := eval.RubricContent{
		TextProperty: "response is concise",
	}

	if rc.TextProperty != "response is concise" {
		t.Errorf("got %s, want 'response is concise'", rc.TextProperty)
	}
}

func TestRubric_Basic(t *testing.T) {
	r := eval.Rubric{
		RubricID:      "rubric1",
		RubricContent: eval.RubricContent{TextProperty: "test rubric"},
	}

	if r.RubricID != "rubric1" {
		t.Errorf("got %s, want rubric1", r.RubricID)
	}
}

func TestRubricScore_Basic(t *testing.T) {
	score := 0.85
	rs := eval.RubricScore{
		RubricID: "rubric1",
		Score:    &score,
	}

	if rs.RubricID != "rubric1" {
		t.Errorf("got %s, want rubric1", rs.RubricID)
	}
	if rs.Score == nil || *rs.Score != 0.85 {
		t.Errorf("got %v, want 0.85", rs.Score)
	}
}
