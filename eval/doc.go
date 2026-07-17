// Package eval provides evaluation tooling for ADK Go agents.
//
// This package mirrors the features of the ADK Python evaluation package,
// including data models for eval sets and cases, evaluator interfaces and
// implementations, eval set management, user simulation, and a local eval
// service for orchestrating inference and evaluation.
//
// Eval sets are stored as JSON files (`.evalset.json`) and contain a
// collection of EvalCases, each with one or more Invocations describing
// expected user inputs, tool calls, and final responses. Evaluators compare
// actual agent behavior against these expectations and produce scores.
//
// The package supports both static conversations (fixed user messages) and
// dynamic conversations driven by an LLM-backed user simulator.
package eval
