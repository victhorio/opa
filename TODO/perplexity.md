# Perplexity Web Search Tools

## Overview

Implement two web search tools using the Perplexity API:

1. **WebSearch** - Direct web search returning raw search results (top 5)
2. **AgenticWebSearch** - AI-powered search using Perplexity's Sonar models with grounded responses

These tools can be registered with the Agent to enable web search capabilities.

## API Authentication

Both tools require a Perplexity API key:

- Read from `PERPLEXITY_API_KEY` environment variable
- Include in request headers: `Authorization: Bearer {api_key}`

## Tool 1: WebSearch

### Purpose

Performs a direct web search and returns up to 5 results with content snippets (max 1024 tokens per page).

### API Endpoint

```
POST https://api.perplexity.ai/search
```

### Request Payload

```json
{
  "query": "search query string",
  "max_results": 5,
  "max_tokens_per_page": 1024
}
```

### Response Format

```json
{
    "results": [
        {
            "title": "Page Title",
            "url": "https://example.com",
            "snippet": "Relevant text snippet...",
            "date": "2024-01-01"
        },
        ...
    ]
}
```

### Tool Specification

The tool should have this schema for the LLM:

```
Name: WebSearch

Description:
Use this tool to search information on the web based on a search query.

Prefer to use specific queries. For example, "artificial intelligence medical diagnosis
accuracy" is much better than "AI medical".

You will get up to 1024 tokens worth of content for the top-5 most relevant results.

Parameters:
- query (string, required): The search query used for the web search
```

### Implementation Notes

- Timeout: 10 seconds
- On API error (non-2xx status), return error message as string: `[WebSearch API returned status code {code}]`
- Return the `results` array as JSON string
- If no `results` key in response, return an error

## Tool 2: AgenticWebSearch

### Purpose

Uses Perplexity's Sonar LLM models to search the web and provide an AI-generated answer grounded in search results. Supports both standard and reasoning modes.

### API Endpoint

```
POST https://api.perplexity.ai/chat/completions
```

### Request Payload

```json
{
  "model": "sonar-pro",
  "messages": [
    {
      "role": "user",
      "content": "prompt for the agent"
    }
  ],
  "web_search_options": {
    "search_context_size": "low"
  }
}
```

**For reasoning mode:**

- `model`: `"sonar-reasoning-pro"` (instead of `"sonar-pro"`)
- `web_search_options.search_context_size`: `"medium"` (instead of `"low"`)

### Response Format

```json
{
  "choices": [
    {
      "message": {
        "content": "AI-generated answer with inline citations like [1][2]"
      }
    }
  ],
  "search_results": [
    {
      "title": "Source Title",
      "url": "https://example.com",
      "date": "2024-01-01"
    }
  ],
  "usage": {
    "cost": {
      "total_cost": 0.00123
    }
  }
}
```

### Tool Specification

```
Name: AgenticWebSearch

Description:
This tool allows you to have an agent search the web for information based on a prompt.
This leverages Perplexity's grounded Sonar LLM to search the web and provide a useful answer
grounded in the search results that are also included in the response.

Use `prompt` as a message to the Sonar LLM indicating its task/the answer it needs to provide.

Use `reasoning` to enable a more capable reasoning agent that can search through more sources
and provide more built-in inference on top of the search results. This is suitable for
reasoning, inference and speculation on top of the search results - not for simple factual
queries/information retrieval.

Parameters:
- prompt (string, required): The prompt for the agent to use to search the web and provide a useful answer to you
- reasoning (bool, required): Whether to enable a more capable reasoning agent that can search through more sources and provide more built-in inference on top of the search results
```

### Response Processing

#### Standard Mode (`reasoning: false`)

Return formatted response:

```
<result>
{content from choices[0].message.content}
</result>

<references>
- [1] {title} ({date}) [{url}]
- [2] {title} ({date}) [{url}]
...
</references>
```

#### Reasoning Mode (`reasoning: true`)

The content includes `<think>...</think>` blocks that should be stripped:

1. Find the `</think>` marker in the content
2. Remove everything up to and including `</think>`
3. Use remaining content in the formatted response

If no `</think>` marker found, log a warning and use content as-is.

### Implementation Notes

- Timeout: 30 seconds for standard mode, 60 seconds for reasoning mode
- On API error, return: `[AgenticWebSearch API returned status code {code}]`
- Citations in content are 1-based indices like `[1]`, `[2][3]`, etc.
- `search_results` array order matches citation indices
- Format references with title, date (use "N/A" if missing), and URL

## Implementation Structure

### Factory Functions

```go
func CreateWebSearchTool(client *http.Client) (Tool, error)
func CreateAgenticWebSearchTool(client *http.Client) (Tool, error)
```

Both should:

1. Check for `PERPLEXITY_API_KEY` environment variable
2. Return error if not set
3. Create and return a Tool with appropriate spec and handler

### Tool Handlers

WebSearch handler signature:

```go
func(ctx context.Context, query string) (string, error)
```

AgenticWebSearch handler signature:

```go
func(ctx context.Context, prompt string, reasoning bool) (string, error)
```

## Error Handling

For both tools:

- Check HTTP response status
- Return descriptive error messages
- Handle missing API keys
- Handle malformed responses
- Handle timeouts appropriately

## Usage Example

```go
// Create tools
webSearch, err := CreateWebSearchTool(client)
if err != nil {
    return err
}

agenticSearch, err := CreateAgenticWebSearchTool(client)
if err != nil {
    return err
}

// Register with agent
agent := NewAgent(
    systemPrompt,
    model,
    store,
    []Tool{webSearch, agenticSearch},
)
```

## Files to Create

- `agg/tools/perplexity.go` - Main implementation with both tools
- `agg/tools/perplexity_test.go` - Unit tests (with mocked HTTP)

## Testing Considerations

- Mock HTTP responses for both APIs
- Test both reasoning modes for AgenticWebSearch
- Test error handling (missing API key, API failures)
- Test response parsing and formatting
- Test reasoning content stripping (`<think>` tags)
- Test citation enrichment
- Verify tool specifications are correct for LLM usage
