package eval

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// AgentEvaluator wraps an agent and evaluates it against eval sets.
type AgentEvaluator struct {
	agentRunner           AgentRunner
	evalSetsMgr           EvalSetsManager
	evalResultsMgr        EvalSetResultsManager
	registry              *MetricEvaluatorRegistry
	llm                   model.LLM
	userSimulatorProvider UserSimulatorProvider
}

// AgentRunner is the interface for running an agent to generate responses.
type AgentRunner interface {
	// RunForSession runs the agent for a given session and user message,
	// returning the events generated.
	RunForSession(ctx context.Context, sessionID, userID, appName string, userContent *genai.Content) ([]*session.Event, error)
}

// UserSimulatorProvider provides user simulators for dynamic conversation eval.
type UserSimulatorProvider interface {
	Provide(evalCase EvalCase) (UserSimulator, error)
}

// UserSimulator generates user messages during dynamic conversation evaluation.
type UserSimulator interface {
	GetNextUserMessage(ctx context.Context, events []*session.Event) (*NextUserMessage, error)
}

// NextUserMessage is the response from a user simulator.
type NextUserMessage struct {
	Status      UserSimulatorStatus
	UserMessage *genai.Content
}

// UserSimulatorStatus represents the status of a user simulator response.
type UserSimulatorStatus string

const (
	// UserSimulatorStatusSuccess indicates the simulator produced a user message.
	UserSimulatorStatusSuccess UserSimulatorStatus = "success"
	// UserSimulatorStatusNoMessageGenerated indicates no message was generated.
	UserSimulatorStatusNoMessageGenerated UserSimulatorStatus = "no_message_generated"
	// UserSimulatorStatusTurnLimitReached indicates the turn limit was reached.
	UserSimulatorStatusTurnLimitReached UserSimulatorStatus = "turn_limit_reached"
	// UserSimulatorStatusStopSignalDetected indicates a stop signal was detected.
	UserSimulatorStatusStopSignalDetected UserSimulatorStatus = "stop_signal_detected"
)

// EvaluateOptions configures an AgentEvaluator.Evaluate run.
type EvaluateOptions struct {
	// AppName is the application name for the eval set.
	AppName string
	// EvalSetID is the ID of the eval set to evaluate.
	EvalSetID string
	// Config is the eval configuration (metrics, thresholds).
	Config EvalConfig
	// NumRuns is the number of inference runs per eval case. Defaults to 1.
	NumRuns int
	// Output is where the tabwriter results table is written. Defaults to os.Stdout.
	Output io.Writer
}

// NewAgentEvaluator creates a new AgentEvaluator.
func NewAgentEvaluator(
	agentRunner AgentRunner,
	evalSetsMgr EvalSetsManager,
	evalResultsMgr EvalSetResultsManager,
	registry *MetricEvaluatorRegistry,
	llm model.LLM,
) *AgentEvaluator {
	if registry == nil {
		registry = DefaultMetricEvaluatorRegistry()
	}
	return &AgentEvaluator{
		agentRunner:    agentRunner,
		evalSetsMgr:    evalSetsMgr,
		evalResultsMgr: evalResultsMgr,
		registry:       registry,
		llm:            llm,
	}
}

// WithUserSimulatorProvider sets the user simulator provider for dynamic
// conversation evaluation and returns the receiver for chaining.
func (a *AgentEvaluator) WithUserSimulatorProvider(provider UserSimulatorProvider) *AgentEvaluator {
	a.userSimulatorProvider = provider
	return a
}

// Evaluate evaluates all eval cases in an eval set.
func (a *AgentEvaluator) Evaluate(
	ctx context.Context,
	appName, evalSetID string,
	config EvalConfig,
) (*EvalSetResult, error) {
	return a.EvaluateWithOptions(ctx, EvaluateOptions{
		AppName:   appName,
		EvalSetID: evalSetID,
		Config:    config,
		NumRuns:   1,
		Output:    os.Stdout,
	})
}

