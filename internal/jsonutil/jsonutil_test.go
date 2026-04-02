package jsonutil

import "testing"

func TestGenerateID(t *testing.T) {
	id1 := GenerateID(16)
	id2 := GenerateID(16)
	if len(id1) != 32 {
		t.Errorf("expected 32 chars, got %d", len(id1))
	}
	if id1 == id2 {
		t.Error("expected unique IDs")
	}
}

func TestMustMarshal(t *testing.T) {
	data := MustMarshal(map[string]string{"key": "value"})
	if string(data) != `{"key":"value"}` {
		t.Errorf("unexpected: %s", data)
	}
}

func TestMapToJSON_RoundTrip(t *testing.T) {
	m := map[string]any{"name": "test", "count": float64(42)}
	data, err := MapToJSON(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := JSONToMap(data)
	if err != nil {
		t.Fatal(err)
	}
	if got["name"] != "test" || got["count"] != float64(42) {
		t.Errorf("round-trip failed: %v", got)
	}
}
