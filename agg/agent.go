package agg

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/victhorio/opa/agg/com"
	"github.com/victhorio/opa/agg/openai"
)

type Agent struct {
	sysPrompt       string
	model           string
	reasoningEffort string
	store           Store
	tools           ToolRegistry
	toolSpecs       []com.Tool
}

func NewAgent(
	sysPrompt string,
	model string,
	reasoningEffort string,
	store Store,
	tools []Tool,
) Agent {
	a := Agent{
		sysPrompt:       sysPrompt,
		model:           model,
		reasoningEffort: reasoningEffort,
		store:           store,
		toolSpecs:       make([]com.Tool, 0, len(tools)),
	}

	if len(tools) > 0 {
		a.tools = NewToolRegistry()
	}

	for _, tool := range tools {
		a.tools.Register(tool.Spec.Name, tool.Handler)
		a.toolSpecs = append(a.toolSpecs, tool.Spec)
	}

	return a
}

func (a *Agent) Run(
	ctx context.Context,
	client *http.Client,
	sessionID string,
	input string,
	includeInternals bool,
) (string, error) {
	ctxChild, cancel := context.WithCancel(ctx)
	defer cancel()

	msgs := a.store.Messages(sessionID)
	// let's remember up to which idx of `msgs` we already have it stored
	msgsStoreIdx := len(msgs)

	// if it's the first message for this session, we need to include system prompt
	if msgsStoreIdx == 0 {
		msgs = append(msgs, com.NewMessageContent("system", a.sysPrompt))
	}
	msgs = append(msgs, com.NewMessageContent("user", input))

	var usage com.Usage
	var out bytes.Buffer

	for range agentRoundsMax {
		// TODO: if we know it's the last round, disable tools in the OpenAI call

		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("Agent.Run: context error: %w", err)
		}

		stream, err := openai.OpenStream(
			ctxChild,
			client,
			msgs,
			a.model,
			a.reasoningEffort,
			a.toolSpecs,
		)
		if err != nil {
			return "", fmt.Errorf("Agent.Run: error opening stream: %w", err)
		}

		events := make(chan openai.Event, 1)
		go stream.Consume(ctxChild, events)

		var resp com.Response
		var toolCallCount int
		toolResults := make(chan com.ToolResult, 4)
		for event := range events {
			switch event.Type {
			case openai.ETToolCall:
				// let's immediately start running the tool call
				toolCallCount++

				tc := event.Call
				go func() {
					toolResult := com.ToolResult{ID: tc.ID}

					result, err := a.tools.Call(ctxChild, tc.Name, []byte(tc.Arguments))
					if err != nil {
						toolResult.Result = fmt.Sprintf("error calling tool %s: %v", tc.Name, err)
					} else {
						toolResult.Result = result
					}

					select {
					case <-ctxChild.Done():
					case toolResults <- toolResult:
					}
				}()

				if includeInternals {
					fmt.Fprintf(&out, "\n[Tool Call: %s, %s, %s]\n\n", tc.Name, tc.ID, tc.Arguments)
				}
			case openai.ETDeltaReasoning:
				if includeInternals {
					fmt.Fprintf(&out, "\n[Reasoning: %s]\n\n", event.Delta)
				}
			case openai.ETResponse:
				resp = event.Response

				lastMsg := resp.Messages[len(resp.Messages)-1]
				content, ok := lastMsg.Content()
				if ok {
					out.WriteString(content.Text)
				}
			case openai.ETError:
				return "", fmt.Errorf("Agent.Run: error during stream: %w", event.Err)
			}
		}

		msgs = append(msgs, resp.Messages...)
		usage.Inc(resp.Usage)

		if toolCallCount == 0 {
			break
		}

		// we need to collect the tool call results
		for range toolCallCount {
			select {
			case <-ctx.Done():
				return "", fmt.Errorf("Agent.Run: context error: %w", ctx.Err())
			case toolResult := <-toolResults:
				msgs = append(msgs, com.NewMessageToolResult(toolResult.ID, toolResult.Result))

				if includeInternals {
					fmt.Fprintf(&out, "\n[Tool Result: %s, %s]\n\n", toolResult.ID, toolResult.Result)
				}
			}
		}
	}

	// before returning, let's update the store
	err := a.store.Extend(sessionID, msgs[msgsStoreIdx:], usage)
	if err != nil {
		return "", fmt.Errorf("Agent.Run: error extending store: %w", err)
	}

	return out.String(), nil
}

const (
	agentRoundsMax = 4
)
