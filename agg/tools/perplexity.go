package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/victhorio/opa/agg"
	"github.com/victhorio/opa/agg/core"
)

const (
	perplexitySearchURL      = "https://api.perplexity.ai/search"
	perplexityCompletionsURL = "https://api.perplexity.ai/chat/completions"

	webSearchTimeout        = 10 * time.Second
	agenticSearchTimeout    = 30 * time.Second
	agenticSearchTimeoutExt = 60 * time.Second
)

// CreateWebSearchTool creates a tool that performs direct web searches using Perplexity API.
// Returns up to 5 results with content snippets (max 1024 tokens per page).
func CreateWebSearchTool(client *http.Client) (agg.Tool, error) {
	apiKey := os.Getenv("PERPLEXITY_API_KEY")
	if apiKey == "" {
		return agg.Tool{}, fmt.Errorf("PERPLEXITY_API_KEY environment variable not set")
	}

	spec := core.Tool{
		Name: "WebSearch",
		Desc: `Use this tool to search information on the web based on a search query.

Prefer to use specific queries. For example, "artificial intelligence medical diagnosis accuracy" is much better than "AI medical".

You will get up to 1024 tokens worth of content for the top-5 most relevant results.`,
		Params: map[string]core.ToolParam{
			"query": {
				Type: core.JSTString,
				Desc: "The search query used for the web search",
			},
		},
	}

	handler := func(ctx context.Context, args struct {
		Query string `json:"query"`
	}) (string, error) {
		// TODO(logging): log the search query and API call timing

		reqBody := webSearchRequest{
			Query:            args.Query,
			MaxResults:       5,
			MaxTokensPerPage: 1024,
		}

		respBody, statusCode, err := makePerplexityRequest(
			ctx,
			client,
			apiKey,
			perplexitySearchURL,
			reqBody,
			webSearchTimeout,
		)
		if err != nil {
			// TODO(logging): log the error details
			return "", fmt.Errorf("WebSearch: error making API request: %w", err)
		}

		if statusCode != http.StatusOK {
			// TODO(logging): log the status code and response body
			return fmt.Sprintf("[WebSearch API returned status code %d]", statusCode), nil
		}

		var resp webSearchResponse
		if err := json.Unmarshal(respBody, &resp); err != nil {
			// TODO(logging): log the parse error and raw response
			return "", fmt.Errorf("WebSearch: error parsing response: %w", err)
		}

		if resp.Results == nil {
			// TODO(logging): log unexpected response structure
			return "[WebSearch API returned response without results]", nil
		}

		// Return the results as JSON string
		resultsJSON, err := json.Marshal(resp.Results)
		if err != nil {
			return "", fmt.Errorf("WebSearch: error marshaling results: %w", err)
		}

		return string(resultsJSON), nil
	}

	return agg.NewTool(handler, spec), nil
}

