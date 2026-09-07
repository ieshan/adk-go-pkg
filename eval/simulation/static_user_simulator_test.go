package simulation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"github.com/ieshan/adk-go-pkg/eval/simulation"
	"google.golang.org/genai"
)

func TestStaticUserSimulator_GetNextUserMessage(t *testing.T) {
	conversation := []eval.Invocation{
		{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hello"}}}},
		{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "how are you"}}}},
	}

	sim := simulation.NewStaticUserSimulator(conversation)

	msg1, err := sim.GetNextUserMessage(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextUserMessage failed: %v", err)
	}
	if msg1.Status != eval.UserSimulatorStatusSuccess {
		t.Errorf("Status = %v, want %v", msg1.Status, eval.UserSimulatorStatusSuccess)
	}
	if msg1.UserMessage == nil {
		t.Fatal("got nil UserMessage, want non-nil")
	}

	msg2, err := sim.GetNextUserMessage(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextUserMessage failed: %v", err)
	}
	if msg2.Status != eval.UserSimulatorStatusSuccess {
		t.Errorf("Status = %v, want %v", msg2.Status, eval.UserSimulatorStatusSuccess)
	}

	// Third call should return turn limit reached.
	msg3, err := sim.GetNextUserMessage(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextUserMessage failed: %v", err)
	}
	if msg3.Status != eval.UserSimulatorStatusTurnLimitReached {
		t.Errorf("Status = %v, want %v", msg3.Status, eval.UserSimulatorStatusTurnLimitReached)
	}
}

func TestStaticUserSimulator_NilUserContent(t *testing.T) {
	conversation := []eval.Invocation{
		{UserContent: nil},
	}

	sim := simulation.NewStaticUserSimulator(conversation)

	msg, err := sim.GetNextUserMessage(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextUserMessage failed: %v", err)
	}
	if msg.Status != eval.UserSimulatorStatusNoMessageGenerated {
		t.Errorf("Status = %v, want %v", msg.Status, eval.UserSimulatorStatusNoMessageGenerated)
	}
}

func TestStaticUserSimulator_EmptyConversation(t *testing.T) {
	sim := simulation.NewStaticUserSimulator(nil)

	msg, err := sim.GetNextUserMessage(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextUserMessage failed: %v", err)
	}
	if msg.Status != eval.UserSimulatorStatusTurnLimitReached {
		t.Errorf("Status = %v, want %v", msg.Status, eval.UserSimulatorStatusTurnLimitReached)
	}
}

func TestStaticUserSimulator_GetSimulationEvaluator(t *testing.T) {
	sim := simulation.NewStaticUserSimulator(nil)
	ev, err := sim.GetSimulationEvaluator()
	if !errors.Is(err, simulation.ErrSimulationEvaluatorNotImplemented) {
		t.Fatalf("got %v, want ErrSimulationEvaluatorNotImplemented", err)
	}
	if ev != nil {
		t.Error("got non-nil evaluator, want nil")
	}
}
