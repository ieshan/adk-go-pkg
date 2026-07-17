package eval

import (
	"testing"
)

func TestValidatePathSegment(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid name", "my_eval_set", false},
		{"valid with numbers", "eval_set_123", false},
		{"valid with hyphens", "my-eval-set", false},
		{"empty string", "", true},
		{"null byte", "eval\x00set", true},
		{"forward slash", "eval/set", true},
		{"backslash", "eval\\set", true},
		{"dot dot", "..", true},
		{"dot dot slash", "../", true},
		{"single dot", ".", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathSegment(tt.input, "testField")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePathSegment(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
