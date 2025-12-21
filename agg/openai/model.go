package openai

// Model holds OpenAI-specific configuration for making API requests.
type Model struct {
	model           string
	reasoningEffort string
}

// NewModel creates a new OpenAI Model with the given configuration.
func NewModel(model string, reasoningEffort string) *Model {
	return &Model{
		model:           model,
		reasoningEffort: reasoningEffort,
	}
}
