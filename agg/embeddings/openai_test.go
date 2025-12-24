package embeddings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victhorio/opa/agg/core"
)

func TestOpenAIEmbeddings_Embed(t *testing.T) {
	// Note: not running in parallel because we modify the global embeddingsEndpoint variable

	tests := []struct {
		name       string
		modelID    EmbeddingModelID
		inputs     []string
		dimensions *int
		wantErr    bool
		mockResp   string
		statusCode int
	}{
		{
			name:    "single input",
			modelID: OpenAISmall,
			inputs:  []string{"hello world"},
			mockResp: `{
				"object": "list",
				"data": [
					{
						"object": "embedding",
						"index": 0,
						"embedding": [0.1, 0.2, 0.3]
					}
				],
				"model": "text-embedding-3-small",
				"usage": {
					"prompt_tokens": 2,
					"total_tokens": 2
				}
			}`,
			statusCode: 200,
			wantErr:    false,
		},
		{
			name:    "multiple inputs",
			modelID: OpenAISmall,
			inputs:  []string{"hello", "world"},
			mockResp: `{
				"object": "list",
				"data": [
					{
						"object": "embedding",
						"index": 0,
						"embedding": [0.1, 0.2]
					},
					{
						"object": "embedding",
						"index": 1,
						"embedding": [0.3, 0.4]
					}
				],
				"model": "text-embedding-3-small",
				"usage": {
					"prompt_tokens": 4,
					"total_tokens": 4
				}
			}`,
			statusCode: 200,
			wantErr:    false,
		},
		{
			name:    "with dimensions",
			modelID: OpenAISmall,
			inputs:  []string{"test"},
			dimensions: func() *int {
				d := 512
				return &d
			}(),
			mockResp: `{
				"object": "list",
				"data": [
					{
						"object": "embedding",
						"index": 0,
						"embedding": [0.5, 0.6]
					}
				],
				"model": "text-embedding-3-small",
				"usage": {
					"prompt_tokens": 1,
					"total_tokens": 1
				}
			}`,
			statusCode: 200,
			wantErr:    false,
		},
		{
			name:       "empty inputs",
			modelID:    OpenAISmall,
			inputs:     []string{},
			statusCode: 200,
			wantErr:    true,
		},
		{
			name:       "API error",
			modelID:    OpenAISmall,
			inputs:     []string{"test"},
			mockResp:   `{"error": {"message": "Invalid API key"}}`,
			statusCode: 401,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and headers
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
				}

				// Decode and verify request body if we're expecting success
				if tt.statusCode == 200 && !tt.wantErr {
					var req embeddingRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Errorf("Failed to decode request body: %v", err)
					} else {
						if req.Model != tt.modelID {
							t.Errorf("Expected model %s, got %s", tt.modelID, req.Model)
						}
						if len(req.Input) != len(tt.inputs) {
							t.Errorf("Expected %d inputs, got %d", len(tt.inputs), len(req.Input))
						}
						if tt.dimensions != nil && (req.Dimensions == nil || *req.Dimensions != *tt.dimensions) {
							t.Errorf("Expected dimensions %v, got %v", tt.dimensions, req.Dimensions)
						}
					}
				}

				// Send mock response
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.mockResp))
			}))
			defer server.Close()

			// Create embeddings client with custom endpoint
			emb, err := NewOpenAIEmbedder(tt.modelID, server.Client())
			if err != nil {
				t.Fatalf("NewOpenAIEmbeddings() error = %v", err)
			}

			// Override the endpoint for testing
			oldEndpoint := embeddingsEndpoint
			embeddingsEndpoint = server.URL
			defer func() { embeddingsEndpoint = oldEndpoint }()

			// Call Embed
			ctx := context.Background()
			result, err := emb.Embed(ctx, tt.inputs, tt.dimensions)

			if (err != nil) != tt.wantErr {
				t.Errorf("Embed() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result == nil {
					t.Error("Embed() returned nil result")
					return
				}
				if len(result.Vectors) != len(tt.inputs) {
					t.Errorf("Expected %d vectors, got %d", len(tt.inputs), len(result.Vectors))
				}
				for i, vec := range result.Vectors {
					if vec == nil {
						t.Errorf("Vector at index %d is nil", i)
					}
				}
				if result.Cost < 0 {
					t.Errorf("Cost is negative: %d", result.Cost)
				}
			}
		})
	}
}

