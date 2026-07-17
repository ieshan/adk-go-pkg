package eval

import (
	"encoding/json"
	"fmt"
	"strings"
)

// EvalStatus represents the evaluation status of a metric or case.
type EvalStatus string

const (
	EvalStatusPassed       EvalStatus = "PASSED"
	EvalStatusFailed       EvalStatus = "FAILED"
	EvalStatusNotEvaluated EvalStatus = "NOT_EVALUATED"
)

// PrebuiltMetrics defines the names of all built-in evaluation metrics.
type PrebuiltMetrics string

const (
	// ToolTrajectoryAvgScore compares tool call trajectories.
	ToolTrajectoryAvgScore PrebuiltMetrics = "tool_trajectory_avg_score"

	// ResponseEvaluationScore evaluates response coherence (1-5 scale).
	ResponseEvaluationScore PrebuiltMetrics = "response_evaluation_score"

	// ResponseMatchScore evaluates response match using ROUGE-1.
	ResponseMatchScore PrebuiltMetrics = "response_match_score"

	// SafetyV1 evaluates safety (harmlessness) of agent responses.
	SafetyV1 PrebuiltMetrics = "safety_v1"

	// FinalResponseMatchV2 evaluates final response match using LLM as judge.
	FinalResponseMatchV2 PrebuiltMetrics = "final_response_match_v2"

	// RubricBasedFinalResponseQualityV1 evaluates final response quality
	// against rubrics using LLM as judge.
	RubricBasedFinalResponseQualityV1 PrebuiltMetrics = "rubric_based_final_response_quality_v1"

	// HallucinationsV1 evaluates hallucinations in agent responses.
	HallucinationsV1 PrebuiltMetrics = "hallucinations_v1"

	// RubricBasedToolUseQualityV1 evaluates tool use quality against rubrics.
	RubricBasedToolUseQualityV1 PrebuiltMetrics = "rubric_based_tool_use_quality_v1"

	// PerTurnUserSimulatorQualityV1 evaluates user simulator quality per turn.
	PerTurnUserSimulatorQualityV1 PrebuiltMetrics = "per_turn_user_simulator_quality_v1"

	// MultiTurnTaskSuccessV1 evaluates multi-turn task success.
	MultiTurnTaskSuccessV1 PrebuiltMetrics = "multi_turn_task_success_v1"

	// MultiTurnTrajectoryQualityV1 evaluates multi-turn trajectory quality.
	MultiTurnTrajectoryQualityV1 PrebuiltMetrics = "multi_turn_trajectory_quality_v1"

	// MultiTurnToolUseQualityV1 evaluates multi-turn tool use quality.
	MultiTurnToolUseQualityV1 PrebuiltMetrics = "multi_turn_tool_use_quality_v1"

	// RubricBasedMultiTurnTrajectoryQualityV1 evaluates multi-turn trajectory
	// against rubrics.
	RubricBasedMultiTurnTrajectoryQualityV1 PrebuiltMetrics = "rubric_based_multi_turn_trajectory_quality_v1"
)

// MatchType defines how tool trajectory matching is performed.
type MatchType string

const (
	MatchExact    MatchType = "EXACT"
	MatchInOrder  MatchType = "IN_ORDER"
	MatchAnyOrder MatchType = "ANY_ORDER"
)

// ParseMatchType parses a match type string, handling case variations,
// hyphens, and spaces.
func ParseMatchType(s string) (MatchType, error) {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), "-", "_"))
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "EXACT":
		return MatchExact, nil
	case "IN_ORDER", "INORDER":
		return MatchInOrder, nil
	case "ANY_ORDER", "ANYORDER":
		return MatchAnyOrder, nil
	default:
		return "", fmt.Errorf("invalid match type: %q", s)
	}
}

// JudgeModelOptions configures the LLM used as an auto-rater.
type JudgeModelOptions struct {
	// JudgeModel is the name of the LLM to use as judge.
	// Default: "gemini-2.5-flash".
	JudgeModel string `json:"judgeModel,omitempty"`

	// JudgeModelConfig is the generation config for the judge model.
	// In Go, this is currently stored as a raw JSON message to avoid
	// tight coupling with genai types.
	JudgeModelConfig json.RawMessage `json:"judgeModelConfig,omitempty"`

	// NumSamples is the number of samples to generate per invocation.
	// Default: 5.
	NumSamples int `json:"numSamples,omitempty"`
}

