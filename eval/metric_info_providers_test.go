package eval

import (
	"testing"
)

func TestMetricInfoProviders_AllHaveMetricName(t *testing.T) {
	providers := []MetricInfoProvider{
		TrajectoryEvaluatorMetricInfoProvider{},
		ResponseEvaluatorMetricInfoProvider{metricName: string(ResponseMatchScore)},
		ResponseEvaluatorMetricInfoProvider{metricName: string(ResponseEvaluationScore)},
		SafetyEvaluatorV1MetricInfoProvider{},
		MultiTurnTaskSuccessV1MetricInfoProvider{},
		MultiTurnTrajectoryQualityV1MetricInfoProvider{},
		MultiTurnToolUseQualityV1MetricInfoProvider{},
		FinalResponseMatchV2EvaluatorMetricInfoProvider{},
		RubricBasedFinalResponseQualityV1EvaluatorMetricInfoProvider{},
		RubricBasedToolUseQualityV1EvaluatorMetricInfoProvider{},
		RubricBasedMultiTurnTrajectoryMetricInfoProvider{},
		HallucinationsV1EvaluatorMetricInfoProvider{},
		PerTurnUserSimulatorQualityV1MetricInfoProvider{},
	}

	for _, p := range providers {
		info := p.GetMetricInfo()
		if info.MetricName == "" {
			t.Errorf("MetricInfo from %T has empty MetricName", p)
		}
	}
}

func TestDefaultRegistry_AllMetricsHaveInfo(t *testing.T) {
	r := DefaultMetricEvaluatorRegistry()
	metrics := r.GetRegisteredMetrics()
	if len(metrics) == 0 {
		t.Error("expected non-empty registered metrics")
	}
	for _, m := range metrics {
		if m.MetricName == "" {
			t.Error("found metric with empty MetricName")
		}
	}
}
