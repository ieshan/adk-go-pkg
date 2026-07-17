package eval

import (
	"fmt"
	"strings"
	"sync"
)

// UserBehavior describes how a simulated user should act.
type UserBehavior struct {
	// Name is the name of the behavior.
	Name string `json:"name"`

	// Description describes the behavior.
	Description string `json:"description"`

	// BehaviorInstructions are instructions for the user simulator.
	BehaviorInstructions []string `json:"behaviorInstructions,omitempty"`

	// ViolationRubrics are rubrics used to evaluate violations of this behavior.
	ViolationRubrics []string `json:"violationRubrics,omitempty"`
}

// GetBehaviorInstructionsStr returns the behavior instructions as a single
// string joined by newlines.
func (b *UserBehavior) GetBehaviorInstructionsStr() string {
	return strings.Join(b.BehaviorInstructions, "\n")
}

// GetViolationRubricsStr returns the violation rubrics as a single string
// joined by newlines.
func (b *UserBehavior) GetViolationRubricsStr() string {
	return strings.Join(b.ViolationRubrics, "\n")
}

// UserPersona aggregates multiple behaviors and describes a simulated user
// persona.
type UserPersona struct {
	// ID is the unique identifier for the persona.
	ID string `json:"id"`

	// Description describes the persona.
	Description string `json:"description"`

	// Behaviors is the list of behaviors associated with this persona.
	Behaviors []UserBehavior `json:"behaviors,omitempty"`
}

// UserPersonaRegistry manages and retrieves user personas.
type UserPersonaRegistry struct {
	mu       sync.RWMutex
	personas map[string]UserPersona
}

// NewUserPersonaRegistry creates a new empty UserPersonaRegistry.
func NewUserPersonaRegistry() *UserPersonaRegistry {
	return &UserPersonaRegistry{
		personas: make(map[string]UserPersona),
	}
}

// RegisterPersona registers a persona with the given ID.
func (r *UserPersonaRegistry) RegisterPersona(id string, persona UserPersona) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.personas[id] = persona
}

// GetPersona returns the persona with the given ID, or an error if not found.
func (r *UserPersonaRegistry) GetPersona(id string) (UserPersona, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	persona, ok := r.personas[id]
	if !ok {
		return UserPersona{}, fmt.Errorf("persona %q not found in registry", id)
	}
	return persona, nil
}

// GetRegisteredPersonas returns all registered personas.
func (r *UserPersonaRegistry) GetRegisteredPersonas() []UserPersona {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]UserPersona, 0, len(r.personas))
	for _, persona := range r.personas {
		result = append(result, persona)
	}
	return result
}
