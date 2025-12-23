# OpenAI Embeddings Implementation

## Overview

Implement OpenAI embeddings support for converting text into vector representations. This is useful for semantic search, clustering, and similarity comparisons in the context of Obsidian vault content.

## API Details

### Endpoint

```
POST https://api.openai.com/v1/embeddings
```

### Authentication

Include API key in request headers:

```
Authorization: Bearer {OPENAI_API_KEY}
Content-Type: application/json
```

The API key should be read from the `OPENAI_API_KEY` environment variable or passed as a constructor parameter.

## Supported Models

### text-embedding-3-small

- **Cost**: $0.02 per 1M tokens
- **Use case**: Most cost-effective for general embeddings
- **Max input**: 8,191 tokens per input

### text-embedding-3-large

- **Cost**: $0.13 per 1M tokens
- **Use case**: Higher quality embeddings for better accuracy
- **Max input**: 8,191 tokens per input

## Request Payload

```json
{
  "input": ["text to embed", "another text", "..."],
  "model": "text-embedding-3-small",
  "encoding_format": "float",
  "dimensions": 1536
}
```

### Fields:

- `input` (required): Array of strings to embed. Can be a single string or batch of strings.
- `model` (required): One of `"text-embedding-3-small"` or `"text-embedding-3-large"`, behind an EmbeddingModelID string type
- `encoding_format` (optional): Use `"float"` for standard float arrays. Alternative is `"base64"` for efficiency but adds complexity.
- `dimensions` (optional): Can reduce dimensionality for `text-embedding-3-*` models. Defaults to model's native dimensions if not specified.

## Response Format

```json
{
    "object": "list",
    "data": [
        {
            "object": "embedding",
            "index": 0,
            "embedding": [0.123, -0.456, 0.789, ...]
        },
        {
            "object": "embedding",
            "index": 1,
            "embedding": [0.234, -0.567, 0.890, ...]
        }
    ],
    "model": "text-embedding-3-small",
    "usage": {
        "prompt_tokens": 12,
        "total_tokens": 12
    }
}
```

### Key Fields:

- `data`: Array of embedding objects, ordered by input index
- `data[].embedding`: Array of floats representing the vector
- `usage.prompt_tokens`: Number of tokens processed (used for cost calculation)

## Implementation Structure

### Type Definition

```go
type EmbeddingModelID string

const (
    EmbeddingModelSmall EmbeddingModelID = "text-embedding-3-small"
    EmbeddingModelLarge EmbeddingModelID = "text-embedding-3-large"
)
```

### Result Type

```go
type EmbeddingsResult struct {
    Vectors    [][]float64
    DollarCost float64
}
```

### Main Struct

```go
type OpenAIEmbeddings struct {
    modelID    EmbeddingModelID
    apiKey     string
    client     *http.Client
}
```

### Constructor

```go
func NewOpenAIEmbeddings(modelID EmbeddingModelID, apiKey string, client *http.Client) (*OpenAIEmbeddings, error)
```

- If `apiKey` is empty string, read from `OPENAI_API_KEY` environment variable
- If `client` is nil, create a default `http.Client`
- Return error if no API key is available

### Embed Method

```go
func (e *OpenAIEmbeddings) Embed(ctx context.Context, inputs []string, dimensions *int) (*EmbeddingsResult, error)
```

- Takes a slice of input strings (batch support)
- Optional `dimensions` parameter (pass nil to use defaults)
- Returns vectors in same order as inputs
- Calculates cost based on model and token usage

## Cost Calculation

Dont literally do it this way, see openai/model.go or anthropic/model.go for a reference on units (we use int64)

```go
costPerToken := map[EmbeddingModelID]float64{
    EmbeddingModelSmall: 0.02 / 1_000_000,  // $0.02 per 1M tokens
    EmbeddingModelLarge: 0.13 / 1_000_000,  // $0.13 per 1M tokens
}

dollarCost := costPerToken[modelID] * float64(usage.PromptTokens)
```

## Error Handling

- Check HTTP response status
- If not successful, attempt to parse error response JSON

## Usage Example

```go
embeddings, err := NewOpenAIEmbeddings(EmbeddingModelSmall, "", nil)
if err != nil {
    return err
}

result, err := embeddings.Embed(ctx, []string{
    "The quick brown fox",
    "jumps over the lazy dog",
}, nil)
if err != nil {
    return err
}

// result.Vectors[0] contains embedding for "The quick brown fox"
// result.Vectors[1] contains embedding for "jumps over the lazy dog"
// result.DollarCost contains total cost of the API call
```

## Files to Create

- `agg/embeddings/openai.go` - Main implementation
- `agg/embeddings/openai_test.go` - Unit tests (can use mocked HTTP responses)

## Testing Considerations

- Mock HTTP responses for unit tests
- Test both models (small and large)
- Test batch processing (multiple inputs)
- Test optional dimensions parameter
- Test cost calculation accuracy
- Test error handling for API failures
- Test environment variable fallback for API key

## Notes

- The `data` array in the response is always ordered by the `index` field, matching input order
- Consider supporting `encoding_format: "base64"` in the future for efficiency, but start with `"float"` for simplicity
- Embeddings are typically normalized (unit length) by OpenAI, but verify if needed for your use case
