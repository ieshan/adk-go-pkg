package eval

import (
	"context"
	"fmt"

	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// ConvertEventsToInvocation converts a slice of session events for a single
// agent run into an Invocation. It extracts the user content (from the first
// user-authored event or the provided userContent), the final response (from
// the last agent event), and the intermediate data (all events as
// InvocationEvents).
func ConvertEventsToInvocation(
	events []*session.Event,
	userContent *genai.Content,
	appDetails *AppDetails,
) Invocation {
	if userContent == nil {
		// Try to extract user content from the first user-authored event.
		for _, event := range events {
			if event != nil && event.Author == UserAuthor && event.Content != nil {
				userContent = event.Content
				break
			}
		}
	}

	var finalResponse *genai.Content
	var invocationEvents []InvocationEvent

	for _, event := range events {
		if event == nil {
			continue
		}

		invocationEvents = append(invocationEvents, InvocationEvent{
			Author:  event.Author,
			Content: event.Content,
		})

		// The final response is the last non-user, non-partial event with
		// content that is a final response.
		if event.Author != UserAuthor && event.Content != nil && event.IsFinalResponse() {
			finalResponse = event.Content
		}
	}

	var intermediateData IntermediateData
	if len(invocationEvents) > 0 {
		intermediateData = &InvocationEventsData{Events: invocationEvents}
	}

	invocation := Invocation{
		UserContent:      userContent,
		FinalResponse:    finalResponse,
		IntermediateData: intermediateData,
	}

	if appDetails != nil {
		invocation.AppDetails = appDetails
	}

	return invocation
}

// ConvertEventsToEvalInvocations groups events by InvocationID and converts
// each group into an Invocation. This is used for multi-turn conversations
// where multiple invocations are run in sequence.
func ConvertEventsToEvalInvocations(
	events []*session.Event,
	appDetails *AppDetails,
) []Invocation {
	groups := make(map[string][]*session.Event)
	var invocationIDs []string

	for _, event := range events {
		if event == nil {
			continue
		}
		id := event.InvocationID
		if id == "" {
			id = "default"
		}
		if _, exists := groups[id]; !exists {
			invocationIDs = append(invocationIDs, id)
		}
		groups[id] = append(groups[id], event)
	}

	var invocations []Invocation
	for _, id := range invocationIDs {
		invocation := ConvertEventsToInvocation(groups[id], nil, appDetails)
		invocation.InvocationID = id
		invocations = append(invocations, invocation)
	}

	return invocations
}

// GenerateStaticInvocations runs the agent for each user message in a static
// conversation and returns the resulting invocations. This is shared logic
// used by both AgentEvaluator and LocalEvalService.
func GenerateStaticInvocations(
	ctx context.Context,
	runner AgentRunner,
	appName string,
	conversation []Invocation,
) ([]Invocation, error) {
	var actualInvocations []Invocation
	for _, expected := range conversation {
		if expected.UserContent == nil {
			continue
		}

		events, err := runner.RunForSession(ctx, "", "", appName, expected.UserContent)
		if err != nil {
			return nil, fmt.Errorf("agent run failed: %w", err)
		}

		invocation := ConvertEventsToInvocation(events, expected.UserContent, nil)
		actualInvocations = append(actualInvocations, invocation)
	}
	return actualInvocations, nil
}

// GenerateDynamicInvocations runs the agent with a user simulator for dynamic
// conversation evaluation. It loops: gets next user message from the simulator,
// runs the agent, collects events, and checks stop/limit conditions.
func GenerateDynamicInvocations(
	ctx context.Context,
	runner AgentRunner,
	appName string,
	evalCase EvalCase,
	provider UserSimulatorProvider,
) ([]Invocation, error) {
	simulator, err := provider.Provide(evalCase)
	if err != nil {
		return nil, fmt.Errorf("failed to get user simulator: %w", err)
	}

	var allEvents []*session.Event
	var conversationEvents []*session.Event
	maxTurns := 20

	for turn := 0; turn < maxTurns; turn++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		nextMsg, err := simulator.GetNextUserMessage(ctx, conversationEvents)
		if err != nil {
			return nil, fmt.Errorf("user simulator failed on turn %d: %w", turn, err)
		}

		if nextMsg.Status != UserSimulatorStatusSuccess || nextMsg.UserMessage == nil {
			break
		}

		events, err := runner.RunForSession(ctx, "", "", appName, nextMsg.UserMessage)
		if err != nil {
			return nil, fmt.Errorf("agent run failed on turn %d: %w", turn, err)
		}

		allEvents = append(allEvents, events...)
		conversationEvents = append(conversationEvents, events...)
	}

	return ConvertEventsToEvalInvocations(allEvents, nil), nil
}
