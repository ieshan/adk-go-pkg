# Evaluation Framework

The `eval` package provides evaluation tooling for ADK-Go agents, mirroring the [ADK Python evaluation package](https://google.github.io/adk-docs/evaluate/). It includes data models for eval sets and cases, evaluator interfaces and implementations, eval set management, user simulation, and a local eval service for orchestrating inference and evaluation.

## Overview

The evaluation framework follows a pipeline architecture:

```
Eval Sets (JSON) → Inference (run agent) → Evaluation (score metrics) → Results
```

1. **Eval Sets** are JSON files containing eval cases with invocations describing expected user inputs, tool calls, and final responses.
2. **Inference** runs the agent for each user message and collects events.
3. **Evaluation** compares actual agent behavior to expectations using registered evaluators, producing scores.
4. **Results** are aggregated per-invocation and overall, with pass/fail status based on thresholds.

### Static vs Dynamic Conversations

- **Static conversations**: The eval case contains a fixed list of invocations with predefined user messages. The agent is run for each user message and the output is compared against expected results.
- **Dynamic conversations**: The eval case contains a `ConversationScenario` with a starting prompt and conversation plan. An LLM-backed user simulator generates user messages. (Dynamic conversation inference is not yet implemented in the local eval service.)

## Core Data Models

### EvalSet

An `EvalSet` is a collection of `EvalCase` objects stored as a JSON file.

```go
type EvalSet struct {
    EvalSetID         string     `json:"evalSetId"`
    Name              string     `json:"name,omitempty"`
    Description       string     `json:"description,omitempty"`
    EvalCases         []EvalCase `json:"evalCases,omitempty"`
    CreationTimestamp float64    `json:"creationTimestamp,omitempty"`
}
```

### EvalCase

An `EvalCase` represents a single test case. It requires exactly one of `Conversation` (static) or `ConversationScenario` (dynamic) — validated during JSON unmarshaling.

```go
type EvalCase struct {
    EvalID               string                `json:"evalId"`
    Conversation         []Invocation          `json:"conversation,omitempty"`
    ConversationScenario *ConversationScenario `json:"conversationScenario,omitempty"`
    SessionInput         *SessionInput         `json:"sessionInput,omitempty"`
    CreationTimestamp    float64               `json:"creationTimestamp,omitempty"`
    Rubrics              []Rubric              `json:"rubrics,omitempty"`
    FinalSessionState    SessionState          `json:"finalSessionState,omitempty"`
}
```

### Invocation

An `Invocation` captures a single turn: the user's input, the agent's final response, and intermediate data (tool calls, events).

```go
type Invocation struct {
    InvocationID     string           `json:"invocationId,omitempty"`
    UserContent      *genai.Content   `json:"userContent,omitempty"`
    FinalResponse    *genai.Content   `json:"finalResponse,omitempty"`
    IntermediateData IntermediateData `json:"-"`
    Rubrics          []Rubric         `json:"rubrics,omitempty"`
    AppDetails       *AppDetails      `json:"appDetails,omitempty"`
}
```

### IntermediateData

`IntermediateData` is an interface with two implementations:

- **`InvocationEventsData`** (current format): Stores a list of `InvocationEvent` objects, each containing `Author` and `Content`.
- **`LegacyIntermediateData`** (legacy format): Stores `ToolUses` and `ToolResponses` as flat slices.

The `UnmarshalIntermediateData` function auto-detects the format by checking for `"events"` or `"tool_uses"`/`"tool_responses"` keys in the JSON.

### SessionInput

`SessionInput` holds the initial session state for an eval case:

```go
type SessionInput struct {
    AppState  map[string]any `json:"appState,omitempty"`
    UserState map[string]any `json:"userState,omitempty"`
    SessionState SessionState `json:"sessionState,omitempty"`
}
```

### AppDetails

`AppDetails` captures metadata about the app's agents (names, instructions, tool declarations) for use by evaluators that need context about the agent under test.

## EvalConfig

`EvalConfig` configures how evaluation is performed. It is passed to `AgentEvaluator.Evaluate` or `LocalEvalService.Evaluate`.

```go
type EvalConfig struct {
    Criteria           map[string]json.RawMessage   `json:"criteria,omitempty"`
    CustomMetrics      map[string]CustomMetricConfig `json:"customMetrics,omitempty"`
    UserSimulatorConfig json.RawMessage             `json:"userSimulatorConfig,omitempty"`
}
```

`Criteria` maps metric names to criterion objects (or a simple threshold float). Use `GetEvalMetricsFromConfig` to parse criteria into a slice of `EvalMetric` structs.

### EvalMetric

```go
type EvalMetric struct {
    MetricName         string         `json:"metricName"`
    Threshold          *float64       `json:"threshold,omitempty"`
    Criterion          BaseCriterion  `json:"-"`
    CustomFunctionPath string         `json:"customFunctionPath,omitempty"`
}
```

## Criteria

Criteria provide per-metric configuration beyond a simple threshold. All criterion types implement the `BaseCriterion` interface:

```go
type BaseCriterion interface {
    GetThreshold() *float64
    GetIncludeIntermediateResponsesInFinal() bool
    IsBaseCriterion()
}
```

### Criterion Subtypes

| Type | Extends | Additional Fields | Used By |
|------|---------|-------------------|---------|
| `BaseCriterionImpl` | — | `Threshold`, `IncludeIntermediateResponsesInFinal` | Trajectory, Rouge |
| `LlmAsAJudgeCriterion` | `BaseCriterionImpl` | `JudgeModelOptions` (judge model, num samples) | FinalResponseMatchV2, Response evaluation |
| `RubricsBasedCriterion` | `LlmAsAJudgeCriterion` | `Rubrics []Rubric` | Rubric-based evaluators |
| `HallucinationsCriterion` | `LlmAsAJudgeCriterion` | `EvaluateIntermediateNLResponses` | HallucinationsV1 |
| `ToolTrajectoryCriterion` | `BaseCriterionImpl` | `MatchType` (EXACT, IN_ORDER, ANY_ORDER) | Trajectory |
| `LlmBackedUserSimulatorCriterion` | `LlmAsAJudgeCriterion` | `StopSignal` | User simulator quality |

### Polymorphic JSON

`EvalMetric` uses custom JSON unmarshaling to detect the criterion subtype by field presence:
1. `rubrics` → `RubricsBasedCriterion`
2. `matchType` → `ToolTrajectoryCriterion`
3. `stopSignal` → `LlmBackedUserSimulatorCriterion`
4. `evaluateIntermediateNlResponses` → `HallucinationsCriterion`
5. `judgeModelOptions` → `LlmAsAJudgeCriterion`
6. Fallback → `BaseCriterionImpl`

### JudgeModelOptions

```go
type JudgeModelOptions struct {
    JudgeModel        string          `json:"judgeModel,omitempty"`   // default: "gemini-2.5-flash"
    JudgeModelConfig  json.RawMessage `json:"judgeModelConfig,omitempty"`
    NumSamples        int             `json:"numSamples,omitempty"`   // default: 5
}
```

## Rubrics

Rubrics are testable criteria used by rubric-based evaluators:

```go
type Rubric struct {
    RubricID      string        `json:"rubricId"`
    RubricContent RubricContent `json:"rubricContent"`
    Description   string        `json:"description,omitempty"`
    Type          string        `json:"type,omitempty"`
}

type RubricContent struct {
    TextProperty string `json:"textProperty,omitempty"`
}
```

`RubricScore` holds the result of assessing a single rubric:

```go
type RubricScore struct {
    RubricID   string   `json:"rubricId"`
    Rationale  string   `json:"rationale,omitempty"`
    Score      *float64 `json:"score,omitempty"`
}
```

## Built-in Evaluators

The registry registers 13 built-in metrics via `DefaultMetricEvaluatorRegistry()`:

| Metric Name | LLM Required | Description | Value Range |
|-------------|:---:|-------------|:---:|
| `tool_trajectory_avg_score` | No | Compares tool call trajectories (expected vs actual) by tool name and arguments. | [0, 1] |
| `response_evaluation_score` | No* | Evaluates response coherence. | [1, 5] |
| `response_match_score` | No | Evaluates final response match using ROUGE-1 F-measure. | [0, 1] |
| `safety_v1` | No* | Evaluates safety (harmlessness) of agent responses. | [0, 1] |
| `final_response_match_v2` | Yes | Evaluates final response match using LLM as judge. | [0, 1] |
| `rubric_based_final_response_quality_v1` | Yes | Assesses final response against rubrics using LLM as judge. | [0, 1] |
| `hallucinations_v1` | Yes | Assesses whether responses contain false/unsupported claims using LLM as judge. | [0, 1] |
| `rubric_based_tool_use_quality_v1` | Yes | Assesses tool usage against rubrics using LLM as judge. | [0, 1] |
| `per_turn_user_simulator_quality_v1` | Yes | Evaluates user simulator message quality per turn. | [0, 1] |
| `multi_turn_task_success_v1` | No* | Evaluates if the agent achieved conversation goals. | [0, 1] |
| `multi_turn_trajectory_quality_v1` | No* | Evaluates overall multi-turn trajectory quality. | [0, 1] |
| `multi_turn_tool_use_quality_v1` | No* | Evaluates function calls during multi-turn conversations. | [0, 1] |
| `rubric_based_multi_turn_trajectory_quality_v1` | Yes | Evaluates multi-turn trajectory against rubrics using LLM as judge. | [0, 1] |

**\*** Metrics marked with asterisks use `StubVertexAiEvalFacade` which returns `NOT_EVALUATED` when GCP dependencies are not configured. To enable them, set `GOOGLE_CLOUD_PROJECT` and `GOOGLE_CLOUD_LOCATION` and provide a real `VertexAiEvalFacade` implementation.

### MatchType (Trajectory)

```go
const (
    MatchExact    MatchType = "EXACT"      // exact sequence match
    MatchInOrder  MatchType = "IN_ORDER"   // tools appear in expected order (extras allowed)
    MatchAnyOrder MatchType = "ANY_ORDER"  // tools match regardless of order
)
```

## Evaluator Interface

```go
type Evaluator interface {
    EvaluateInvocations(
        ctx context.Context,
        actualInvocations []Invocation,
        expectedInvocations []Invocation,
        conversationScenario *ConversationScenario,
    ) (*EvaluationResult, error)
}
```

### EvaluationResult

```go
type EvaluationResult struct {
    OverallScore          *float64              // aggregated score
    OverallEvalStatus     EvalStatus            // PASSED, FAILED, or NOT_EVALUATED
    PerInvocationResults  []PerInvocationResult
    OverallRubricScores   []RubricScore
}
```

### PerInvocationResult

```go
type PerInvocationResult struct {
    ActualInvocation   Invocation
    ExpectedInvocation *Invocation
    Score              *float64
    EvalStatus         EvalStatus
    RubricScores       []RubricScore
}
```

### EvalStatus

```go
const (
    EvalStatusPassed       EvalStatus = "PASSED"
    EvalStatusFailed       EvalStatus = "FAILED"
    EvalStatusNotEvaluated EvalStatus = "NOT_EVALUATED"
)
```

## LlmAsJudgeEvaluator

`LlmAsJudgeEvaluator` is the base struct for all LLM-based auto-rater evaluators. Concrete evaluators embed it and override function fields:

```go
type LlmAsJudgeEvaluator struct {
    FormatAutoRaterPrompt             func(ctx context.Context, actual Invocation, expected *Invocation) (string, error)
    ConvertAutoRaterResponseToScore   func(response *model.LLMResponse) AutoRaterScore
    AggregatePerInvocationSamplesFunc func(samples []PerInvocationResult) PerInvocationResult
    AggregateInvocationResultsFunc    func(perInvocation []PerInvocationResult) EvaluationResult
}
```

- **numSamples**: Each invocation is evaluated N times (default 5). Samples are aggregated via `AggregatePerInvocationSamplesFunc` (default: first successful result).
- **Aggregation across invocations**: `AggregateInvocationResultsFunc` (default: average scores).

## Registry

`MetricEvaluatorRegistry` manages evaluator factory registration and retrieval:

```go
// Create with all built-in evaluators
registry := eval.DefaultMetricEvaluatorRegistry()

// Register a custom evaluator (no LLM needed)
registry.RegisterEvaluator(metricInfo, factoryFunc)

// Register a custom evaluator that requires an LLM
registry.RegisterEvaluatorWithLLM(metricInfo, factoryFuncWithLLM)

// Check if a metric is registered
registry.HasMetric("my_custom_metric")

// Get an evaluator (no LLM)
evaluator, err := registry.GetEvaluator(evalMetric)

// Get an evaluator (with LLM)
evaluator, err := registry.GetEvaluatorWithLLM(evalMetric, llm)

// List all registered metrics
metrics := registry.ListAllMetrics()
```

## Custom Evaluators

Implement custom metrics with `CustomMetricEvaluator`:

```go
type CustomMetricFunc func(
    ctx context.Context,
    evalMetric EvalMetric,
    actualInvocations []Invocation,
    expectedInvocations []Invocation,
    conversationScenario *ConversationScenario,
) (*EvaluationResult, error)

evaluator := eval.NewCustomMetricEvaluator(evalMetric, myFunc)
```

## Model Plugins

The eval package provides two model plugins (implementing
`llmagent.BeforeModelCallback` and `llmagent.AfterModelCallback`) that can be
registered on an LLM agent to support eval workflows:

### RequestIntercepterPlugin

```go
plugin := eval.NewRequestIntercepterPlugin()
```

Captures LLM requests keyed by UUID for `AppDetails` generation. The plugin
stores a UUID in session state under `__request_intercepter_id__` and caches
the original `*model.LLMRequest` so it can be retrieved after the response is
produced. Used internally by the eval infrastructure to reconstruct what the
agent sent to the model.

### EnsureRetryOptionsPlugin

```go
plugin := &eval.EnsureRetryOptionsPlugin{}
```

Adds default HTTP retry options to LLM requests before they are sent, mirroring
the Python ADK's `HttpRetryOptions` behavior. **Note:** The Go `genai` SDK does
not currently expose `HttpRetryOptions` on `GenerateContentConfig`, so this
plugin only ensures the config is initialized — it cannot set retry parameters
yet. When the Go SDK adds retry configuration support, this plugin should be
updated to match the Python defaults.

You can also call `eval.AddDefaultRetryOptionsIfNotPresent(req)` directly on a
`*model.LLMRequest` without registering the plugin.

## Eval Set Managers

### EvalSetsManager Interface

```go
type EvalSetsManager interface {
    GetEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error)
    CreateEvalSet(ctx context.Context, appName, evalSetID string) (*EvalSet, error)
    ListEvalSets(ctx context.Context, appName string) ([]string, error)
    GetEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) (*EvalCase, error)
    AddEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error
    UpdateEvalCase(ctx context.Context, appName, evalSetID string, evalCase EvalCase) error
    DeleteEvalCase(ctx context.Context, appName, evalSetID, evalCaseID string) error
}
```

### Implementations

- **`InMemoryEvalSetsManager`** — Thread-safe in-memory storage. Useful for testing.
- **`LocalEvalSetsManager`** — File-based storage. Eval sets are stored as `.evalset.json` files under `<agentsDir>/<appName>/eval/<evalSetID>.evalset.json`.

### Helper Functions

- `GetEvalCaseFromEvalSet(evalSet, evalCaseID)` — Find a case by ID (returns nil if not found).
- `AddEvalCaseToEvalSet(evalSet, evalCase)` — Add a case (rejects duplicates).
- `UpdateEvalCaseInEvalSet(evalSet, updatedCase)` — Replace a case by ID.
- `DeleteEvalCaseFromEvalSet(evalSet, evalCaseID)` — Remove a case by ID.

## Results Managers

### EvalSetResultsManager Interface

```go
type EvalSetResultsManager interface {
    SaveEvalSetResult(ctx context.Context, appName, evalSetID string, results []EvalCaseResult) error
    GetEvalSetResult(ctx context.Context, appName, evalSetResultID string) (*EvalSetResult, error)
    ListEvalSetResults(ctx context.Context, appName string) ([]string, error)
}
```

### Implementations

- **`LocalEvalSetResultsManager`** — File-based storage. Results are stored as `.evalset_result.json` files under `<agentsDir>/<appName>/.adk/eval_history/`.

### Result Types

- **`EvalCaseResult`** — Per-case results with `FinalEvalStatus`, `OverallEvalMetricResults`, and `EvalMetricResultPerInvocation`.
- **`EvalSetResult`** — Aggregated results for an entire eval set with `EvalSetResultID`, `EvalCaseResults`, and `CreationTimestamp`.

## AgentEvaluator

`AgentEvaluator` wraps an agent runner and evaluates it against eval sets:

```go
type AgentRunner interface {
    RunForSession(ctx context.Context, sessionID, userID, appName string, userContent *genai.Content) ([]*session.Event, error)
}

evaluator := eval.NewAgentEvaluator(
    agentRunner,     // implements AgentRunner
    evalSetsMgr,     // EvalSetsManager
    evalResultsMgr,  // EvalSetResultsManager (optional, nil to skip saving)
    registry,        // *MetricEvaluatorRegistry (nil uses default)
    llm,             // model.LLM for LLM-based evaluators (optional)
)

result, err := evaluator.Evaluate(ctx, "my-app", "my-eval-set", config)
```

The `Evaluate` method:
1. Fetches the eval set from the manager.
2. For each eval case, runs the agent for each user message (static conversations only).
3. Converts events to invocations via `ConvertEventsToInvocation`.
4. Evaluates each metric using registered evaluators (with or without LLM).
5. Aggregates results and saves if a results manager is configured.

## LocalEvalService

`LocalEvalService` implements `BaseEvalService` with parallel inference and evaluation:

```go
type BaseEvalService interface {
    PerformInference(ctx context.Context, req *InferenceRequest) iter.Seq2[*InferenceResult, error]
    Evaluate(ctx context.Context, req *EvaluateRequest) iter.Seq2[*EvalCaseResult, error]
}
```

### Usage

```go
service := eval.NewLocalEvalService(
    evalSetsMgr,     // EvalSetsManager
    resultsMgr,      // EvalSetResultsManager (optional)
    registry,        // *MetricEvaluatorRegistry (nil uses default)
    agentRunner,     // AgentRunner
    llm,             // model.LLM (optional, for LLM-based evaluators)
)

// Run inference
for result, err := range service.PerformInference(ctx, &eval.InferenceRequest{
    AppName:   "my-app",
    EvalSetID: "my-eval-set",
    InferenceConfig: eval.InferenceConfig{Parallelism: 4},
}) {
    if err != nil { /* handle */ }
    // result is *InferenceResult
}

// Evaluate results
for caseResult, err := range service.Evaluate(ctx, &eval.EvaluateRequest{
    InferenceResults: inferenceResults,
    EvaluateConfig: eval.EvaluateConfig{
        EvalMetrics:  metrics,
        Parallelism:   4,
    },
}) {
    if err != nil { /* handle */ }
    // caseResult is *EvalCaseResult
}
```

### InferenceConfig

```go
type InferenceConfig struct {
    Labels             map[string]string
    Parallelism        int  // default: 4
    UseLive            bool // not yet implemented
    LiveTimeoutSeconds int  // default: 300
}
```

### EvaluateConfig

```go
type EvaluateConfig struct {
    EvalMetrics  []EvalMetric
    Parallelism  int  // default: 4
}
```

## Simulation Subpackage (`eval/simulation/`)

The simulation subpackage provides user simulator implementations for dynamic conversations.

### UserSimulator Interface

```go
type UserSimulator interface {
    GetNextUserMessage(ctx context.Context, events []*genai.Content) (*NextUserMessage, error)
    GetSimulationEvaluator() (eval.Evaluator, error)
}
```

### NextUserMessage

```go
type NextUserMessage struct {
    Status      Status          // success, no_message_generated, turn_limit_reached, stop_signal_detected
    UserMessage *genai.Content
}
```

### Implementations

- **`StaticUserSimulator`** — Returns messages from a fixed list of invocations. Used for static eval cases.
- **`LlmBackedUserSimulator`** — Uses an LLM to generate user messages based on a conversation plan and optional persona. Configurable via `LlmBackedUserSimulatorConfig`:

```go
type LlmBackedUserSimulatorConfig struct {
    Type                  string                       // discriminator: "llm_backed"
    Model                 string                       // default: "gemini-2.5-flash"
    ModelConfiguration    *genai.GenerateContentConfig
    MaxAllowedInvocations int                          // default: 20
    CustomInstructions    string                       // must contain {{ stop_signal }}, {{ conversation_plan }}, {{ conversation_history }}
    IncludeFunctionCalls  bool
}
```

### UserSimulatorProvider

`UserSimulatorProvider` dispatches to the correct simulator based on eval case data:

```go
provider := simulation.NewUserSimulatorProvider(llm, simulation.DefaultLlmBackedUserSimulatorConfig())
simulator, err := provider.Provide(evalCase)
```

- If `evalCase.Conversation` is set → returns `StaticUserSimulator`.
- If `evalCase.ConversationScenario` is set → returns `LlmBackedUserSimulator`.

## User Personas

`UserPersona` aggregates multiple behaviors for the user simulator:

```go
type UserPersona struct {
    ID          string
    Description string
    Behaviors   []UserBehavior
}

type UserBehavior struct {
    Name                string
    Description         string
    BehaviorInstructions []string
    ViolationRubrics    []string
}
```

`UserPersonaRegistry` manages persona registration and lookup:

```go
registry := eval.NewUserPersonaRegistry()
registry.RegisterPersona("expert", persona)
persona, err := registry.GetPersona("expert")
all := registry.GetRegisteredPersonas()
```

### Pre-built Personas

The framework ships with three pre-built personas (mirroring the Python ADK
`_PreBuiltPersonas`), available via the `PreBuiltPersonas` map or the
`GetDefaultPersonaRegistry()` convenience constructor:

| Persona ID | Description |
|------------|-------------|
| `EXPERT` | Knows exactly what they want; expects efficient execution with little patience for unnecessary interactions. |
| `NOVICE` | Solving a problem they don't fully understand; relies on the agent for guidance. Patient but cannot troubleshoot or correct mistakes. |
| `EVALUATOR` | Aims to assess whether the agent accomplishes the goals in the conversation plan. |

```go
// Get a registry pre-populated with EXPERT, NOVICE, and EVALUATOR.
registry := eval.GetDefaultPersonaRegistry()
persona, err := registry.GetPersona("EXPERT")
```

Each persona is composed from pre-built `UserBehavior` values such as
`AdvanceDetailOriented`, `AdvanceGoalOriented`, `AnswerRelevantOnly`,
`AnswerAll`, `DoNotCorrectAgent`, `EndNoTroubleshooting`,
`ToneProfessional`, and `ToneConversational`.

## Conversation Scenarios

`ConversationScenario` describes a dynamic conversation for user simulation:

```go
type ConversationScenario struct {
    StartingPrompt     string       // fixed first user message
    ConversationPlan   string       // plan the simulator follows
    UserPersona        *UserPersona  // optional persona
}
```

`ConversationGenerationConfig` configures scenario generation (requires GCP):

```go
type ConversationGenerationConfig struct {
    Count                int    // number of scenarios to generate
    GenerationInstruction string // optional natural language goal
    EnvironmentContext    string // backend data/state for the agent's tools
    ModelName             string // Gemini model for generation
}
```

`VertexAiScenarioGenerationFacade` is the interface for scenario generation. `StubVertexAiScenarioGenerationFacade` returns an error indicating GCP is required.

## Vertex AI Stubs

The following evaluators use `StubVertexAiEvalFacade` which returns `NOT_EVALUATED` for all invocations when GCP is not configured:

- `safety_v1` (safety)
- `response_evaluation_score` (coherence)
- `multi_turn_task_success_v1`
- `multi_turn_trajectory_quality_v1`
- `multi_turn_tool_use_quality_v1`

To enable these metrics, provide a real `VertexAiEvalFacade` implementation that delegates to the Vertex AI Eval SDK.

## Utility Functions

| Function | Description |
|----------|-------------|
| `ConvertEventsToInvocation(events, userContent, appDetails)` | Converts session events for a single agent run into an `Invocation`. |
| `ConvertEventsToEvalInvocations(events, appDetails)` | Groups events by `InvocationID` and converts each group into an `Invocation`. |
| `GetAllToolCalls(invocation)` | Extracts all `FunctionCall` from intermediate data (legacy + events format). |
| `GetAllToolResponses(invocation)` | Extracts all `FunctionResponse` from intermediate data. |
| `GetAllToolCallsWithResponses(invocation)` | Pairs tool calls with their responses. |
| `GetTextFromContent(content)` | Extracts text from a `genai.Content`. |
| `GetEvalStatus(score, threshold)` | Returns `PASSED` if score >= threshold, else `FAILED`. |
| `GetSessionID()` | Generates a unique eval session ID (`___eval___session___<uuid>`). |

## JSON Eval Set File Format

Eval sets are stored as `.evalset.json` files:

```json
{
  "evalSetId": "basic-eval",
  "name": "Basic Evaluation",
  "description": "Basic trajectory and response evaluation",
  "evalCases": [
    {
      "evalId": "case-1",
      "conversation": [
        {
          "userContent": {
            "role": "user",
            "parts": [{"text": "What is the weather in London?"}]
          },
          "finalResponse": {
            "role": "model",
            "parts": [{"text": "The weather in London is 15°C and rainy."}]
          },
          "intermediateData": {
            "events": [
              {
                "author": "model",
                "content": {
                  "role": "model",
                  "parts": [{"functionCall": {"name": "get_weather", "args": {"city": "London"}}}]
                }
              },
              {
                "author": "user",
                "content": {
                  "role": "user",
                  "parts": [{"functionResponse": {"name": "get_weather", "response": {"temp": 15, "condition": "rainy"}}}]
                }
              }
            ]
          }
        }
      ],
      "rubrics": [
        {
          "rubricId": "r1",
          "rubricContent": {"textProperty": "Response mentions temperature"},
          "description": "Check if the response includes temperature"
        }
      ]
    }
  ]
}
```

## Usage Examples

### Creating an Eval Set Programmatically

```go
ctx := context.Background()
setsMgr := eval.NewInMemoryEvalSetsManager()
setsMgr.CreateEvalSet(ctx, "my-app", "basic-eval")
setsMgr.AddEvalCase(ctx, "my-app", "basic-eval", eval.EvalCase{
    EvalID: "case-1",
    Conversation: []eval.Invocation{
        {UserContent: genai.NewContentFromText("Hello", "user")},
    },
})
```

### Running Evaluation with AgentEvaluator

```go
evaluator := eval.NewAgentEvaluator(
    agentRunner, setsMgr, nil,
    eval.DefaultMetricEvaluatorRegistry(), llm,
)

config := eval.EvalConfig{
    Criteria: map[string]json.RawMessage{
        "tool_trajectory_avg_score": json.RawMessage(`{"threshold": 0.8}`),
        "final_response_match_v2":   json.RawMessage(`{"threshold": 0.7, "judgeModelOptions": {"judgeModel": "gemini-2.5-flash", "numSamples": 3}}`),
    },
}

result, err := evaluator.Evaluate(ctx, "my-app", "basic-eval", config)
```

### Using LocalEvalService for Inference + Evaluation

```go
service := eval.NewLocalEvalService(
    setsMgr, resultsMgr, nil, agentRunner, llm,
)

// Phase 1: Inference
var inferenceResults []eval.InferenceResult
for result, err := range service.PerformInference(ctx, &eval.InferenceRequest{
    AppName:         "my-app",
    EvalSetID:       "basic-eval",
    InferenceConfig: eval.InferenceConfig{Parallelism: 4},
}) {
    if err != nil { log.Fatal(err) }
    inferenceResults = append(inferenceResults, *result)
}

// Phase 2: Evaluation
for caseResult, err := range service.Evaluate(ctx, &eval.EvaluateRequest{
    InferenceResults: inferenceResults,
    EvaluateConfig: eval.EvaluateConfig{
        EvalMetrics:  eval.GetEvalMetricsFromConfig(config),
        Parallelism:   4,
    },
}) {
    if err != nil { log.Fatal(err) }
    fmt.Printf("Case %s: %s\n", caseResult.EvalID, caseResult.FinalEvalStatus)
}
```

### Registering a Custom Evaluator

```go
registry := eval.NewMetricEvaluatorRegistry()
registry.RegisterEvaluator(
    eval.MetricInfo{
        MetricName: "my_custom_metric",
        Description: "Checks if response contains a keyword",
        MetricValueInfo: &eval.MetricValueInfo{
            Interval: &eval.Interval{MinValue: 0, MaxValue: 1},
        },
    },
    func(em eval.EvalMetric) (eval.Evaluator, error) {
        return eval.NewCustomMetricEvaluator(em, func(
            ctx context.Context,
            em eval.EvalMetric,
            actual, expected []eval.Invocation,
            scenario *eval.ConversationScenario,
        ) (*eval.EvaluationResult, error) {
            // Your custom evaluation logic
            score := 1.0
            return &eval.EvaluationResult{
                OverallScore:      &score,
                OverallEvalStatus: eval.EvalStatusPassed,
            }, nil
        }), nil
    },
)
```

### File-Based Eval Set Management

```go
setsMgr := eval.NewLocalEvalSetsManager("./agents")
resultsMgr := eval.NewLocalEvalSetResultsManager("./agents")

// Eval sets stored at: ./agents/my-app/eval/basic-eval.evalset.json
// Results stored at:   ./agents/my-app/.adk/eval_history/*.evalset_result.json
```

## Compatibility

- **Go 1.26+** — Uses `iter.Seq2` and range-over-func
- **ADK-Go v2.0.0+** (`google.golang.org/adk/v2`)
- **GenAI v1.64.0** (`google.golang.org/genai`)
