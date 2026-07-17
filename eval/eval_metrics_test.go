package eval

import (
	"encoding/json"
	"testing"
)

func TestParseMatchType(t *testing.T) {
	tests := []struct {
		input string
		want  MatchType
	}{
		{"EXACT", MatchExact},
		{"exact", MatchExact},
		{"IN_ORDER", MatchInOrder},
		{"in-order", MatchInOrder},
		{"INORDER", MatchInOrder},
		{"inorder", MatchInOrder},
		{"IN ORDER", MatchInOrder},
		{"ANY_ORDER", MatchAnyOrder},
		{"any-order", MatchAnyOrder},
		{"ANYORDER", MatchAnyOrder},
		{"anyorder", MatchAnyOrder},
		{"ANY ORDER", MatchAnyOrder},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMatchType(tt.input)
			if err != nil {
				t.Fatalf("ParseMatchType(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseMatchType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}

	t.Run("invalid", func(t *testing.T) {
		_, err := ParseMatchType("invalid")
		if err == nil {
			t.Error("expected error for invalid match type")
		}
	})
}

func TestDefaultJudgeModelOptions(t *testing.T) {
	opts := DefaultJudgeModelOptions()
	if opts.JudgeModel != "gemini-2.5-flash" {
		t.Errorf("JudgeModel = %q, want %q", opts.JudgeModel, "gemini-2.5-flash")
	}
	if opts.NumSamples != 5 {
		t.Errorf("NumSamples = %d, want 5", opts.NumSamples)
	}
}

func TestEvalMetric_MarshalUnmarshal(t *testing.T) {
	threshold := 0.8
	metric := EvalMetric{
		MetricName: "tool_trajectory_avg_score",
		Threshold:  &threshold,
		Criterion: &ToolTrajectoryCriterion{
			BaseCriterionImpl: BaseCriterionImpl{Threshold: &threshold},
			MatchType:         MatchExact,
		},
	}

	data, err := json.Marshal(metric)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var unmarshaled EvalMetric
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if unmarshaled.MetricName != metric.MetricName {
		t.Errorf("MetricName = %q, want %q", unmarshaled.MetricName, metric.MetricName)
	}
	if unmarshaled.Criterion == nil {
		t.Fatal("Criterion is nil after unmarshal")
	}
	tc, ok := unmarshaled.Criterion.(*ToolTrajectoryCriterion)
	if !ok {
		t.Fatalf("Criterion type = %T, want *ToolTrajectoryCriterion", unmarshaled.Criterion)
	}
	if tc.MatchType != MatchExact {
		t.Errorf("MatchType = %v, want %v", tc.MatchType, MatchExact)
	}
}

func TestEvalMetric_UnmarshalCriterionTypes(t *testing.T) {
	t.Run("rubrics_based", func(t *testing.T) {
		data := `{"metricName":"rubric_based_final_response_quality_v1","criterion":{"rubrics":[{"rubricId":"r1"}]}}`
		var m EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if _, ok := m.Criterion.(*RubricsBasedCriterion); !ok {
			t.Fatalf("Criterion type = %T, want *RubricsBasedCriterion", m.Criterion)
		}
	})

	t.Run("tool_trajectory", func(t *testing.T) {
		data := `{"metricName":"tool_trajectory_avg_score","criterion":{"matchType":"IN_ORDER"}}`
		var m EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		tc, ok := m.Criterion.(*ToolTrajectoryCriterion)
		if !ok {
			t.Fatalf("Criterion type = %T, want *ToolTrajectoryCriterion", m.Criterion)
		}
		if tc.MatchType != MatchInOrder {
			t.Errorf("MatchType = %v, want %v", tc.MatchType, MatchInOrder)
		}
	})

	t.Run("user_simulator", func(t *testing.T) {
		data := `{"metricName":"per_turn_user_simulator_quality_v1","criterion":{"stopSignal":"</stop>"}}`
		var m EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if _, ok := m.Criterion.(*LlmBackedUserSimulatorCriterion); !ok {
			t.Fatalf("Criterion type = %T, want *LlmBackedUserSimulatorCriterion", m.Criterion)
		}
	})

	t.Run("hallucinations", func(t *testing.T) {
		data := `{"metricName":"hallucinations_v1","criterion":{"evaluateIntermediateNlResponses":true}}`
		var m EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		hc, ok := m.Criterion.(*HallucinationsCriterion)
		if !ok {
			t.Fatalf("Criterion type = %T, want *HallucinationsCriterion", m.Criterion)
		}
		if !hc.EvaluateIntermediateNLResponses {
			t.Error("EvaluateIntermediateNLResponses = false, want true")
		}
	})

	t.Run("llm_judge", func(t *testing.T) {
		data := `{"metricName":"final_response_match_v2","criterion":{"judgeModelOptions":{"judgeModel":"gemini-2.5-pro"}}}`
		var m EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		lc, ok := m.Criterion.(*LlmAsAJudgeCriterion)
		if !ok {
			t.Fatalf("Criterion type = %T, want *LlmAsAJudgeCriterion", m.Criterion)
		}
		if lc.JudgeModelOptions.JudgeModel != "gemini-2.5-pro" {
			t.Errorf("JudgeModel = %q, want %q", lc.JudgeModelOptions.JudgeModel, "gemini-2.5-pro")
		}
	})

	t.Run("base_criterion_fallback", func(t *testing.T) {
		data := `{"metricName":"custom_metric","criterion":{"threshold":0.5}}`
		var m EvalMetric
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if _, ok := m.Criterion.(*BaseCriterionImpl); !ok {
			t.Fatalf("Criterion type = %T, want *BaseCriterionImpl", m.Criterion)
		}
	})
}

func TestToolTrajectoryCriterion_UnmarshalJSON_DefaultMatchType(t *testing.T) {
	data := `{"threshold":0.8}`
	var c ToolTrajectoryCriterion
	if err := json.Unmarshal([]byte(data), &c); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if c.MatchType != MatchExact {
		t.Errorf("MatchType = %v, want %v (default)", c.MatchType, MatchExact)
	}
}

func TestBaseCriterionImpl(t *testing.T) {
	threshold := 0.9
	b := BaseCriterionImpl{
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
