package aguiadk

import (
	"encoding/json"
	"fmt"
	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// RemoteAgentConfig configures an ADK agent that delegates to a remote AG-UI
// endpoint.
type RemoteAgentConfig struct {
	// Name is the ADK agent name (required).
	Name string
	// Description is the ADK agent description.
	Description string
	// Endpoint is the remote AG-UI SSE URL (required).
	Endpoint string
	// Client is an optional pre-configured AG-UI client. When nil, a
	// ClientAgent is created from Endpoint.
	Client agui.Agent
	// BeforeAgentCallbacks run before the remote call.
	BeforeAgentCallbacks []agent.BeforeAgentCallback
	// AfterAgentCallbacks run after the remote call completes.
	AfterAgentCallbacks []agent.AfterAgentCallback
	// SubAgents are child ADK agents.
	SubAgents []agent.Agent
}

// NewRemoteAgent creates an ADK agent that streams events from a remote AG-UI
// endpoint. It maps ADK invocation context (user content + session history)
// into AG-UI RunAgentInput.Messages, streams AG-UI events from the remote
// endpoint, and maps TEXT_MESSAGE_CONTENT, TEXT_MESSAGE_END, TOOL_CALL_START,
// TOOL_CALL_ARGS, STATE_SNAPSHOT, and RUN_ERROR events back into ADK
// *session.Event instances.
func NewRemoteAgent(cfg RemoteAgentConfig) (agent.Agent, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("aguiadk: RemoteAgentConfig.Name is required")
	}
	if cfg.Endpoint == "" && cfg.Client == nil {
		return nil, fmt.Errorf("aguiadk: RemoteAgentConfig.Endpoint or Client is required")
	}

	var client agui.Agent
	if cfg.Client != nil {
		client = cfg.Client
	} else {
		client = agui.NewClientAgent(agui.ClientConfig{Endpoint: cfg.Endpoint})
	}

	return agent.New(agent.Config{
		Name:                 cfg.Name,
		Description:          cfg.Description,
		SubAgents:            cfg.SubAgents,
		BeforeAgentCallbacks: cfg.BeforeAgentCallbacks,
		AfterAgentCallbacks:  cfg.AfterAgentCallbacks,
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return runRemoteAgent(ctx, client, cfg.Name)
		},
	})
}

// runRemoteAgent builds the AG-UI input from the ADK invocation context,
// streams events from the remote endpoint, and maps them to ADK session
// events.
func runRemoteAgent(ctx agent.InvocationContext, client agui.Agent, agentName string) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		input := buildRunAgentInput(ctx)

		for ev, err := range client.Run(ctx, input) {
			if err != nil {
				yield(nil, fmt.Errorf("remote agent %q: %w", agentName, err))
				return
			}
			if ev == nil {
				continue
			}
			adkEv, ok := mapAGUIEventToADKEvent(ctx, ev, agentName)
			if !ok {
				continue
			}
			if !yield(adkEv, nil) {
				return
			}
		}
	}
}

// buildRunAgentInput constructs the AG-UI RunAgentInput from the ADK
// invocation context's user content and session event history.
func buildRunAgentInput(ctx agent.InvocationContext) types.RunAgentInput {
	input := types.RunAgentInput{
		ThreadID: ctx.Session().ID(),
		RunID:    ctx.InvocationID(),
	}

	// Map session history to AG-UI messages using the same high-fidelity
	// converter as the MESSAGES_SNAPSHOT path (preserves tool calls, tool
	// responses, and author names).
	if sess := ctx.Session(); sess != nil {
		input.Messages = sessionEventsToMessages(sess.Events())
	}

	// Append the current user content as the final user message.
	if uc := ctx.UserContent(); uc != nil {
		var text string
		for _, part := range uc.Parts {
			if part != nil && part.Text != "" {
				text += part.Text
			}
		}
		if text != "" {
			input.Messages = append(input.Messages, types.Message{
				ID:      "user-" + ctx.InvocationID(),
				Role:    types.RoleUser,
				Content: text,
			})
		}
	}

	return input
}

// mapAGUIEventToADKEvent converts a single AG-UI event into an ADK session
// event. Returns (nil, false) for events that have no ADK counterpart (e.g.
// STEP_*, ACTIVITY_*).
func mapAGUIEventToADKEvent(ctx agent.InvocationContext, ev events.Event, agentName string) (*session.Event, bool) {
	switch ev.Type() {
	case events.EventTypeTextMessageContent:
		contentEv, ok := ev.(*events.TextMessageContentEvent)
		if !ok {
			return nil, false
		}
		adkEv := session.NewEvent(ctx, ctx.InvocationID())
		adkEv.Author = agentName
		adkEv.LLMResponse = model.LLMResponse{
			Content: &genai.Content{
				Role:  genai.RoleModel,
				Parts: []*genai.Part{{Text: contentEv.Delta}},
			},
			Partial: true,
		}
		return adkEv, true

	case events.EventTypeTextMessageEnd:
		adkEv := session.NewEvent(ctx, ctx.InvocationID())
		adkEv.Author = agentName
		adkEv.LLMResponse = model.LLMResponse{
			TurnComplete: true,
		}
		return adkEv, true

	case events.EventTypeToolCallStart:
		toolEv, ok := ev.(*events.ToolCallStartEvent)
		if !ok {
			return nil, false
		}
		adkEv := session.NewEvent(ctx, ctx.InvocationID())
		adkEv.Author = agentName
		adkEv.LLMResponse = model.LLMResponse{
			Content: &genai.Content{
				Role: genai.RoleModel,
				Parts: []*genai.Part{{
					FunctionCall: &genai.FunctionCall{
						ID:   toolEv.ToolCallID,
						Name: toolEv.ToolCallName,
					},
				}},
			},
		}
		return adkEv, true

	case events.EventTypeToolCallArgs:
		toolEv, ok := ev.(*events.ToolCallArgsEvent)
		if !ok {
			return nil, false
		}
		var args map[string]any
		if toolEv.Delta != "" {
			_ = json.Unmarshal([]byte(toolEv.Delta), &args)
		}
		adkEv := session.NewEvent(ctx, ctx.InvocationID())
		adkEv.Author = agentName
		adkEv.LLMResponse = model.LLMResponse{
			Content: &genai.Content{
				Role: genai.RoleModel,
				Parts: []*genai.Part{{
					FunctionCall: &genai.FunctionCall{
						ID:   toolEv.ToolCallID,
						Args: args,
					},
				}},
			},
		}
		return adkEv, true

	case events.EventTypeStateSnapshot:
		stateEv, ok := ev.(*events.StateSnapshotEvent)
		if !ok {
			return nil, false
		}
		adkEv := session.NewEvent(ctx, ctx.InvocationID())
		adkEv.Author = agentName
		if stateMap, ok := stateEv.Snapshot.(map[string]any); ok {
			adkEv.Actions.StateDelta = stateMap
		}
		return adkEv, true

	case events.EventTypeRunError:
		errEv, ok := ev.(*events.RunErrorEvent)
		if !ok {
			return nil, false
		}
		adkEv := session.NewEvent(ctx, ctx.InvocationID())
		adkEv.Author = agentName
		adkEv.LLMResponse = model.LLMResponse{
			ErrorMessage: errEv.Message,
		}
		return adkEv, true

	default:
		return nil, false
	}
}
