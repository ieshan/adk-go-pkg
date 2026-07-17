package eval

import (
	"fmt"
	"sync"

	"google.golang.org/adk/v2/model"
)

// EvaluatorFactory creates an Evaluator for the given EvalMetric.
type EvaluatorFactory func(EvalMetric) (Evaluator, error)

// EvaluatorFactoryWithLLM creates an Evaluator that requires an LLM.
type EvaluatorFactoryWithLLM func(EvalMetric, model.LLM) (Evaluator, error)

// registryEntry holds an evaluator factory and its metric info.
type registryEntry struct {
	factory    EvaluatorFactory
	info       MetricInfo
	needsLLM   bool
	llmFactory EvaluatorFactoryWithLLM
}

// MetricEvaluatorRegistry manages registration and retrieval of evaluator
// factories keyed by metric name.
type MetricEvaluatorRegistry struct {
	mu       sync.RWMutex
	registry map[string]registryEntry
}

// NewMetricEvaluatorRegistry creates a new empty registry.
func NewMetricEvaluatorRegistry() *MetricEvaluatorRegistry {
	return &MetricEvaluatorRegistry{
		registry: make(map[string]registryEntry),
	}
}

// RegisterEvaluator registers an evaluator factory for a metric.
func (r *MetricEvaluatorRegistry) RegisterEvaluator(info MetricInfo, factory EvaluatorFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registry[info.MetricName] = registryEntry{
		factory: factory,
		info:    info,
	}
}

// RegisterEvaluatorWithLLM registers an evaluator factory that requires an LLM.
func (r *MetricEvaluatorRegistry) RegisterEvaluatorWithLLM(info MetricInfo, factory EvaluatorFactoryWithLLM) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registry[info.MetricName] = registryEntry{
		needsLLM:   true,
		llmFactory: factory,
		info:       info,
	}
}

// GetEvaluator returns an evaluator for the given metric. If the metric
// requires an LLM, use GetEvaluatorWithLLM instead.
func (r *MetricEvaluatorRegistry) GetEvaluator(evalMetric EvalMetric) (Evaluator, error) {
	r.mu.RLock()
	entry, ok := r.registry[evalMetric.MetricName]
	r.mu.RUnlock()

	if !ok {
		return nil, NewNotFoundError("metric", evalMetric.MetricName)
	}

	if entry.needsLLM {
		return nil, fmt.Errorf("metric %q requires an LLM, use GetEvaluatorWithLLM", evalMetric.MetricName)
	}

	return entry.factory(evalMetric)
}

// GetEvaluatorWithLLM returns an evaluator that requires an LLM.
func (r *MetricEvaluatorRegistry) GetEvaluatorWithLLM(evalMetric EvalMetric, llm model.LLM) (Evaluator, error) {
	r.mu.RLock()
	entry, ok := r.registry[evalMetric.MetricName]
	r.mu.RUnlock()

	if !ok {
		return nil, NewNotFoundError("metric", evalMetric.MetricName)
	}

	if entry.needsLLM {
		return entry.llmFactory(evalMetric, llm)
	}

	return entry.factory(evalMetric)
}

// GetRegisteredMetrics returns info for all registered metrics.
func (r *MetricEvaluatorRegistry) GetRegisteredMetrics() []MetricInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]MetricInfo, 0, len(r.registry))
	for _, entry := range r.registry {
		result = append(result, entry.info)
	}
	return result
}

// HasMetric returns true if the metric is registered.
func (r *MetricEvaluatorRegistry) HasMetric(metricName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.registry[metricName]
	return ok
}

