package core

import (
	"context"
	"net/http"
)

// Model represents an AI model provider that can create response streams.
type Model interface {
	OpenStream(ctx context.Context, client *http.Client, msgs []Message, tools []Tool) (ResponseStream, error)
}

// ResponseStream represents a stream of events from an AI model response.
// Implementations must close both the underlying stream and the output channel when done.
type ResponseStream interface {
	Consume(ctx context.Context, out chan<- Event)
}
