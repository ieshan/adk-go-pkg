package eval

import (
	"encoding/json"
	"os"
)

// CodeConfig defines the location of a custom metric function.
type CodeConfig struct {
	// ModulePath is the Go import path of the module containing the function.
	// Not used in Go — custom metrics are registered via CustomMetricFunc.
	// Kept for JSON compatibility with Python eval configs.
	ModulePath string `json:"modulePath,omitempty"`
}

// CustomMetricConfig defines a custom metric.
type CustomMetricConfig struct {
	// CodeConfig specifies the location of the custom metric function.
	CodeConfig CodeConfig `json:"codeConfig,omitempty"`

	// MetricInfo provides metadata about the custom metric.
	MetricInfo *MetricInfo `json:"metricInfo,omitempty"`

	// Description is a human-readable description.
	Description string `json:"description,omitempty"`
}

// EvalConfig is the configuration for evaluating an agent.
type EvalConfig struct {
	// Criteria maps metric names to their thresholds or criterion objects.
	// Values can be either a float64 (threshold) or a criterion object.
	Criteria map[string]json.RawMessage `json:"criteria,omitempty"`

	// CustomMetrics defines custom metrics.
	CustomMetrics map[string]CustomMetricConfig `json:"customMetrics,omitempty"`

	// UserSimulatorConfig is the configuration for the user simulator.
	UserSimulatorConfig json.RawMessage `json:"userSimulatorConfig,omitempty"`
}

// GetEvaluationCriteriaOrDefault loads eval config from the given path beneath
// root. If root is nil or configPath is empty, or the file doesn't exist,
// returns a default config with tool_trajectory_avg_score=1.0 and
// response_match_score=0.8.
func GetEvaluationCriteriaOrDefault(root *os.Root, configPath string) EvalConfig {
	if root == nil || configPath == "" {
		return defaultEvalConfig()
	}
	data, err := root.ReadFile(configPath)
	if err != nil {
		return defaultEvalConfig()
	}

	var config EvalConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return defaultEvalConfig()
	}

	// Inject default type="llm_backed" for user simulator config if missing.
	if len(config.UserSimulatorConfig) > 0 && string(config.UserSimulatorConfig) != "null" {
		config.UserSimulatorConfig = injectDefaultUserSimulatorType(config.UserSimulatorConfig)
	}

	return config
}

// defaultEvalConfig returns the default eval configuration.
func defaultEvalConfig() EvalConfig {
	criteria := map[string]json.RawMessage{}
	criteria["tool_trajectory_avg_score"] = json.RawMessage("1.0")
	criteria["response_match_score"] = json.RawMessage("0.8")
	return EvalConfig{
		Criteria: criteria,
	}
}

// injectDefaultUserSimulatorType adds type="llm_backed" to the user simulator
// config JSON if the type field is missing (backward compatibility).
func injectDefaultUserSimulatorType(raw json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}
	if _, ok := m["type"]; !ok {
		m["type"] = json.RawMessage(`"llm_backed"`)
	}
	data, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return data
}

// GetEvalMetricsFromConfig converts an EvalConfig to a list of EvalMetric.
func GetEvalMetricsFromConfig(config EvalConfig) []EvalMetric {
	var metrics []EvalMetric

	for metricName, rawCriterion := range config.Criteria {
		metric := EvalMetric{
			MetricName: metricName,
		}

		// Try to parse as a float threshold first.
		var threshold float64
		if err := json.Unmarshal(rawCriterion, &threshold); err == nil {
			metric.Threshold = &threshold
		} else {
			// Parse as a criterion object.
			criterion, err := unmarshalCriterion(rawCriterion)
			if err == nil {
				metric.Criterion = criterion
				metric.Threshold = criterion.GetThreshold()
			}
		}

		metrics = append(metrics, metric)
	}

	// Add custom metrics.
	for metricName, customConfig := range config.CustomMetrics {
		metric := EvalMetric{
			MetricName:         metricName,
			CustomFunctionPath: customConfig.CodeConfig.ModulePath,
		}
		if customConfig.MetricInfo != nil {
			metric.MetricName = customConfig.MetricInfo.MetricName
		}
		metrics = append(metrics, metric)
	}

	return metrics
}
