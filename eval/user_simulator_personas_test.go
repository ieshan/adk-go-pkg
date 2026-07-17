package eval

import (
	"testing"
)

func TestUserPersonaRegistry_RegisterAndGet(t *testing.T) {
	registry := NewUserPersonaRegistry()
	persona := UserPersona{
		ID:          "test_persona",
		Description: "A test persona",
		Behaviors: []UserBehavior{
			{Name: "behavior1", Description: "test behavior"},
		},
	}
	registry.RegisterPersona("test_persona", persona)

	got, err := registry.GetPersona("test_persona")
	if err != nil {
		t.Fatalf("GetPersona failed: %v", err)
	}
	if got.ID != "test_persona" {
		t.Errorf("got %s, want test_persona", got.ID)
	}
}

func TestUserPersonaRegistry_GetNotFound(t *testing.T) {
	registry := NewUserPersonaRegistry()
	_, err := registry.GetPersona("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent persona")
	}
}

func TestUserPersonaRegistry_GetRegisteredPersonas(t *testing.T) {
	registry := NewUserPersonaRegistry()
	registry.RegisterPersona("p1", UserPersona{ID: "p1"})
	registry.RegisterPersona("p2", UserPersona{ID: "p2"})

	personas := registry.GetRegisteredPersonas()
	if len(personas) != 2 {
		t.Errorf("expected 2 personas, got %d", len(personas))
	}
}

func TestUserBehavior_GetBehaviorInstructionsStr(t *testing.T) {
	b := UserBehavior{
		BehaviorInstructions: []string{"line1", "line2"},
	}
	s := b.GetBehaviorInstructionsStr()
	if s != "line1\nline2" {
		t.Errorf("got %q, want %q", s, "line1\nline2")
	}
}

func TestUserBehavior_GetViolationRubricsStr(t *testing.T) {
	b := UserBehavior{
		ViolationRubrics: []string{"rubric1", "rubric2"},
	}
	s := b.GetViolationRubricsStr()
	if s != "rubric1\nrubric2" {
		t.Errorf("got %q, want %q", s, "rubric1\nrubric2")
	}
}
