package prompts

import (
	"testing"
)

func TestLoadToolSpec(t *testing.T) {
	tests := []struct {
		name         string
		specName     string
		expectedName string
		numParams    int
	}{
		{"ReadNote", "read_note", "ReadNote", 1},
		{"SmartReadNote", "smart_read_note", "SmartReadNote", 2},
		{"ListDir", "list_dir", "ListDir", 1},
		{"RipGrep", "rip_grep", "RipGrep", 3},
		{"SemanticSearch", "semantic_search", "SemanticSearch", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := LoadToolSpec(tt.specName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if spec.Name != tt.expectedName {
				t.Errorf("expected name %s, got %s", tt.expectedName, spec.Name)
			}

			if spec.Desc == "" {
				t.Error("description should not be empty")
			}

			if len(spec.Params) != tt.numParams {
				t.Errorf("expected %d params, got %d", tt.numParams, len(spec.Params))
			}

			// Verify all params have descriptions
			for paramName, param := range spec.Params {
				if param.Desc == "" {
					t.Errorf("param %s has empty description", paramName)
				}
				if param.Type == "" {
					t.Errorf("param %s has empty type", paramName)
				}
			}
		})
	}
}

func TestLoadToolSpecError(t *testing.T) {
	_, err := LoadToolSpec("non_existent_tool")
	if err == nil {
		t.Error("expected error for non-existent spec")
	}
}
