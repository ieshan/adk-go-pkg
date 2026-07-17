package eval

import (
	"context"
	"fmt"
	"iter"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/adk/v2/model"
)

// LocalEvalService implements BaseEvalService using local managers and
// an AgentRunner for inference.
type LocalEvalService struct {
	evalSetsMgr           EvalSetsManager
	resultsMgr            EvalSetResultsManager
	registry              *MetricEvaluatorRegistry
	agentRunner           AgentRunner
	llm                   model.LLM
	userSimulatorProvider UserSimulatorProvider
}

// NewLocalEvalService creates a new LocalEvalService.
func NewLocalEvalService(
	evalSetsMgr EvalSetsManager,
	resultsMgr EvalSetResultsManager,
	registry *MetricEvaluatorRegistry,
	agentRunner AgentRunner,
	llm model.LLM,
) *LocalEvalService {
	if registry == nil {
		registry = DefaultMetricEvaluatorRegistry()
	}
	return &LocalEvalService{
		evalSetsMgr: evalSetsMgr,
		resultsMgr:  resultsMgr,
		registry:    registry,
		agentRunner: agentRunner,
		llm:         llm,
	}
}

// WithUserSimulatorProvider sets the user simulator provider for dynamic
// conversation evaluation and returns the receiver for chaining.
func (s *LocalEvalService) WithUserSimulatorProvider(provider UserSimulatorProvider) *LocalEvalService {
	s.userSimulatorProvider = provider
	return s
}

// PerformInference runs the agent on eval cases and yields results.
func (s *LocalEvalService) PerformInference(
	ctx context.Context,
	req *InferenceRequest,
) iter.Seq2[*InferenceResult, error] {
	return func(yield func(*InferenceResult, error) bool) {
		if req.InferenceConfig.UseLive {
			yield(nil, fmt.Errorf("live mode inference is not yet implemented"))
			return
		}

		evalSet, err := s.evalSetsMgr.GetEvalSet(ctx, req.AppName, req.EvalSetID)
		if err != nil {
			yield(nil, fmt.Errorf("failed to get eval set: %w", err))
			return
		}

		// Filter eval cases by EvalCaseIDs (or all if empty).
		cases := filterEvalCases(evalSet.EvalCases, req.EvalCaseIDs)

		parallelism := req.InferenceConfig.Parallelism
		if parallelism <= 0 {
			parallelism = 4
		}

		sem := make(chan struct{}, parallelism)
		var wg sync.WaitGroup
		var mu sync.Mutex
		results := make([]*InferenceResult, 0, len(cases))

		for _, evalCase := range cases {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(ec EvalCase) {
				defer wg.Done()
				defer func() { <-sem }()

				result := s.runInferenceForCase(ctx, req.AppName, req.EvalSetID, ec)
				mu.Lock()
				results = append(results, result)
				mu.Unlock()
			}(evalCase)
		}
		wg.Wait()

		for _, result := range results {
			if !yield(result, nil) {
				return
			}
		}
	}
}

func (s *LocalEvalService) runInferenceForCase(
	ctx context.Context,
	appName, evalSetID string,
	evalCase EvalCase,
) *InferenceResult {
	sessionID := GetSessionID()

	invocations, err := s.generateInvocations(ctx, appName, evalCase)
	if err != nil {
		return &InferenceResult{
			AppName:      appName,
			EvalSetID:    evalSetID,
			EvalCaseID:   evalCase.EvalID,
			SessionID:    sessionID,
			Status:       InferenceStatusFailure,
			ErrorMessage: err.Error(),
		}
	}

	return &InferenceResult{
		AppName:    appName,
		EvalSetID:  evalSetID,
		EvalCaseID: evalCase.EvalID,
		Inferences: invocations,
		SessionID:  sessionID,
		Status:     InferenceStatusSuccess,
	}
}

func (s *LocalEvalService) generateInvocations(
	ctx context.Context,
	appName string,
	evalCase EvalCase,
) ([]Invocation, error) {
	if len(evalCase.Conversation) > 0 {
		return GenerateStaticInvocations(ctx, s.agentRunner, appName, evalCase.Conversation)
	}

	if evalCase.ConversationScenario != nil && s.userSimulatorProvider != nil {
		return GenerateDynamicInvocations(ctx, s.agentRunner, appName, evalCase, s.userSimulatorProvider)
	}

	return nil, fmt.Errorf("no conversation or conversation scenario provided for eval case %q", evalCase.EvalID)
}

