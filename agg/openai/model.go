package openai

import "log"

// Model holds OpenAI-specific configuration for making API requests.
type Model struct {
	model           ModelID
	reasoningEffort string
}

// NewModel creates a new OpenAI Model with the given configuration.
func NewModel(model ModelID, reasoningEffort string) *Model {
	return &Model{
		model:           model,
		reasoningEffort: reasoningEffort,
	}
}

type ModelID string

const (
	GPT41    ModelID = "gpt-4.1"
	GPT5Nano ModelID = "gpt-5-nano"
	GPT5Mini ModelID = "gpt-5-mini"
	GPT5Pro  ModelID = "gpt-5-pro"
	GPT51    ModelID = "gpt-5.1"
	GPT52    ModelID = "gpt-5.2"
	GPT52Pro ModelID = "gpt-5.2-pro"
)

type modelCost struct {
	InputTokens  int64
	CachedTokens int64
	OutputTokens int64
}

var modelCosts = map[ModelID]modelCost{
	GPT41: {
		InputTokens:  2000, // $2.000 per 1M
		CachedTokens: 500,  // $0.500 per 1M
		OutputTokens: 8000, // $8.000 per 1M
	},
	GPT5Nano: {
		InputTokens:  50,  // $0.050 per 1M
		CachedTokens: 5,   // $0.005 per 1M
		OutputTokens: 400, // $0.400 per 1M
	},
	GPT5Mini: {
		InputTokens:  250,  // $0.250 per 1M
		CachedTokens: 25,   // $0.025 per 1M
		OutputTokens: 2000, // $2.000 per 1M
	},
	GPT5Pro: {
		InputTokens:  15000,  // $15.000 per 1M
		CachedTokens: 15000,  // $15.000 per 1M
		OutputTokens: 120000, // $120.000 per 1M
	},
	GPT51: {
		InputTokens:  1250,  // $1.250 per 1M
		CachedTokens: 125,   // $0.125 per 1M
		OutputTokens: 10000, // $10.000 per 1M
	},
	GPT52: {
		InputTokens:  1750,  // $1.750 per 1M
		CachedTokens: 175,   // $0.175 per 1M
		OutputTokens: 14000, // $14.000 per 1M
	},
	GPT52Pro: {
		InputTokens:  21000,  // $21.000 per 1M
		CachedTokens: 21000,  // $21.000 per 1M
		OutputTokens: 168000, // $168.000 per 1M
	},
}

func costFromUsage(model ModelID, usage usage) int64 {
	costs, ok := modelCosts[model]
	if !ok {
		log.Printf("cannot compute costs: unknown model: %s", model)
		return 0
	}

	// OpenAI reports all Input Tokens as "Input" and a subset of it that was cached as "Cached",
	// so to avoid double counting we need to separate "RegularInput" from "Cached".
	regularInput := usage.Input - usage.InputDetails.Cached
	if regularInput < 0 {
		panic("assumption violated: more cached tokens than input tokens")
	}

	return (costs.InputTokens*regularInput +
		costs.CachedTokens*usage.InputDetails.Cached +
		costs.OutputTokens*usage.Output)
}
