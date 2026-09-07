package jsonutil_test

import (
	"fmt"

	"github.com/ieshan/adk-go-pkg/internal/jsonutil"
)

// ExampleGenerateID demonstrates ID generation. GenerateID(16) returns a
// 32-character hex string (16 bytes encoded as hex).
func ExampleGenerateID() {
	id, err := jsonutil.GenerateID(16)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	// The ID is random, but its length is deterministic: 16 bytes -> 32 hex chars.
	fmt.Println(len(id))
	// Output: 32
}

// ExampleJSONToMap demonstrates converting JSON bytes into a map.
func ExampleJSONToMap() {
	data := []byte(`{"name":"Alice","age":30}`)
	m, err := jsonutil.JSONToMap(data)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Printf("name: %v\n", m["name"])
	fmt.Printf("age: %v\n", m["age"])
	// Output:
	// name: Alice
	// age: 30
}
