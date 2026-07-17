package simulation

import (
	"context"
	"fmt"
	"strings"

	"github.com/ieshan/adk-go-pkg/eval"
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

// ValidateCustomInstructions checks for required placeholders.
func (c LlmBackedUserSimulatorConfig) ValidateCustomInstructions() error {
	required := []string{"{{ stop_signal }}", "{{ conversation_plan }}", "{{ conversation_history }}"}
	for _, placeholder := range required {
		if !strings.Contains(c.CustomInstructions, placeholder) {
			return fmt.Errorf("custom instructions must contain %q", placeholder)
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

	var prompt string
	if s.config.CustomInstructions != "" {
		prompt = s.config.CustomInstructions
		prompt = strings.ReplaceAll(prompt, "{{ stop_signal }}", "</finished>")
		prompt = strings.ReplaceAll(prompt, "{{ conversation_plan }}", plan)
		prompt = strings.ReplaceAll(prompt, "{{ conversation_history }}", history)
	} else if s.userPersona != nil {
		prompt = GetLlmBackedUserSimulatorPromptWithPersona(plan, history, "</finished>", s.userPersona)
	} else {
		prompt = GetLlmBackedUserSimulatorPrompt(plan, history, "</finished>")
	}

	// Call LLM.
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Parts: []*genai.Part{{Text: prompt}}, Role: "user"},
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
