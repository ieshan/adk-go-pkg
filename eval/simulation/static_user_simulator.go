package simulation

import (
	"context"

	"github.com/ieshan/adk-go-pkg/eval"
	"google.golang.org/adk/v2/session"
)

// StaticUserSimulator returns messages from a fixed list of invocations.
type StaticUserSimulator struct {
	staticConversation []eval.Invocation
	invocationIdx      int
}

// NewStaticUserSimulator creates a new StaticUserSimulator.
func NewStaticUserSimulator(staticConversation []eval.Invocation) *StaticUserSimulator {
	return &StaticUserSimulator{
		staticConversation: staticConversation,
	}
}

// GetNextUserMessage returns the next user message from the static conversation list.
func (s *StaticUserSimulator) GetNextUserMessage(ctx context.Context, events []*session.Event) (*eval.NextUserMessage, error) {
	if s.invocationIdx >= len(s.staticConversation) {
		return &eval.NextUserMessage{Status: eval.UserSimulatorStatusTurnLimitReached}, nil
	}
	nextUserContent := s.staticConversation[s.invocationIdx].UserContent
	s.invocationIdx++
	if nextUserContent == nil {
		return &eval.NextUserMessage{Status: eval.UserSimulatorStatusNoMessageGenerated}, nil
	}
	return &eval.NextUserMessage{
		Status:      eval.UserSimulatorStatusSuccess,
		UserMessage: nextUserContent,
	}, nil
}

// GetSimulationEvaluator returns an evaluator for the simulator's output.
func (s *StaticUserSimulator) GetSimulationEvaluator() (eval.Evaluator, error) {
	return nil, ErrSimulationEvaluatorNotImplemented
}

// Compile-time interface check: ensures *StaticUserSimulator satisfies
// eval.UserSimulator. If the interface changes (e.g., a method is added
// or a signature is modified), this produces a clear error at the type
// definition rather than at a distant call site.
var _ eval.UserSimulator = (*StaticUserSimulator)(nil)
