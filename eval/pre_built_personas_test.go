package eval_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestPreBuiltPersonas_AllPresent(t *testing.T) {
	expected := []string{"EXPERT", "NOVICE", "EVALUATOR"}
	for _, id := range expected {
		p, ok := eval.PreBuiltPersonas[id]
		if !ok {
			t.Errorf("pre-built persona %q not found", id)
			continue
		}
		if p.ID != id {
			t.Errorf("persona ID = %q, want %q", p.ID, id)
		}
		if p.Description == "" {
			t.Errorf("persona %q has empty description", id)
		}
		if len(p.Behaviors) == 0 {
			t.Errorf("persona %q has no behaviors", id)
		}
	}
}

func TestGetDefaultPersonaRegistry(t *testing.T) {
	registry := eval.GetDefaultPersonaRegistry()

	for _, id := range []string{"EXPERT", "NOVICE", "EVALUATOR"} {
		persona, err := registry.GetPersona(id)
		if err != nil {
			t.Errorf("GetPersona(%q) failed: %v", id, err)
			continue
		}
		if persona.ID != id {
			t.Errorf("persona ID = %q, want %q", persona.ID, id)
		}
	}

	personas := registry.GetRegisteredPersonas()
	if len(personas) != 3 {
		t.Errorf("got %d registered personas, want 3", len(personas))
	}
}

func TestGetDefaultPersonaRegistry_NotFound(t *testing.T) {
	registry := eval.GetDefaultPersonaRegistry()
	_, err := registry.GetPersona("NONEXISTENT")
	if err == nil {
		t.Errorf("got nil error, want non-nil error for nonexistent persona")
	}
}
