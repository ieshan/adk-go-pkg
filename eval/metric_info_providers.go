package eval

// TrajectoryEvaluatorMetricInfoProvider provides metric info for TrajectoryEvaluator.
type TrajectoryEvaluatorMetricInfoProvider struct{}

// GetMetricInfo returns the metric info for the tool trajectory average score metric.
func (TrajectoryEvaluatorMetricInfoProvider) GetMetricInfo() MetricInfo {
	return MetricInfo{
		MetricName:  string(ToolTrajectoryAvgScore),
		Description: "This metric compares two tool call trajectories (expected vs. actual) for the same user interaction. It performs an exact match on the tool name and arguments for each step in the trajectory. A score of 1.0 indicates a perfect match, while 0.0 indicates a mismatch. Higher values are better.",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}

// ResponseEvaluatorMetricInfoProvider provides metric info for ResponseEvaluator.
type ResponseEvaluatorMetricInfoProvider struct {
	metricName string
}

// GetMetricInfo returns the metric info for the response evaluation metric.
func (p ResponseEvaluatorMetricInfoProvider) GetMetricInfo() MetricInfo {
	if p.metricName == string(ResponseEvaluationScore) {
		return MetricInfo{
			MetricName:  string(ResponseEvaluationScore),
			Description: "This metric evaluates how coherent agent's response was. Value range of this metric is [1,5], with values closer to 5 more desirable.",
			MetricValueInfo: &MetricValueInfo{
				Interval: &Interval{MinValue: 1.0, MaxValue: 5.0},
			},
		}
	}
	return MetricInfo{
		MetricName:  string(ResponseMatchScore),
		Description: "This metric evaluates if the agent's final response matches a golden/expected final response using Rouge_1 metric. Value range for this metric is [0,1], with values closer to 1 more desirable.",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}

// SafetyEvaluatorV1MetricInfoProvider provides metric info for SafetyEvaluatorV1.
type SafetyEvaluatorV1MetricInfoProvider struct{}

// GetMetricInfo returns the metric info for the safety v1 metric.
func (SafetyEvaluatorV1MetricInfoProvider) GetMetricInfo() MetricInfo {
	return MetricInfo{
		MetricName:  string(SafetyV1),
		Description: "This metric evaluates the safety (harmlessness) of an Agent's Response. Value range of the metric is [0, 1], with values closer to 1 to be more desirable (safe).",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}

// MultiTurnTaskSuccessV1MetricInfoProvider provides metric info for MultiTurnTaskSuccessV1.
type MultiTurnTaskSuccessV1MetricInfoProvider struct{}

// GetMetricInfo returns the metric info for the multi-turn task success v1 metric.
func (MultiTurnTaskSuccessV1MetricInfoProvider) GetMetricInfo() MetricInfo {
	return MetricInfo{
		MetricName:  string(MultiTurnTaskSuccessV1),
		Description: "Evaluates if the agent was able to achieve the goal or goals of the conversation. Value range of the metric is [0, 1], with values closer to 1 to be more desirable.",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}

// MultiTurnTrajectoryQualityV1MetricInfoProvider provides metric info for MultiTurnTrajectoryQualityV1.
type MultiTurnTrajectoryQualityV1MetricInfoProvider struct{}

// GetMetricInfo returns the metric info for the multi-turn trajectory quality v1 metric.
func (MultiTurnTrajectoryQualityV1MetricInfoProvider) GetMetricInfo() MetricInfo {
	return MetricInfo{
		MetricName:  string(MultiTurnTrajectoryQualityV1),
		Description: "Evaluates the overall trajectory of the conversation. This is a reference free metric. Value range of the metric is [0, 1], with values closer to 1 to be more desirable.",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}

// MultiTurnToolUseQualityV1MetricInfoProvider provides metric info for MultiTurnToolUseQualityV1.
type MultiTurnToolUseQualityV1MetricInfoProvider struct{}

// GetMetricInfo returns the metric info for the multi-turn tool use quality v1 metric.
func (MultiTurnToolUseQualityV1MetricInfoProvider) GetMetricInfo() MetricInfo {
	return MetricInfo{
		MetricName:  string(MultiTurnToolUseQualityV1),
		Description: "Evaluates the function calls made during a multi-turn conversation. This is a reference free metric. Value range of the metric is [0, 1], with values closer to 1 to be more desirable.",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}

// FinalResponseMatchV2EvaluatorMetricInfoProvider provides metric info for FinalResponseMatchV2Evaluator.
type FinalResponseMatchV2EvaluatorMetricInfoProvider struct{}

// GetMetricInfo returns the metric info for the final response match v2 metric.
func (FinalResponseMatchV2EvaluatorMetricInfoProvider) GetMetricInfo() MetricInfo {
	return MetricInfo{
		MetricName:  string(FinalResponseMatchV2),
		Description: "This metric evaluates if the agent's final response matches a golden/expected final response using LLM as a judge. Value range for this metric is [0,1], with values closer to 1 more desirable.",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}

// RubricBasedFinalResponseQualityV1EvaluatorMetricInfoProvider provides metric info.
type RubricBasedFinalResponseQualityV1EvaluatorMetricInfoProvider struct{}

// GetMetricInfo returns the metric info for the rubric-based final response quality v1 metric.
func (RubricBasedFinalResponseQualityV1EvaluatorMetricInfoProvider) GetMetricInfo() MetricInfo {
	return MetricInfo{
		MetricName:  string(RubricBasedFinalResponseQualityV1),
		Description: "This metric assess if the agent's final response against a set of rubrics using LLM as a judge. Value range for this metric is [0,1], with values closer to 1 more desirable.",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}

// HallucinationsV1EvaluatorMetricInfoProvider provides metric info for HallucinationsV1Evaluator.
type HallucinationsV1EvaluatorMetricInfoProvider struct{}

// GetMetricInfo returns the metric info for the hallucinations v1 metric.
func (HallucinationsV1EvaluatorMetricInfoProvider) GetMetricInfo() MetricInfo {
	return MetricInfo{
		MetricName:  string(HallucinationsV1),
		Description: "This metric assesses whether a model response contains any false, contradictory, or unsupported claims using a LLM as judge. Value range for this metric is [0,1], with values closer to 1 more desirable.",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}

// RubricBasedToolUseQualityV1EvaluatorMetricInfoProvider provides metric info.
type RubricBasedToolUseQualityV1EvaluatorMetricInfoProvider struct{}

// GetMetricInfo returns the metric info for the rubric-based tool use quality v1 metric.
func (RubricBasedToolUseQualityV1EvaluatorMetricInfoProvider) GetMetricInfo() MetricInfo {
	return MetricInfo{
		MetricName:  string(RubricBasedToolUseQualityV1),
		Description: "This metric assess if the agent's usage of tools against a set of rubrics using LLM as a judge. Value range for this metric is [0,1], with values closer to 1 more desirable.",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}

// PerTurnUserSimulatorQualityV1MetricInfoProvider provides metric info.
type PerTurnUserSimulatorQualityV1MetricInfoProvider struct{}

// GetMetricInfo returns the metric info for the per-turn user simulator quality v1 metric.
func (PerTurnUserSimulatorQualityV1MetricInfoProvider) GetMetricInfo() MetricInfo {
	return MetricInfo{
		MetricName:  string(PerTurnUserSimulatorQualityV1),
		Description: "This metric evaluates if the user messages generated by a user simulator follow the given conversation scenario. It validates each message separately. The resulting metric computes the percentage of user messages that we mark as valid. The value range for this metric is [0,1], with values closer to 1 more desirable.",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}

// RubricBasedMultiTurnTrajectoryMetricInfoProvider provides metric info.
type RubricBasedMultiTurnTrajectoryMetricInfoProvider struct{}

// GetMetricInfo returns the metric info for the rubric-based multi-turn trajectory metric.
func (RubricBasedMultiTurnTrajectoryMetricInfoProvider) GetMetricInfo() MetricInfo {
	return MetricInfo{
		MetricName:  string(RubricBasedMultiTurnTrajectoryQualityV1),
		Description: "This metric evaluates the agent's multi-turn trajectory against a set of user-provided rubrics using an LLM as a judge. Value range for this metric is [0,1], with values closer to 1 more desirable.",
		MetricValueInfo: &MetricValueInfo{
			Interval: &Interval{MinValue: 0.0, MaxValue: 1.0},
		},
	}
}
