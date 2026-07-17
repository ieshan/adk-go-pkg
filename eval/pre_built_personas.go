package eval

// Pre-built user behaviors for composing personas.
var (
	// AdvanceDetailOriented behavior: the user provides detailed, specific
	// instructions and expects the agent to follow them precisely.
	AdvanceDetailOriented = UserBehavior{
		Name:        "ADVANCE_DETAIL_ORIENTED",
		Description: "The user provides detailed and specific instructions to the agent.",
		BehaviorInstructions: []string{
			"Provide detailed and specific instructions to the agent.",
			"Include relevant context and constraints in your messages.",
		},
		ViolationRubrics: []string{
			"The user message is too vague or lacks necessary detail.",
		},
	}

	// AdvanceGoalOriented behavior: the user focuses on the goal rather than
	// specific steps, giving the agent freedom in how to accomplish tasks.
	AdvanceGoalOriented = UserBehavior{
		Name:        "ADVANCE_GOAL_ORIENTED",
		Description: "The user focuses on the goal rather than specific steps.",
		BehaviorInstructions: []string{
			"Focus on the goal rather than specific steps.",
			"Give the agent freedom in how to accomplish tasks.",
		},
		ViolationRubrics: []string{
			"The user message prescribes specific steps rather than focusing on the goal.",
		},
	}

	// AnswerRelevantOnly behavior: the user only answers with information
	// relevant to the agent's questions.
	AnswerRelevantOnly = UserBehavior{
		Name:        "ANSWER_RELEVANT_ONLY",
		Description: "The user only provides information relevant to the agent's questions.",
		BehaviorInstructions: []string{
			"Only answer with information relevant to the agent's questions.",
			"Do not provide unsolicited information.",
		},
		ViolationRubrics: []string{
			"The user provides irrelevant or unsolicited information.",
		},
	}

	// AnswerAll behavior: the user answers all questions, including
	// tangential ones, and provides extra context.
	AnswerAll = UserBehavior{
		Name:        "ANSWER_ALL",
		Description: "The user answers all questions and provides extra context.",
		BehaviorInstructions: []string{
			"Answer all questions from the agent, including tangential ones.",
			"Provide extra context when possible.",
		},
		ViolationRubrics: []string{
			"The user ignores or skips a question from the agent.",
		},
	}

	// DoNotCorrectAgent behavior: the user does not correct the agent when
	// it makes mistakes.
	DoNotCorrectAgent = UserBehavior{
		Name:        "DO_NOT_CORRECT_AGENT",
		Description: "The user does not correct the agent when it makes mistakes.",
		BehaviorInstructions: []string{
			"Do not correct the agent when it makes mistakes.",
			"Accept incorrect responses without challenging them.",
		},
		ViolationRubrics: []string{
			"The user corrects the agent or points out mistakes.",
		},
	}

	// EndNoTroubleshooting behavior: the user ends the conversation without
	// attempting to troubleshoot issues.
	EndNoTroubleshooting = UserBehavior{
		Name:        "END_NO_TROUBLESHOOTING",
		Description: "The user ends the conversation without troubleshooting.",
		BehaviorInstructions: []string{
			"End the conversation without attempting to troubleshoot issues.",
			"Use the stop signal when the agent's response is complete, even if imperfect.",
		},
		ViolationRubrics: []string{
			"The user attempts to troubleshoot or fix issues before ending.",
		},
	}

	// ToneProfessional behavior: the user communicates in a professional tone.
	ToneProfessional = UserBehavior{
		Name:        "TONE_PROFESSIONAL",
		Description: "The user communicates in a professional tone.",
		BehaviorInstructions: []string{
			"Communicate in a professional and formal tone.",
			"Avoid casual or colloquial language.",
		},
		ViolationRubrics: []string{
			"The user uses casual or colloquial language.",
		},
	}

	// ToneConversational behavior: the user communicates in a casual,
	// conversational tone.
	ToneConversational = UserBehavior{
		Name:        "TONE_CONVERSATIONAL",
		Description: "The user communicates in a casual, conversational tone.",
		BehaviorInstructions: []string{
			"Communicate in a casual and conversational tone.",
			"Use everyday language and colloquialisms.",
		},
		ViolationRubrics: []string{
			"The user uses overly formal or rigid language.",
		},
	}
)

// PreBuiltPersonas defines the pre-built user personas available in the eval
// framework. These mirror the Python ADK _PreBuiltPersonas enum.
var PreBuiltPersonas = map[string]UserPersona{
	"EXPERT": {
		ID:          "EXPERT",
		Description: "A user who knows exactly what they want and expects the agent to execute commands efficiently, with little patience for unnecessary interactions.",
		Behaviors: []UserBehavior{
			AdvanceDetailOriented,
			AnswerRelevantOnly,
			ToneProfessional,
		},
	},
	"NOVICE": {
		ID:          "NOVICE",
		Description: "A user who is trying to solve a problem they don't fully understand and relies on the agent for guidance. Patient but unable to troubleshoot or correct the agent's mistakes.",
		Behaviors: []UserBehavior{
			AdvanceGoalOriented,
			DoNotCorrectAgent,
			AnswerAll,
			ToneConversational,
		},
	},
	"EVALUATOR": {
		ID:          "EVALUATOR",
		Description: "A user who aims to assess whether the agent can accomplish the goals outlined in the conversation plan.",
		Behaviors: []UserBehavior{
			AdvanceDetailOriented,
			AnswerRelevantOnly,
			EndNoTroubleshooting,
			DoNotCorrectAgent,
			ToneConversational,
		},
	},
}

// GetDefaultPersonaRegistry returns a UserPersonaRegistry pre-populated with
// the pre-built personas (EXPERT, NOVICE, EVALUATOR).
func GetDefaultPersonaRegistry() *UserPersonaRegistry {
	registry := NewUserPersonaRegistry()
	for _, persona := range PreBuiltPersonas {
		registry.RegisterPersona(persona.ID, persona)
	}
	return registry
}
