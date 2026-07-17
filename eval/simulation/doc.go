// Package simulation provides user simulator implementations for generating
// user interactions during evaluation.
//
// The package supports two types of user simulators:
//   - StaticUserSimulator: Returns messages from a fixed list of invocations.
//   - LlmBackedUserSimulator: Uses an LLM to generate user messages based on
//     a conversation plan and optional persona.
//
// A UserSimulatorProvider dispatches to the correct simulator based on the
// eval case configuration.
package simulation