// CreateAgenticWebSearchTool creates a tool that uses Perplexity's Sonar LLM models to search
// the web and provide AI-generated answers grounded in search results.
func CreateAgenticWebSearchTool(client *http.Client) (agg.Tool, error) {
	apiKey := os.Getenv("PERPLEXITY_API_KEY")
	if apiKey == "" {
		return agg.Tool{}, fmt.Errorf("PERPLEXITY_API_KEY environment variable not set")
	}

	spec := core.Tool{
		Name: "AgenticWebSearch",
		Desc: `This tool allows you to have an agent search the web for information based on a prompt. This leverages Perplexity's grounded Sonar LLM to search the web and provide a useful answer grounded in the search results that are also included in the response.

Use 'prompt' as a message to the Sonar LLM indicating its task/the answer it needs to provide.

Use 'reasoning' to enable a more capable reasoning agent that can search through more sources and provide more built-in inference on top of the search results. This is suitable for reasoning, inference and speculation on top of the search results - not for simple factual queries/information retrieval.`,
		Params: map[string]core.ToolParam{
			"prompt": {
				Type: core.JSTString,
				Desc: "The prompt for the agent to use to search the web and provide a useful answer to you",
			},
			"reasoning": {
				Type: core.JSTBoolean,
				Desc: "Whether to enable a more capable reasoning agent that can search through more sources and provide more built-in inference on top of the search results",
			},
		},
	}

	handler := func(ctx context.Context, args struct {
		Prompt    string `json:"prompt"`
		Reasoning bool   `json:"reasoning"`
	}) (string, error) {
		// TODO(logging): log the prompt, reasoning mode, and API call timing

		// Choose model and context size based on reasoning mode
		model := "sonar-pro"
		contextSize := "low"
		timeout := agenticSearchTimeout
		if args.Reasoning {
			model = "sonar-reasoning-pro"
			contextSize = "medium"
			timeout = agenticSearchTimeoutExt
		}

		reqBody := agenticSearchRequest{
			Model: model,
			Messages: []agenticSearchMessage{
				{
					Role:    "user",
					Content: args.Prompt,
				},
			},
			WebSearchOptions: &agenticSearchWebSearchOpt{
				SearchContextSize: contextSize,
			},
		}

		respBody, statusCode, err := makePerplexityRequest(
			ctx,
			client,
			apiKey,
			perplexityCompletionsURL,
			reqBody,
			timeout,
		)
		if err != nil {
			// TODO(logging): log the error details
			return "", fmt.Errorf("AgenticWebSearch: error making API request: %w", err)
		}

		if statusCode != http.StatusOK {
			// TODO(logging): log the status code and response body
			return fmt.Sprintf("[AgenticWebSearch API returned status code %d]", statusCode), nil
		}

		var resp agenticSearchResponse
		if err := json.Unmarshal(respBody, &resp); err != nil {
			// TODO(logging): log the parse error and raw response
			return "", fmt.Errorf("AgenticWebSearch: error parsing response: %w", err)
		}

		if len(resp.Choices) == 0 {
			// TODO(logging): log unexpected response structure
			return "[AgenticWebSearch API returned no choices]", nil
		}

		content := resp.Choices[0].Message.Content

		// In reasoning mode, strip <think>...</think> blocks
		if args.Reasoning {
			content = stripThinkTags(content)
		}

		// Format the response with result and references
		return formatAgenticSearchResponse(content, resp.SearchResults), nil
	}

	return agg.NewTool(handler, spec), nil
}

// WebSearch API types

type webSearchRequest struct {
	Query            string `json:"query"`
	MaxResults       int    `json:"max_results"`
	MaxTokensPerPage int    `json:"max_tokens_per_page"`
}

type webSearchResponse struct {
	Results []webSearchResult `json:"results"`
}

type webSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Date    string `json:"date"`
}

// AgenticWebSearch API types

type agenticSearchRequest struct {
	Model            string                     `json:"model"`
	Messages         []agenticSearchMessage     `json:"messages"`
	WebSearchOptions *agenticSearchWebSearchOpt `json:"web_search_options"`
}

type agenticSearchMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type agenticSearchWebSearchOpt struct {
	SearchContextSize string `json:"search_context_size"`
}

type agenticSearchResponse struct {
	Choices       []agenticSearchChoice `json:"choices"`
	SearchResults []agenticSearchSource `json:"search_results"`
	Usage         *agenticSearchUsage   `json:"usage,omitempty"`
}

type agenticSearchChoice struct {
	Message agenticSearchMessage `json:"message"`
}

type agenticSearchSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Date  string `json:"date"`
}

type agenticSearchUsage struct {
	Cost *agenticSearchCost `json:"cost,omitempty"`
}

type agenticSearchCost struct {
	TotalCost float64 `json:"total_cost"`
}

// makePerplexityRequest makes an HTTP request to the Perplexity API with the given parameters.
// Returns the response body, status code, and any error.
func makePerplexityRequest(
	ctx context.Context,
	client *http.Client,
	apiKey string,
	url string,
	reqBody any,
	timeout time.Duration,
) ([]byte, int, error) {
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, 0, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Create a context with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req = req.WithContext(ctxWithTimeout)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("error reading response body: %w", err)
	}

	return body, resp.StatusCode, nil
}

// stripThinkTags removes everything up to and including the </think> marker in reasoning mode.
// If no </think> marker is found, returns the content as-is.
func stripThinkTags(content string) string {
	_, after, ok := strings.Cut(content, "</think>")
	if !ok {
		// TODO(logging): log warning that no </think> marker was found in reasoning mode
		return content
	}
	return after
}

// formatAgenticSearchResponse formats the agentic search response with result and references.
func formatAgenticSearchResponse(content string, sources []agenticSearchSource) string {
	var sb strings.Builder

	sb.WriteString("<result>\n")
	sb.WriteString(content)
	sb.WriteString("\n</result>\n\n<references>\n")

	for i, source := range sources {
		date := source.Date
		if date == "" {
			date = "N/A"
		}
		// Citations are 1-based indices: [1], [2], etc.
		fmt.Fprintf(&sb, "- [%d] %s (%s) [%s]\n", i+1, source.Title, date, source.URL)
	}

	sb.WriteString("</references>")

	return sb.String()
}