func TestOpenAIEmbeddings_CostCalculation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		modelID        EmbeddingModelID
		tokens         int64
		expectedCost   int64
		toleranceRatio float64 // relative tolerance for floating point comparison
	}{
		{
			name:         "small model 1000 tokens",
			modelID:      OpenAISmall,
			tokens:       1000,
			expectedCost: 20000,
		},
		{
			name:         "small model 1M tokens",
			modelID:      OpenAISmall,
			tokens:       1_000_000,
			expectedCost: 20000000,
		},
		{
			name:         "large model 1000 tokens",
			modelID:      OpenAILarge,
			tokens:       1000,
			expectedCost: 13000000,
		},
		{
			name:         "large model 1M tokens",
			modelID:      OpenAILarge,
			tokens:       1_000_000,
			expectedCost: 1300000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			emb := &OpenAIEmbeddings{modelID: tt.modelID}
			cost := emb.calculateCost(tt.tokens)

			diff := cost - tt.expectedCost
			if diff != 0 {
				t.Errorf("calculateCost() = %d, want %d", cost, tt.expectedCost)
			}
		})
	}
}

func TestOpenAIEmbeddings_InterfaceCompliance(t *testing.T) {
	t.Parallel()

	// This is a compile-time check that OpenAIEmbeddings implements core.Embedder
	var _ core.Embedder = (*OpenAIEmbeddings)(nil)
}

func TestOpenAIEmbeddings_Embed_OutOfOrderResponse(t *testing.T) {
	// Note: not running in parallel because we modify the global embeddingsEndpoint variable

	// Test that embeddings are correctly ordered even if the API returns them out of order
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return embeddings in reverse order
		resp := `{
			"object": "list",
			"data": [
				{
					"object": "embedding",
					"index": 2,
					"embedding": [0.5, 0.6]
				},
				{
					"object": "embedding",
					"index": 1,
					"embedding": [0.3, 0.4]
				},
				{
					"object": "embedding",
					"index": 0,
					"embedding": [0.1, 0.2]
				}
			],
			"model": "text-embedding-3-small",
			"usage": {
				"prompt_tokens": 6,
				"total_tokens": 6
			}
		}`
		w.WriteHeader(200)
		w.Write([]byte(resp))
	}))
	defer server.Close()

	emb, err := NewOpenAIEmbedder(OpenAISmall, server.Client())
	if err != nil {
		t.Fatalf("NewOpenAIEmbeddings() error = %v", err)
	}

	oldEndpoint := embeddingsEndpoint
	embeddingsEndpoint = server.URL
	defer func() { embeddingsEndpoint = oldEndpoint }()

	inputs := []string{"first", "second", "third"}
	result, err := emb.Embed(context.Background(), inputs, nil)
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	// Verify vectors are in the correct order
	if len(result.Vectors) != 3 {
		t.Fatalf("Expected 3 vectors, got %d", len(result.Vectors))
	}

	expectedVectors := [][]float64{
		{0.1, 0.2},
		{0.3, 0.4},
		{0.5, 0.6},
	}

	for i, expected := range expectedVectors {
		if len(result.Vectors[i]) != len(expected) {
			t.Errorf("Vector %d has length %d, want %d", i, len(result.Vectors[i]), len(expected))
			continue
		}
		for j, val := range expected {
			if result.Vectors[i][j] != val {
				t.Errorf("Vector[%d][%d] = %f, want %f", i, j, result.Vectors[i][j], val)
			}
		}
	}
}
