package simulation

import (
	"fmt"

	"github.com/ieshan/adk-go-pkg/eval"
	"google.golang.org/adk/v2/model"
)

// UserSimulatorProvider dispatches user simulator instances based on
// EvalCase data and config type. It implements eval.UserSimulatorProvider.
type UserSimulatorProvider struct {
	llm    model.LLM
	config LlmBackedUserSimulatorConfig
}

// NewUserSimulatorProvider creates a new UserSimulatorProvider.
func NewUserSimulatorProvider(llm model.LLM, config LlmBackedUserSimulatorConfig) *UserSimulatorProvider {
	return &UserSimulatorProvider{
		llm:    llm,
		config: config,
	}
}

// Provide returns an eval.UserSimulator for the given eval case.
func (p *UserSimulatorProvider) Provide(evalCase eval.EvalCase) (eval.UserSimulator, error) {
	if len(evalCase.Conversation) > 0 {
		if evalCase.ConversationScenario != nil {
			return nil, fmt.Errorf("both static conversation and conversation scenario provided; provide exactly one")
		}
		return NewStaticUserSimulator(evalCase.Conversation), nil
	}

	if evalCase.ConversationScenario == nil {
		return nil, fmt.Errorf("neither static conversation nor conversation scenario provided; provide exactly one")
	}

	// Dispatch by config type.
	switch p.config.Type {
	case "llm_backed":
		return NewLlmBackedUserSimulator(p.config, evalCase.ConversationScenario, p.llm), nil
	default:
		return nil, fmt.Errorf("no user simulator registered for config type %q", p.config.Type)
	}
}

// Compile-time interface check.
var _ eval.UserSimulatorProvider = (*UserSimulatorProvider)(nil)