// DefaultJudgeModelOptions returns JudgeModelOptions with default values.
func DefaultJudgeModelOptions() JudgeModelOptions {
	return JudgeModelOptions{
		JudgeModel: "gemini-2.5-flash",
		NumSamples: 5,
	}
}

// Interval defines the value range of a metric.
type Interval struct {
	MinValue  float64 `json:"minValue,omitempty"`
	OpenAtMin bool    `json:"openAtMin,omitempty"`
	MaxValue  float64 `json:"maxValue,omitempty"`
	OpenAtMax bool    `json:"openAtMax,omitempty"`
}

// MetricValueInfo describes the value range of a metric.
type MetricValueInfo struct {
	Interval *Interval `json:"interval,omitempty"`
}

// MetricInfo provides metadata about a metric.
type MetricInfo struct {
	MetricName      string           `json:"metricName"`
	Description     string           `json:"description,omitempty"`
	MetricValueInfo *MetricValueInfo `json:"metricValueInfo,omitempty"`
}

// MetricInfoProvider is an interface for providing metric info.
type MetricInfoProvider interface {
	GetMetricInfo() MetricInfo
}

// BaseCriterion is the interface for evaluation criteria. It is implemented
// by several concrete types, each with additional fields.
type BaseCriterion interface {
	// GetThreshold returns the threshold for passing the metric.
	GetThreshold() *float64

	// GetIncludeIntermediateResponsesInFinal returns whether intermediate
	// responses should be included in final response text.
	GetIncludeIntermediateResponsesInFinal() bool

	// IsBaseCriterion marks the interface.
	IsBaseCriterion()
}

// BaseCriterionImpl is the base implementation of BaseCriterion.
type BaseCriterionImpl struct {
	// Threshold is the minimum score required to pass the metric.
	Threshold *float64 `json:"threshold,omitempty"`

	// IncludeIntermediateResponsesInFinal controls whether intermediate
	// agent responses are included when extracting final response text.
	IncludeIntermediateResponsesInFinal bool `json:"includeIntermediateResponsesInFinal,omitempty"`
}

// GetThreshold returns the threshold.
func (b *BaseCriterionImpl) GetThreshold() *float64 { return b.Threshold }

// GetIncludeIntermediateResponsesInFinal returns the flag.
func (b *BaseCriterionImpl) GetIncludeIntermediateResponsesInFinal() bool {
	return b.IncludeIntermediateResponsesInFinal
}

// IsBaseCriterion marks the type as implementing BaseCriterion.
func (*BaseCriterionImpl) IsBaseCriterion() {}

// LlmAsAJudgeCriterion extends BaseCriterion with judge model options.
type LlmAsAJudgeCriterion struct {
	BaseCriterionImpl
	// JudgeModelOptions configures the LLM used as judge.
	JudgeModelOptions JudgeModelOptions `json:"judgeModelOptions,omitempty"`
}

// IsBaseCriterion marks the type.
func (*LlmAsAJudgeCriterion) IsBaseCriterion() {}

// RubricsBasedCriterion extends LlmAsAJudgeCriterion with rubrics.
type RubricsBasedCriterion struct {
	LlmAsAJudgeCriterion
	// Rubrics is the list of rubrics to evaluate against.
	Rubrics []Rubric `json:"rubrics,omitempty"`
}

// IsBaseCriterion marks the type.
func (*RubricsBasedCriterion) IsBaseCriterion() {}

// HallucinationsCriterion extends LlmAsAJudgeCriterion with hallucination
// evaluation options.
type HallucinationsCriterion struct {
	LlmAsAJudgeCriterion
	// EvaluateIntermediateNLResponses controls whether intermediate natural
	// language responses are evaluated for hallucinations.
	EvaluateIntermediateNLResponses bool `json:"evaluateIntermediateNlResponses,omitempty"`
}

// IsBaseCriterion marks the type.
func (*HallucinationsCriterion) IsBaseCriterion() {}

// ToolTrajectoryCriterion extends BaseCriterion with a match type for
// trajectory comparison.
type ToolTrajectoryCriterion struct {
	BaseCriterionImpl
	// MatchType defines how tool trajectories are compared.
	MatchType MatchType `json:"matchType,omitempty"`
}

// IsBaseCriterion marks the type.
func (*ToolTrajectoryCriterion) IsBaseCriterion() {}

