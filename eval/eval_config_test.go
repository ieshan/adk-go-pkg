package eval

import (
	"encoding/json"
	"os"
	"testing"
)

func TestGetEvaluationCriteriaOrDefault_EmptyPath(t *testing.T) {
	config := GetEvaluationCriteriaOrDefault(nil, "")
	if len(config.Criteria) == 0 {
		t.Error("expected default criteria to be non-empty")
	}
}

func TestGetEvaluationCriteriaOrDefault_NonExistentFile(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	config := GetEvaluationCriteriaOrDefault(root, "nonexistent/path.json")
	if len(config.Criteria) == 0 {
		t.Error("expected default criteria to be non-empty")
	}
}

func TestGetEvaluationCriteriaOrDefault_DefaultValues(t *testing.T) {
	config := GetEvaluationCriteriaOrDefault(nil, "")
	if _, ok := config.Criteria["tool_trajectory_avg_score"]; !ok {
		t.Error("expected tool_trajectory_avg_score in default config")
	}
	if _, ok := config.Criteria["response_match_score"]; !ok {
		t.Error("expected response_match_score in default config")
	}
}

func TestGetEvalMetricsFromConfig_FloatThreshold(t *testing.T) {
	config := EvalConfig{
		Criteria: map[string]json.RawMessage{
			"tool_trajectory_avg_score": json.RawMessage("1.0"),
		},
	}
	metrics := GetEvalMetricsFromConfig(config)
	if len(metrics) != 1 {
		t.Fatalf("len(metrics) = %d, want 1", len(metrics))
	}
	if metrics[0].MetricName != "tool_trajectory_avg_score" {
		t.Errorf("MetricName = %q, want %q", metrics[0].MetricName, "tool_trajectory_avg_score")
	}
	if metrics[0].Threshold == nil || *metrics[0].Threshold != 1.0 {
		t.Errorf("Threshold = %v, want 1.0", metrics[0].Threshold)
	}
}

func TestGetEvalMetricsFromConfig_CriterionObject(t *testing.T) {
	config := EvalConfig{
		Criteria: map[string]json.RawMessage{
			"tool_trajectory_avg_score": json.RawMessage(`{"matchType":"IN_ORDER","threshold":0.8}`),
		},
	}
	metrics := GetEvalMetricsFromConfig(config)
	if len(metrics) != 1 {
		t.Fatalf("len(metrics) = %d, want 1", len(metrics))
	}
	if metrics[0].Criterion == nil {
		t.Fatal("Criterion is nil")
	}
	tc, ok := metrics[0].Criterion.(*ToolTrajectoryCriterion)
	if !ok {
		t.Fatalf("Criterion type = %T, want *ToolTrajectoryCriterion", metrics[0].Criterion)
	}
	if tc.MatchType != MatchInOrder {
		t.Errorf("MatchType = %v, want %v", tc.MatchType, MatchInOrder)
	}
}

func TestGetEvalMetricsFromConfig_CustomMetrics(t *testing.T) {
	config := EvalConfig{
		CustomMetrics: map[string]CustomMetricConfig{
			"my_custom": {
				Description: "A custom metric",
				MetricInfo:  &MetricInfo{MetricName: "my_custom_metric"},
			},
		},
	}
	metrics := GetEvalMetricsFromConfig(config)
	if len(metrics) != 1 {
		t.Fatalf("len(metrics) = %d, want 1", len(metrics))
	}
	if metrics[0].MetricName != "my_custom_metric" {
		t.Errorf("MetricName = %q, want %q", metrics[0].MetricName, "my_custom_metric")
	}
}

func TestInjectDefaultUserSimulatorType(t *testing.T) {
	t.Run("adds_type_when_missing", func(t *testing.T) {
		raw := json.RawMessage(`{"model":"gemini-2.5-flash"}`)
		result := injectDefaultUserSimulatorType(raw)
		var m map[string]json.RawMessage
		if err := json.Unmarshal(result, &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if _, ok := m["type"]; !ok {
			t.Error("expected type field to be added")
		}
	})

	t.Run("preserves_existing_type", func(t *testing.T) {
		raw := json.RawMessage(`{"type":"static","model":"gemini-2.5-flash"}`)
		result := injectDefaultUserSimulatorType(raw)
		var m map[string]json.RawMessage
		if err := json.Unmarshal(result, &m); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		typeVal := string(m["type"])
		if typeVal != `"static"` {
			t.Errorf("type = %s, want %q", typeVal, `"static"`)
		}
	})
}
