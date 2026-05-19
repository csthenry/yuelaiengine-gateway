package api

import (
	"encoding/json"
	"testing"
)

func TestOpenAPISpecIsValidJSON(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(openAPISpec), &doc); err != nil {
		t.Fatalf("openAPISpec must be valid JSON: %v", err)
	}

	if _, ok := doc["openapi"]; !ok {
		t.Fatalf("openAPISpec missing required top-level field: openapi")
	}

	if _, ok := doc["paths"]; !ok {
		t.Fatalf("openAPISpec missing required top-level field: paths")
	}
}