// UnmarshalJSON implements custom JSON unmarshaling for ToolTrajectoryCriterion
// to handle match type string normalization.
func (c *ToolTrajectoryCriterion) UnmarshalJSON(data []byte) error {
	type alias ToolTrajectoryCriterion
	var aux struct {
		alias
		MatchType string `json:"matchType,omitempty"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal tool trajectory criterion: %w", err)
	}
	*c = ToolTrajectoryCriterion(aux.alias)
	if aux.MatchType != "" {
		mt, err := ParseMatchType(aux.MatchType)
		if err != nil {
			return err
		}
		c.MatchType = mt
	} else {
		c.MatchType = MatchExact // default
	}
	return nil
}

// LlmBackedUserSimulatorCriterion extends LlmAsAJudgeCriterion with a stop
// signal for user simulation.
type LlmBackedUserSimulatorCriterion struct {
	LlmAsAJudgeCriterion
	// StopSignal is the string the user simulator uses to signal the end
	// of the conversation. Default: "</finished>".
	StopSignal string `json:"stopSignal,omitempty"`
}

// IsBaseCriterion marks the type.
func (*LlmBackedUserSimulatorCriterion) IsBaseCriterion() {}

// EvalMetric represents a metric to be evaluated.
type EvalMetric struct {
	// MetricName is the name of the metric.
	MetricName string `json:"metricName"`

	// Threshold is the minimum score required to pass the metric.
	// If nil, the metric is evaluated but no pass/fail status is assigned.
	Threshold *float64 `json:"threshold,omitempty"`

	// Criterion contains additional configuration for the metric.
	// This is polymorphic — the concrete type depends on the metric.
	Criterion BaseCriterion `json:"-"`

	// CustomFunctionPath is the import path for custom metric functions.
	// Only used for custom metrics.
	CustomFunctionPath string `json:"customFunctionPath,omitempty"`
}

// MarshalJSON implements custom JSON marshaling for EvalMetric.
func (em EvalMetric) MarshalJSON() ([]byte, error) {
	type alias EvalMetric
	aux := struct {
		alias
		Criterion json.RawMessage `json:"criterion,omitempty"`
	}{}
	aux.alias = alias(em)
	if em.Criterion != nil {
		data, err := json.Marshal(em.Criterion)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal criterion: %w", err)
		}
		aux.Criterion = data
	}
	return json.Marshal(aux)
}

// UnmarshalJSON implements custom JSON unmarshaling for EvalMetric.
// It detects the criterion subtype by field presence.
func (em *EvalMetric) UnmarshalJSON(data []byte) error {
	type alias EvalMetric
	var aux struct {
		alias
		Criterion json.RawMessage `json:"criterion,omitempty"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal eval metric: %w", err)
	}
	*em = EvalMetric(aux.alias)

	if len(aux.Criterion) > 0 && string(aux.Criterion) != "null" {
		criterion, err := unmarshalCriterion(aux.Criterion)
		if err != nil {
			return fmt.Errorf("failed to unmarshal criterion: %w", err)
		}
		em.Criterion = criterion
	}

	return nil
}

// unmarshalCriterion detects the criterion subtype by field presence and
// unmarshals accordingly.
func unmarshalCriterion(data []byte) (BaseCriterion, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse criterion: %w", err)
	}

	if _, ok := raw["rubrics"]; ok {
		var c RubricsBasedCriterion
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("failed to unmarshal rubrics criterion: %w", err)
		}
		return &c, nil
	}

	if _, ok := raw["matchType"]; ok {
		var c ToolTrajectoryCriterion
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool trajectory criterion: %w", err)
		}
		return &c, nil
	}

	if _, ok := raw["stopSignal"]; ok {
		var c LlmBackedUserSimulatorCriterion
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user simulator criterion: %w", err)
		}
		return &c, nil
	}

	if _, ok := raw["evaluateIntermediateNlResponses"]; ok {
		var c HallucinationsCriterion
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("failed to unmarshal hallucinations criterion: %w", err)
		}
		return &c, nil
	}

	if _, ok := raw["judgeModelOptions"]; ok {
		var c LlmAsAJudgeCriterion
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("failed to unmarshal llm judge criterion: %w", err)
		}
		return &c, nil
	}

	var c BaseCriterionImpl
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal base criterion: %w", err)
	}
	return &c, nil
}