// EvaluateWithOptions evaluates all eval cases in an eval set with the given
// options. It runs inference NumRuns times per eval case, evaluates metrics,
// prints a results table using text/tabwriter, and returns the aggregated
// result. Returns an error if any eval case fails.
func (a *AgentEvaluator) EvaluateWithOptions(
	ctx context.Context,
	opts EvaluateOptions,
) (*EvalSetResult, error) {
	if opts.NumRuns <= 0 {
		opts.NumRuns = 1
	}
	if opts.Output == nil {
		opts.Output = os.Stdout
	}

	evalSet, err := a.evalSetsMgr.GetEvalSet(ctx, opts.AppName, opts.EvalSetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get eval set: %w", err)
	}

	metrics := GetEvalMetricsFromConfig(opts.Config)
	var caseResults []EvalCaseResult
	var failures []string

	for _, evalCase := range evalSet.EvalCases {
		var bestResult *EvalCaseResult
		for run := 0; run < opts.NumRuns; run++ {
			caseResult, err := a.evaluateCase(ctx, opts.AppName, evalCase, metrics)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate case %q (run %d): %w", evalCase.EvalID, run+1, err)
			}
			if bestResult == nil || caseResult.FinalEvalStatus == EvalStatusPassed {
				bestResult = caseResult
			}
		}
		if bestResult == nil {
			return nil, fmt.Errorf("no result generated for eval case %q", evalCase.EvalID)
		}
		caseResults = append(caseResults, *bestResult)
		if bestResult.FinalEvalStatus != EvalStatusPassed {
			failures = append(failures, bestResult.EvalID)
		}
	}

	result := CreateEvalSetResult(opts.AppName, opts.EvalSetID, caseResults)

	// Print results table using tabwriter.
	a.printResultsTable(opts.Output, opts.AppName, opts.EvalSetID, caseResults, metrics)

	if a.evalResultsMgr != nil {
		if err := a.evalResultsMgr.SaveEvalSetResult(ctx, opts.AppName, opts.EvalSetID, caseResults); err != nil {
			return nil, fmt.Errorf("failed to save eval set result: %w", err)
		}
	}

	if len(failures) > 0 {
		return result, fmt.Errorf("eval cases failed: %v", failures)
	}

	return result, nil
}

// printResultsTable writes a results summary table to the given writer using
// text/tabwriter for aligned columns.
func (a *AgentEvaluator) printResultsTable(
	w io.Writer,
	appName, evalSetID string,
	caseResults []EvalCaseResult,
	metrics []EvalMetric,
) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "App:\t%s\n", appName)
	_, _ = fmt.Fprintf(tw, "Eval Set:\t%s\n", evalSetID)
	_, _ = fmt.Fprintf(tw, "Cases:\t%d\n", len(caseResults))
	_, _ = fmt.Fprintln(tw, "")
	_, _ = fmt.Fprintf(tw, "Eval ID\tStatus\t")
	for _, m := range metrics {
		_, _ = fmt.Fprintf(tw, "%s\t", m.MetricName)
	}
	_, _ = fmt.Fprintln(tw)

	for _, cr := range caseResults {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t", cr.EvalID, cr.FinalEvalStatus)
		for _, m := range metrics {
			for _, mr := range cr.OverallEvalMetricResults {
				if mr.MetricName == m.MetricName {
					scoreStr := "N/A"
					if mr.Score != nil {
						scoreStr = fmt.Sprintf("%.2f", *mr.Score)
					}
					_, _ = fmt.Fprintf(tw, "%s (%s)\t", scoreStr, mr.EvalStatus)
					break
				}
			}
		}
		_, _ = fmt.Fprintln(tw)
	}
	_ = tw.Flush()
}

func (a *AgentEvaluator) evaluateCase(
	ctx context.Context,
	appName string,
	evalCase EvalCase,
	metrics []EvalMetric,
) (*EvalCaseResult, error) {
	// Generate actual invocations by running the agent.
	actualInvocations, err := a.generateInvocations(ctx, appName, evalCase)
	if err != nil {
		return nil, fmt.Errorf("failed to generate invocations: %w", err)
	}

	// Determine expected invocations.
	expectedInvocations := evalCase.Conversation

	// Evaluate each metric.
	var metricResults []EvalMetricResult
	var allPerInvocationResults []EvalMetricResultPerInvocation

	allPassed := true

	for _, metric := range metrics {
		var evaluator Evaluator
		var err error

		if a.llm != nil && a.registry.HasMetric(metric.MetricName) {
			// Check if this metric needs an LLM.
			evaluator, err = a.registry.GetEvaluatorWithLLM(metric, a.llm)
		} else {
			evaluator, err = a.registry.GetEvaluator(metric)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to get evaluator for metric %q: %w", metric.MetricName, err)
		}
		if evaluator == nil {
			return nil, fmt.Errorf("nil evaluator returned for metric %q", metric.MetricName)
		}

		result, err := evaluator.EvaluateInvocations(
			ctx,
			actualInvocations,
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

		// Build per-invocation results.
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

	return &EvalCaseResult{
		EvalID:                        evalCase.EvalID,
		FinalEvalStatus:               finalStatus,
		OverallEvalMetricResults:      metricResults,
		EvalMetricResultPerInvocation: allPerInvocationResults,
	}, nil
}

func (a *AgentEvaluator) generateInvocations(
	ctx context.Context,
	appName string,
	evalCase EvalCase,
) ([]Invocation, error) {
	if len(evalCase.Conversation) > 0 {
		return GenerateStaticInvocations(ctx, a.agentRunner, appName, evalCase.Conversation)
	}

	if evalCase.ConversationScenario != nil && a.userSimulatorProvider != nil {
		return GenerateDynamicInvocations(ctx, a.agentRunner, appName, evalCase, a.userSimulatorProvider)
	}

	return nil, fmt.Errorf("no conversation or conversation scenario provided for eval case %q", evalCase.EvalID)
}
