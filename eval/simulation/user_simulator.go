package simulation

import (
	"errors"
)

// BaseUserSimulatorConfig is the base configuration for user simulators.
// The Type field acts as a discriminator for config subtypes.
type BaseUserSimulatorConfig struct {
	Type string `json:"type"`
}

// ErrSimulationEvaluatorNotImplemented is returned by GetSimulationEvaluator
// when the simulator does not provide an evaluator.
var ErrSimulationEvaluatorNotImplemented = errors.New("simulation evaluator not implemented for this simulator type")
