package core

import (
	"context"
	"net/http"
)

// StreamCfg configures behavior for opening a model stream.
type StreamCfg struct {
	// DisableTools prevents the model from using tool calls in this stream.
	// When true, the model will be forced to generate a direct response.
	DisableTools bool

	// DetailedReasoning configures the model to provide a detailed summary of the reasoning
	// process instead of the default "concise" one.
	DetailedReasoning bool
}

// Model represents an AI model provider that can create response streams.
type Model interface {
	OpenStream(ctx context.Context, client *http.Client, msgs []*Msg, tools []Tool, cfg StreamCfg) (ResponseStream, error)
	Provider() Provider
}

// ResponseStream represents a stream of events from an AI model response.
// Implementations must close both the underlying stream and the output channel when done.
type ResponseStream interface {
	Consume(ctx context.Context, out chan<- Event)
}

type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
)
