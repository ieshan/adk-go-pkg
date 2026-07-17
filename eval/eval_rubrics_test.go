package eval

import (
	"testing"
)

func TestRubricContent_Basic(t *testing.T) {
	rc := RubricContent{
		TextProperty: "response is concise",
	}

	if rc.TextProperty != "response is concise" {
		t.Errorf("got %s, want 'response is concise'", rc.TextProperty)
	}
}

func TestRubric_Basic(t *testing.T) {
	r := Rubric{
		RubricID:      "rubric1",
		RubricContent: RubricContent{TextProperty: "test rubric"},
	}

	if r.RubricID != "rubric1" {
		t.Errorf("got %s, want rubric1", r.RubricID)
	}
}

func TestRubricScore_Basic(t *testing.T) {
	score := 0.85
	rs := RubricScore{
		RubricID: "rubric1",
		Score:    &score,
	}

	if rs.RubricID != "rubric1" {
		t.Errorf("got %s, want rubric1", rs.RubricID)
	}
	if rs.Score == nil || *rs.Score != 0.85 {
		t.Error("expected score 0.85")
	}
}
