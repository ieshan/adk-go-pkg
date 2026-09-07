package eval_test

import (
	"encoding/json"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestParseMatchType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  eval.MatchType
	}{
		{"EXACT", eval.MatchExact},
		{"exact", eval.MatchExact},
		{"IN_ORDER", eval.MatchInOrder},
		{"in-order", eval.MatchInOrder},
		{"INORDER", eval.MatchInOrder},
		{"inorder", eval.MatchInOrder},
		{"IN ORDER", eval.MatchInOrder},
		{"ANY_ORDER", eval.MatchAnyOrder},
		{"any-order", eval.MatchAnyOrder},
		{"ANYORDER", eval.MatchAnyOrder},
		{"anyorder", eval.MatchAnyOrder},
		{"ANY ORDER", eval.MatchAnyOrder},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, err := eval.ParseMatchType(tt.input)
			if err != nil {
				t.Errorf("ParseMatchType(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseMatchType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		_, err := eval.ParseMatchType("invalid")
		if err == nil {
			t.Errorf("got nil error, want non-nil error for invalid match type")
		}
	})
}

func TestDefaultJudgeModelOptions(t *testing.T) {
	t.Parallel()
	opts := eval.DefaultJudgeModelOptions()
	if opts.JudgeModel != "gemini-2.5-flash" {
		t.Errorf("JudgeModel = %q, want %q", opts.JudgeModel, "gemini-2.5-flash")
	}
	if opts.NumSamples != 5 {
		t.Errorf("NumSamples = %d, want 5", opts.NumSamples)
	}
}

func TestEvalMetric_MarshalUnmarshal(t *testing.T) {
	t.Parallel()
	threshold := 0.8
	metric := eval.EvalMetric{
		MetricName: "tool_trajectory_avg_score",
		Threshold:  &threshold,
		Criterion: &eval.ToolTrajectoryCriterion{
			BaseCriterionImpl: eval.BaseCriterionImpl{Threshold: &threshold},
			MatchType:         eval.MatchExact,
		},
	}

	data, err := json.Marshal(metric)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var unmarshaled eval.EvalMetric
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if unmarshaled.MetricName != metric.MetricName {
		t.Errorf("MetricName = %q, want %q", unmarshaled.MetricName, metric.MetricName)
	}
	if unmarshaled.Criterion == nil {
		t.Fatal("Criterion is nil after unmarshal")
	}
	tc, ok := unmarshaled.Criterion.(*eval.ToolTrajectoryCriterion)
	if !ok {
		t.Fatalf("Criterion type = %T, want *ToolTrajectoryCriterion", unmarshaled.Criterion)
	}
	if tc.MatchType != eval.MatchExact {
		t.Errorf("MatchType = %v, want %v", tc.MatchType, eval.MatchExact)
	}
}

func TestEvalMetric_UnmarshalCriterionTypes(t *testing.T) {
	t.Parallel()
	t.Run("rubrics_based", func(t *testing.T) {
		t.Parallel()
		data := `{"metricName":"rubric_based_final_response_quality_v1","criterion":{"rubrics":[{"rubricId":"r1"}]}}`
		var m eval.EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if _, ok := m.Criterion.(*eval.RubricsBasedCriterion); !ok {
			t.Errorf("Criterion type = %T, want *RubricsBasedCriterion", m.Criterion)
		}
	})

	t.Run("tool_trajectory", func(t *testing.T) {
		t.Parallel()
		data := `{"metricName":"tool_trajectory_avg_score","criterion":{"matchType":"IN_ORDER"}}`
		var m eval.EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		tc, ok := m.Criterion.(*eval.ToolTrajectoryCriterion)
		if !ok {
			t.Fatalf("Criterion type = %T, want *ToolTrajectoryCriterion", m.Criterion)
		}
		if tc.MatchType != eval.MatchInOrder {
			t.Errorf("MatchType = %v, want %v", tc.MatchType, eval.MatchInOrder)
		}
	})

	t.Run("user_simulator", func(t *testing.T) {
		t.Parallel()
		data := `{"metricName":"per_turn_user_simulator_quality_v1","criterion":{"stopSignal":"</stop>"}}`
		var m eval.EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if _, ok := m.Criterion.(*eval.LlmBackedUserSimulatorCriterion); !ok {
			t.Errorf("Criterion type = %T, want *LlmBackedUserSimulatorCriterion", m.Criterion)
		}
	})

	t.Run("hallucinations", func(t *testing.T) {
		t.Parallel()
		data := `{"metricName":"hallucinations_v1","criterion":{"evaluateIntermediateNlResponses":true}}`
		var m eval.EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		hc, ok := m.Criterion.(*eval.HallucinationsCriterion)
		if !ok {
			t.Fatalf("Criterion type = %T, want *HallucinationsCriterion", m.Criterion)
		}
		if !hc.EvaluateIntermediateNLResponses {
			t.Error("EvaluateIntermediateNLResponses = false, want true")
		}
	})

	t.Run("llm_judge", func(t *testing.T) {
		t.Parallel()
		data := `{"metricName":"final_response_match_v2","criterion":{"judgeModelOptions":{"judgeModel":"gemini-2.5-pro"}}}`
		var m eval.EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		lc, ok := m.Criterion.(*eval.LlmAsAJudgeCriterion)
		if !ok {
			t.Fatalf("Criterion type = %T, want *LlmAsAJudgeCriterion", m.Criterion)
		}
		if lc.JudgeModelOptions.JudgeModel != "gemini-2.5-pro" {
			t.Errorf("JudgeModel = %q, want %q", lc.JudgeModelOptions.JudgeModel, "gemini-2.5-pro")
		}
	})

	t.Run("base_criterion_fallback", func(t *testing.T) {
		t.Parallel()
		data := `{"metricName":"custom_metric","criterion":{"threshold":0.5}}`
		var m eval.EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if _, ok := m.Criterion.(*eval.BaseCriterionImpl); !ok {
			t.Errorf("Criterion type = %T, want *BaseCriterionImpl", m.Criterion)
		}
	})
}

func TestToolTrajectoryCriterion_UnmarshalJSON_DefaultMatchType(t *testing.T) {
	t.Parallel()
	data := `{"threshold":0.8}`
	var c eval.ToolTrajectoryCriterion
	if err := json.Unmarshal([]byte(data), &c); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if c.MatchType != eval.MatchExact {
		t.Errorf("MatchType = %v, want %v (default)", c.MatchType, eval.MatchExact)
	}
}

func TestBaseCriterionImpl(t *testing.T) {
	t.Parallel()
	threshold := 0.9
	b := eval.BaseCriterionImpl{
		Threshold:                           &threshold,
		IncludeIntermediateResponsesInFinal: true,
	}
	if b.GetThreshold() == nil || *b.GetThreshold() != 0.9 {
		t.Error("GetThreshold returned wrong value")
	}
	if !b.GetIncludeIntermediateResponsesInFinal() {
		t.Error("GetIncludeIntermediateResponsesInFinal = false, want true")
	}
}

// FuzzParseMatchType verifies that eval.ParseMatchType never panics on
// arbitrary string input. Valid match type strings should parse without error;
// invalid strings should return an error (no panic).
func FuzzParseMatchType(f *testing.F) {
	// Seed: valid match type.
	f.Add("exact")
	// Seed: invalid match type.
	f.Add("invalid")
	// Seed: empty string.
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		_, err := eval.ParseMatchType(input)
		// The function must not panic — reaching here is the primary assertion.
		// Both error and non-error outcomes are acceptable as long as no panic
		// occurred.
		_ = err
	})
}