// Evaluate evaluates inference results against expected results.
func (s *LocalEvalService) Evaluate(
	ctx context.Context,
	req *EvaluateRequest,
) iter.Seq2[*EvalCaseResult, error] {
	return func(yield func(*EvalCaseResult, error) bool) {
		parallelism := req.EvaluateConfig.Parallelism
		if parallelism <= 0 {
			parallelism = 4
		}

		sem := make(chan struct{}, parallelism)
		var wg sync.WaitGroup
		var mu sync.Mutex
		results := make([]*EvalCaseResult, 0, len(req.InferenceResults))

		for _, inferenceResult := range req.InferenceResults {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(ir InferenceResult) {
				defer wg.Done()
				defer func() { <-sem }()

				result, err := s.evaluateInferenceResult(ctx, ir, req.EvaluateConfig.EvalMetrics)
				if err != nil {
					mu.Lock()
					results = append(results, &EvalCaseResult{
						EvalID:          ir.EvalCaseID,
						FinalEvalStatus: EvalStatusFailed,
					})
					mu.Unlock()
					return
				}
				mu.Lock()
				results = append(results, result)
				mu.Unlock()
			}(inferenceResult)
		}
		wg.Wait()

		for _, result := range results {
			if !yield(result, nil) {
				return
			}
		}
	}
}

func (s *LocalEvalService) evaluateInferenceResult(
	ctx context.Context,
	ir InferenceResult,
	metrics []EvalMetric,
) (*EvalCaseResult, error) {
	if ir.Status == InferenceStatusFailure {
		return &EvalCaseResult{
			EvalID:          ir.EvalCaseID,
			FinalEvalStatus: EvalStatusFailed,
		}, nil
	}

	// Get expected invocations from the eval set.
	evalSet, err := s.evalSetsMgr.GetEvalSet(ctx, ir.AppName, ir.EvalSetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get eval set: %w", err)
	}

	evalCase := GetEvalCaseFromEvalSet(evalSet, ir.EvalCaseID)
	if evalCase == nil {
		return nil, NewNotFoundError("eval case", ir.EvalCaseID)
	}

	expectedInvocations := evalCase.Conversation

	// Copy rubrics from eval case to actual invocations.
	for i := range ir.Inferences {
		if i < len(evalCase.Conversation) {
			ir.Inferences[i].Rubrics = evalCase.Conversation[i].Rubrics
		}
	}

	var metricResults []EvalMetricResult
	var allPerInvocationResults []EvalMetricResultPerInvocation
	allPassed := true

	for _, metric := range metrics {
		var evaluator Evaluator
		if s.llm != nil && s.registry.HasMetric(metric.MetricName) {
			evaluator, err = s.registry.GetEvaluatorWithLLM(metric, s.llm)
		} else {
			evaluator, err = s.registry.GetEvaluator(metric)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get evaluator for metric %q: %w", metric.MetricName, err)
		}

		result, err := evaluator.EvaluateInvocations(
			ctx,
			ir.Inferences,
			expectedInvocations,
			evalCase.ConversationScenario,
		)
		if err != nil {
			return nil, fmt.Errorf("evaluator failed for metric %q: %w", metric.MetricName, err)
		}

		metricResult := EvalMetricResult{
			EvalMetric: metric,
			Score:      result.OverallScore,
			EvalStatus: result.OverallEvalStatus,
		}
		if result.OverallEvalStatus != EvalStatusPassed {
			allPassed = false
		}

		for _, pir := range result.PerInvocationResults {
			r := EvalMetricResultPerInvocation{
				ActualInvocation:   pir.ActualInvocation,
				ExpectedInvocation: pir.ExpectedInvocation,
				EvalMetricResults: []EvalMetricResult{
					{
						EvalMetric: metric,
						Score:      pir.Score,
						EvalStatus: pir.EvalStatus,
					},
				},
			}
			allPerInvocationResults = append(allPerInvocationResults, r)
		}

		if len(result.OverallRubricScores) > 0 {
			metricResult.Details = EvalMetricResultDetails{
				RubricScores: result.OverallRubricScores,
			}
		}

		metricResults = append(metricResults, metricResult)
	}

	finalStatus := EvalStatusPassed
	if !allPassed {
		finalStatus = EvalStatusFailed
	}

	caseResult := &EvalCaseResult{
		EvalID:                        ir.EvalCaseID,
		FinalEvalStatus:               finalStatus,
		OverallEvalMetricResults:      metricResults,
		EvalMetricResultPerInvocation: allPerInvocationResults,
		SessionID:                     ir.SessionID,
	}

	// Save results if a results manager is configured.
	if s.resultsMgr != nil {
		if err := s.resultsMgr.SaveEvalSetResult(ctx, ir.AppName, ir.EvalSetID, []EvalCaseResult{*caseResult}); err != nil {
			return nil, fmt.Errorf("failed to save eval set result: %w", err)
		}
	}

	return caseResult, nil
}

// GetSessionID generates a unique session ID.
func GetSessionID() string {
	return EvalSessionIDPrefix + uuid.NewString()
}

// filterEvalCases returns eval cases filtered by IDs, or all if IDs is empty.
func filterEvalCases(cases []EvalCase, ids []string) []EvalCase {
	if len(ids) == 0 {
		return cases
	}
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var filtered []EvalCase
	for _, c := range cases {
		if idSet[c.EvalID] {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// Compile-time interface check.
var _ BaseEvalService = (*LocalEvalService)(nil)
