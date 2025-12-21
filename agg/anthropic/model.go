package anthropic

import "log"

// Model holds Anthropic-specific configuration for making API requests.
type Model struct {
	model        ModelID
	maxTok       int
	maxTokReason int
}

// NewModel creates a new Anthropic Model with the given configuration.
func NewModel(model ModelID, maxTok int, maxTokReason int) *Model {
	return &Model{
		model:        model,
		maxTok:       maxTok,
		maxTokReason: maxTokReason,
	}
}

type ModelID string

const (
	Haiku  ModelID = "claude-haiku-4-5-20251001"
	Sonnet ModelID = "claude-sonnet-4-5-20250929"
	Opus   ModelID = "claude-opus-4-5-20251101"
)

// no cache because I'm not leveraging it anyway for now
type modelCost struct {
	InputTokens  int64
	OutputTokens int64
}

var modelCosts = map[ModelID]modelCost{
	Haiku: {
		InputTokens:  1000, // $1.000 per 1M
		OutputTokens: 5000, // $5.000 per 1M
	},
	Sonnet: {
		InputTokens:  3000,  // $3.000 per 1M
		OutputTokens: 15000, // $15.000 per 1M
	},
	Opus: {
		InputTokens:  5000,  // $5.000 per 1M
		OutputTokens: 25000, // $25.000 per 1M
	},
}

func costFromUsage(model ModelID, usage usage) int64 {
	costs, ok := modelCosts[model]
	if !ok {
		log.Printf("cannot compute costs: unknown model: %s", model)
		return 0
	}

	return costs.InputTokens*usage.Input + costs.OutputTokens*usage.Output
}
