package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/victhorio/opa/agg/core"
)

type EmbeddingModelID string

const (
	OpenAISmall EmbeddingModelID = "text-embedding-3-small"
	OpenAILarge EmbeddingModelID = "text-embedding-3-large"
)

type OpenAIEmbeddings struct {
	modelID EmbeddingModelID
	apiKey  string
	client  *http.Client
}

// NewOpenAIEmbedder creates a new OpenAI embeddings client.
// The OPENAI_API_KEY environment variable will be used to fetch the api key.
// If client is nil, a default http.Client will be created.
// Returns an error if no API key is available.
func NewOpenAIEmbedder(modelID EmbeddingModelID, client *http.Client) (*OpenAIEmbeddings, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}

	if client == nil {
		client = &http.Client{}
	}

	return &OpenAIEmbeddings{
		modelID: modelID,
		apiKey:  apiKey,
		client:  client,
	}, nil
}

// Provider returns the provider identifier.
func (e *OpenAIEmbeddings) Provider() core.Provider {
	return core.ProviderOpenAI
}

// Embed generates embeddings for the provided inputs.
// The dimensions parameter is optional; pass nil to use the model's default dimensions.
// Returns vectors in the same order as the inputs.
func (e *OpenAIEmbeddings) Embed(ctx context.Context, inputs []string, dimensions *int) (*core.EmbeddingsResult, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("no inputs provided")
	}

	// Prepare request payload
	payload := embeddingRequest{
		Input:          inputs,
		Model:          e.modelID,
		EncodingFormat: "float",
		Dimensions:     dimensions,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", embeddingsEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.apiKey))
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if err != nil {
			return nil, fmt.Errorf("openai embeddings api error: status=%s (failed to read body: %w)", resp.Status, err)
		}
		return nil, fmt.Errorf("openai embeddings api error: status=%s, body=%s", resp.Status, string(body))
	}

	// Parse response
	var embResp embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract vectors in order
	vectors := make([][]float64, len(inputs))
	for _, item := range embResp.Data {
		if item.Index < 0 || item.Index >= len(inputs) {
			return nil, fmt.Errorf("invalid index %d in response (expected 0-%d)", item.Index, len(inputs)-1)
		}
		vectors[item.Index] = item.Embedding
	}

	// Verify all vectors were populated
	for i, v := range vectors {
		if v == nil {
			return nil, fmt.Errorf("missing embedding for input at index %d", i)
		}
	}

	// Calculate cost
	cost := e.calculateCost(embResp.Usage.PromptTokens)

	return &core.EmbeddingsResult{
		Vectors: vectors,
		Cost:    cost,
	}, nil
}

// embeddingRequest is the request payload for the OpenAI embeddings API.
type embeddingRequest struct {
	Input          []string         `json:"input"`
	Model          EmbeddingModelID `json:"model"`
	EncodingFormat string           `json:"encoding_format,omitempty"`
	Dimensions     *int             `json:"dimensions,omitempty"`
}

// embeddingResponse is the response from the OpenAI embeddings API.
type embeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int64 `json:"prompt_tokens"`
		TotalTokens  int64 `json:"total_tokens"`
	} `json:"usage"`
}

// calculateCost computes the dollar cost from token usage.
func (e *OpenAIEmbeddings) calculateCost(tokens int64) int64 {
	costPerToken, ok := embeddingModelCosts[e.modelID]
	if !ok {
		// Unknown model, return 0 cost
		return 0
	}

	return tokens * costPerToken
}

// embeddingModelCosts stores the cost per token for each model.
// Values are in the same units as used in agg/openai and agg/anthropic packages:
// billionths of a dollar per token.
//
// To derive these values from published pricing (in dollars per 1M tokens):
//   cost_per_token = (price_per_1M_tokens / 1_000_000) * 1_000_000_000
//   cost_per_token = price_per_1M_tokens * 1000
//
// Examples:
//   $0.020 per 1M tokens → 0.020 * 1000 = 20 billionths/token
//   $0.130 per 1M tokens → 0.130 * 1000 = 130 billionths/token
var embeddingModelCosts = map[EmbeddingModelID]int64{
	OpenAISmall: 20,  // $0.020 per 1M tokens
	OpenAILarge: 130, // $0.130 per 1M tokens
}

// embeddingsEndpoint is the OpenAI embeddings API endpoint.
// It's a variable (not a const) to allow overriding in tests.
var embeddingsEndpoint = "https://api.openai.com/v1/embeddings"
