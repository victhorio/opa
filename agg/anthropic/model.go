package anthropic

import (
	"log"
)

// Model holds Anthropic-specific configuration for making API requests.
type Model struct {
	model        ModelID
	maxTok       int
	maxTokReason int
	shouldCache  bool
}

// NewModel creates a new Anthropic Model with the given configuration.
func NewModel(model ModelID, maxTok int, maxTokReason int, shouldCache bool) *Model {
	return &Model{
		model:        model,
		maxTok:       maxTok,
		maxTokReason: maxTokReason,
		shouldCache:  shouldCache,
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
	In           int64
	InCacheWrite int64
	InCacheRead  int64
	Out          int64
}

var modelCosts = map[ModelID]modelCost{
	Haiku: {
		In:           1000, // $1.000 per 1M
		InCacheWrite: 1250, // $1.250 per 1M
		InCacheRead:  100,  // $0.100 per 1M
		Out:          5000, // $5.000 per 1M
	},
	Sonnet: {
		In:           3000,  // $3.000 per 1M
		InCacheWrite: 3750,  // $3.750 per 1M
		InCacheRead:  300,   // $0.300 per 1M
		Out:          15000, // $15.000 per 1M
	},
	Opus: {
		In:           5000,  // $5.000 per 1M
		InCacheWrite: 6250,  // $6.250 per 1M
		InCacheRead:  500,   // $0.500 per 1M
		Out:          25000, // $25.000 per 1M
	},
}

func costFromUsage(model ModelID, usage usage) int64 {
	costs, ok := modelCosts[model]
	if !ok {
		log.Printf("cannot compute costs: unknown model: %s", model)
		return 0
	}

	return (costs.In*usage.In +
		costs.InCacheWrite*usage.InCacheWrite +
		costs.InCacheRead*usage.InCacheRead +
		costs.Out*usage.Out)
}
