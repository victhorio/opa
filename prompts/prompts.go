package prompts

import (
	"embed"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/victhorio/opa/agg/core"
)

//go:embed opa.txt
var OpaSysPrompt string

//go:embed smart_read_note.txt
var SmartReadNotePrompt string

//go:embed tools/*.yaml
var toolSpecs embed.FS

// LoadToolSpec loads a tool specification from the embedded YAML files.
// The name should be the snake_case version of the tool name (e.g., "read_note", "smart_read_note").
// Returns an error if the spec file is missing or malformed.
func LoadToolSpec(name string) (core.Tool, error) {
	filename := fmt.Sprintf("tools/%s.yaml", name)
	data, err := toolSpecs.ReadFile(filename)
	if err != nil {
		return core.Tool{}, fmt.Errorf("failed to read tool spec %s: %w", filename, err)
	}

	var spec core.Tool
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return core.Tool{}, fmt.Errorf("failed to unmarshal tool spec %s: %w", filename, err)
	}

	return spec, nil
}