// DefaultMetricEvaluatorRegistry returns a registry with all built-in
// evaluators pre-registered.
func DefaultMetricEvaluatorRegistry() *MetricEvaluatorRegistry {
	r := NewMetricEvaluatorRegistry()

	// Trajectory evaluator (no LLM needed).
	r.RegisterEvaluator(
		TrajectoryEvaluatorMetricInfoProvider{}.GetMetricInfo(),
		func(em EvalMetric) (Evaluator, error) {
			return NewTrajectoryEvaluator(em), nil
		},
	)

	// Response evaluator (no LLM needed).
	r.RegisterEvaluator(
		ResponseEvaluatorMetricInfoProvider{metricName: string(ResponseMatchScore)}.GetMetricInfo(),
		func(em EvalMetric) (Evaluator, error) {
			return NewResponseEvaluator(em), nil
		},
	)
	r.RegisterEvaluator(
		ResponseEvaluatorMetricInfoProvider{metricName: string(ResponseEvaluationScore)}.GetMetricInfo(),
		func(em EvalMetric) (Evaluator, error) {
			return NewResponseEvaluator(em), nil
		},
	)

	// Safety evaluator (no LLM needed, uses Vertex AI stub).
	r.RegisterEvaluator(
		SafetyEvaluatorV1MetricInfoProvider{}.GetMetricInfo(),
		func(em EvalMetric) (Evaluator, error) {
			return NewSafetyEvaluatorV1(em), nil
		},
	)

	// Multi-turn evaluators (no LLM needed, use Vertex AI stub).
	r.RegisterEvaluator(
		MultiTurnTaskSuccessV1MetricInfoProvider{}.GetMetricInfo(),
		func(em EvalMetric) (Evaluator, error) {
			return NewMultiTurnTaskSuccessV1Evaluator(em), nil
		},
	)
	r.RegisterEvaluator(
		MultiTurnTrajectoryQualityV1MetricInfoProvider{}.GetMetricInfo(),
		func(em EvalMetric) (Evaluator, error) {
			return NewMultiTurnTrajectoryQualityV1Evaluator(em), nil
		},
	)
	r.RegisterEvaluator(
		MultiTurnToolUseQualityV1MetricInfoProvider{}.GetMetricInfo(),
		func(em EvalMetric) (Evaluator, error) {
			return NewMultiTurnToolUseQualityV1Evaluator(em), nil
		},
	)

	// FinalResponseMatchV2 (requires LLM).
	r.RegisterEvaluatorWithLLM(
		FinalResponseMatchV2EvaluatorMetricInfoProvider{}.GetMetricInfo(),
		func(em EvalMetric, llm model.LLM) (Evaluator, error) {
			return NewFinalResponseMatchV2Evaluator(em, llm), nil
		},
	)

	// Rubric-based evaluators (require LLM).
	r.RegisterEvaluatorWithLLM(
		RubricBasedFinalResponseQualityV1EvaluatorMetricInfoProvider{}.GetMetricInfo(),
		func(em EvalMetric, llm model.LLM) (Evaluator, error) {
			return NewRubricBasedFinalResponseQualityV1Evaluator(em, llm)
		},
	)
	r.RegisterEvaluatorWithLLM(
		RubricBasedToolUseQualityV1EvaluatorMetricInfoProvider{}.GetMetricInfo(),
		func(em EvalMetric, llm model.LLM) (Evaluator, error) {
			return NewRubricBasedToolUseQualityV1Evaluator(em, llm)
		},
	)
	r.RegisterEvaluatorWithLLM(
		RubricBasedMultiTurnTrajectoryMetricInfoProvider{}.GetMetricInfo(),
		func(em EvalMetric, llm model.LLM) (Evaluator, error) {
			return NewRubricBasedMultiTurnTrajectoryEvaluator(em, llm)
		},
	)

	// Hallucinations evaluator (requires LLM).
	r.RegisterEvaluatorWithLLM(
		HallucinationsV1EvaluatorMetricInfoProvider{}.GetMetricInfo(),
		func(em EvalMetric, llm model.LLM) (Evaluator, error) {
			return NewHallucinationsV1Evaluator(em, llm)
		},
	)

	// Per-turn user simulator quality (requires LLM).
	r.RegisterEvaluatorWithLLM(
		PerTurnUserSimulatorQualityV1MetricInfoProvider{}.GetMetricInfo(),
		func(em EvalMetric, llm model.LLM) (Evaluator, error) {
			return NewPerTurnUserSimulatorQualityV1Evaluator(em, llm)
		},
	)

	return r
}
