package simulation

import (
	"context"
	"fmt"
	"strings"

	"github.com/ieshan/adk-go-pkg/eval"
	"github.com/ieshan/adk-go-pkg/prompt"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// LlmBackedUserSimulatorConfig configures an LLM-backed user simulator.
type LlmBackedUserSimulatorConfig struct {
	Type                  string                       `json:"type"` // discriminator: "llm_backed"
	Model                 string                       `json:"model,omitempty"`
	ModelConfiguration    *genai.GenerateContentConfig `json:"modelConfiguration,omitempty"`
	MaxAllowedInvocations int                          `json:"maxAllowedInvocations,omitempty"`
	CustomInstructions    string                       `json:"customInstructions,omitempty"`
	IncludeFunctionCalls  bool                         `json:"includeFunctionCalls,omitempty"`
}

// DefaultLlmBackedUserSimulatorConfig returns default config values.
func DefaultLlmBackedUserSimulatorConfig() LlmBackedUserSimulatorConfig {
	return LlmBackedUserSimulatorConfig{
		Type:                  "llm_backed",
		Model:                 "gemini-2.5-flash",
		MaxAllowedInvocations: 20,
	}
}

// ValidateCustomInstructions checks for required {{.Input.*}} placeholders.
func (c LlmBackedUserSimulatorConfig) ValidateCustomInstructions() error {
	required := []string{"stop_signal", "conversation_plan", "conversation_history"}
	for _, param := range required {
		if !hasTemplatePlaceholder(c.CustomInstructions, param) {
			return fmt.Errorf("custom instructions must contain {{.Input.%s}}", param)
		}
	}
	return nil
}

// LlmBackedUserSimulator uses an LLM to generate user messages.
type LlmBackedUserSimulator struct {
	config               LlmBackedUserSimulatorConfig
	conversationScenario *eval.ConversationScenario
	llm                  model.LLM
	invocationCount      int
	userPersona          *eval.UserPersona
}

// NewLlmBackedUserSimulator creates a new LlmBackedUserSimulator.
func NewLlmBackedUserSimulator(
	config LlmBackedUserSimulatorConfig,
	conversationScenario *eval.ConversationScenario,
	llm model.LLM,
) *LlmBackedUserSimulator {
	if config.Model == "" {
		config.Model = "gemini-2.5-flash"
	}
	if config.MaxAllowedInvocations <= 0 {
		config.MaxAllowedInvocations = 20
	}

	var persona *eval.UserPersona
	if conversationScenario != nil && conversationScenario.UserPersona != nil {
		persona = conversationScenario.UserPersona
	}

	return &LlmBackedUserSimulator{
		config:               config,
		conversationScenario: conversationScenario,
		llm:                  llm,
		userPersona:          persona,
	}
}

// GetNextUserMessage generates the next user message using the LLM.
func (s *LlmBackedUserSimulator) GetNextUserMessage(ctx context.Context, events []*session.Event) (*eval.NextUserMessage, error) {
	// First invocation: return starting prompt.
	if s.invocationCount == 0 && s.conversationScenario != nil && s.conversationScenario.StartingPrompt != "" {
		s.invocationCount++
		return &eval.NextUserMessage{
			Status: eval.UserSimulatorStatusSuccess,
			UserMessage: &genai.Content{
				Parts: []*genai.Part{{Text: s.conversationScenario.StartingPrompt}},
				Role:  "user",
			},
		}, nil
	}

	// Check invocation limit.
	if s.invocationCount >= s.config.MaxAllowedInvocations {
		return &eval.NextUserMessage{Status: eval.UserSimulatorStatusTurnLimitReached}, nil
	}

	// Build prompt from conversation plan and history.
	plan := ""
	if s.conversationScenario != nil {
		plan = s.conversationScenario.ConversationPlan
	}

	history := s.summarizeConversation(events)

	var userPrompt string
	if s.config.CustomInstructions != "" {
		tmpl, err := prompt.New().Parse("custom-user-simulator", s.config.CustomInstructions)
		if err != nil {
			return nil, fmt.Errorf("invalid custom instructions template: %w", err)
		}
		rendered, err := tmpl.Execute(prompt.BuildData(map[string]any{
			"stop_signal":          "</finished>",
			"conversation_plan":    plan,
			"conversation_history": history,
		}))
		if err != nil {
			return nil, fmt.Errorf("failed to render custom instructions template: %w", err)
		}
		userPrompt = rendered
	} else if s.userPersona != nil {
		var err error
		userPrompt, err = GetLlmBackedUserSimulatorPromptWithPersona(plan, history, "</finished>", s.userPersona)
		if err != nil {
			return nil, fmt.Errorf("failed to render persona user simulator prompt: %w", err)
		}
	} else {
		var err error
		userPrompt, err = GetLlmBackedUserSimulatorPrompt(plan, history, "</finished>")
		if err != nil {
			return nil, fmt.Errorf("failed to render default user simulator prompt: %w", err)
		}
	}

	// Call LLM.
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Parts: []*genai.Part{{Text: userPrompt}}, Role: "user"},
		},
	}

	var lastResp *model.LLMResponse
	for resp, err := range s.llm.GenerateContent(ctx, req, false) {
		if err != nil {
			return nil, fmt.Errorf("user simulator LLM call failed: %w", err)
		}
		lastResp = resp
	}

	if lastResp == nil || lastResp.Content == nil {
		return &eval.NextUserMessage{Status: eval.UserSimulatorStatusNoMessageGenerated}, nil
	}

	text := ""
	for _, part := range lastResp.Content.Parts {
		text += part.Text
	}

	if text == "" {
		return &eval.NextUserMessage{Status: eval.UserSimulatorStatusNoMessageGenerated}, nil
	}

	// Check for stop signal.
	if strings.Contains(text, "</finished>") {
		return &eval.NextUserMessage{Status: eval.UserSimulatorStatusStopSignalDetected}, nil
	}

	s.invocationCount++
	return &eval.NextUserMessage{
		Status: eval.UserSimulatorStatusSuccess,
		UserMessage: &genai.Content{
			Parts: []*genai.Part{{Text: text}},
			Role:  "user",
		},
	}, nil
}

// GetSimulationEvaluator returns an evaluator for the simulator's output.
func (s *LlmBackedUserSimulator) GetSimulationEvaluator() (eval.Evaluator, error) {
	return nil, ErrSimulationEvaluatorNotImplemented
}

// summarizeConversation converts events to "author: text" lines.
func (s *LlmBackedUserSimulator) summarizeConversation(events []*session.Event) string {
	var lines []string
	for _, event := range events {
		if event == nil || event.Content == nil {
			continue
		}
		author := event.Author
		if author == "" {
			author = "user"
		}
		var text strings.Builder
		for _, part := range event.Content.Parts {
			if part.Text != "" {
				text.WriteString(part.Text)
			}
			if s.config.IncludeFunctionCalls && part.FunctionCall != nil {
				text.WriteString(fmt.Sprintf(" [Function call: %s(%v)]", part.FunctionCall.Name, part.FunctionCall.Args))
			}
			if s.config.IncludeFunctionCalls && part.FunctionResponse != nil {
				text.WriteString(fmt.Sprintf(" [Function response: %s]", part.FunctionResponse.Name))
			}
		}
		if text.Len() > 0 {
			lines = append(lines, fmt.Sprintf("%s: %s", author, text.String()))
		}
	}
	return strings.Join(lines, "\n")
}

// Compile-time interface check.
var _ eval.UserSimulator = (*LlmBackedUserSimulator)(nil)
