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

func (s *StaticUserSimulator) GetSimulationEvaluator() (eval.Evaluator, error) {
	return nil, ErrSimulationEvaluatorNotImplemented
}

// Compile-time interface check.
var _ eval.UserSimulator = (*StaticUserSimulator)(nil)
