package core

import "context"

// EmbeddingsResult holds the output from an embedding operation.
type EmbeddingsResult struct {
	// TODO(optimize): let's eventually move to a base64 representation instead of an explicit
	//                 array of floats.
	Vectors [][]float64
	// Cost unit is thousandths of a millionth of a dollar.
	Cost int64
}

// Embedder is implemented by providers that can generate embeddings.
type Embedder interface {
	Embed(ctx context.Context, inputs []string, dimensions *int) (*EmbeddingsResult, error)
	Provider() Provider
}
