package agg

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/victhorio/opa/agg/core"
)

type Agent struct {
	Store Store

	sysPrompt string
	model     core.Model
	tools     ToolRegistry
	toolSpecs []core.Tool
}

func NewAgent(
	sysPrompt string,
	model core.Model,
	store Store,
	tools []Tool,
) Agent {
	a := Agent{
		sysPrompt: sysPrompt,
		model:     model,
		Store:     store,
		toolSpecs: make([]core.Tool, 0, len(tools)),
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

	msgs := a.Store.Messages(sessionID)
	// let's remember up to which idx of `msgs` we already have it stored
	msgsStoreIdx := len(msgs)

	// if it's the first message for this session, we need to include system prompt
	if msgsStoreIdx == 0 {
		msgs = append(msgs, core.NewMsgContent("system", a.sysPrompt))
	}
	msgs = append(msgs, core.NewMsgContent("user", input))

	var usage core.Usage
	var out bytes.Buffer

	for round := range agentRoundsMax {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("Agent.Run: context error: %w", err)
		}

		cfg := core.StreamCfg{}
		if round == agentRoundsMax-1 {
			// When we're at the last round, we need to behave differently between OpenAI and
			// Anthropic models due to different behaviors from them.
			//
			// **OpenAI**
			//
			// If we simply forbit tool choice for the OpenAI model at this point, it will generate
			// a confused response because it will actually be /completely unaware/ of the available
			// tools and more importantly, it will not have visibility of the tool calls/results
			// already made. So it ends up generating a confused message about not being able to
			// do anything at all, whereas if we got here, it 100% already did quite a bit of calls.
			//
			// Thankfully, OpenAI particularly allows system messages in the middle of the
			// conversation history, so we can just add a system message here indicating that the
			// harness forbids another tool call without an intermediate user interaction first,
			// driving it to create a regular message.
			//
			// **Anthropic**
			//
			// Anthropic models still preserve visibility of their tool calls even if we force it
			// to not use them, so we can just disable them directly. Unfortunately, Anthropic does
			// not allow system messages in the middle of the conversation history. This would've
			// been helpful because currently the model mostly messages something like "Now I'll
			// make this tool call:" (and actually doesnt't), so the UX is not perfect but fine.

			switch a.model.Provider() {
			case core.ProviderOpenAI:
				msgs = append(msgs, core.NewMsgContent("system", toolCallLimitReachedPrompt))
			case core.ProviderAnthropic:
				cfg.DisableTools = true
			}
		}

		stream, err := a.model.OpenStream(
			ctxChild,
			client,
			msgs,
			a.toolSpecs,
			cfg,
		)
		if err != nil {
			return "", fmt.Errorf("Agent.Run: error opening stream: %w", err)
		}

		events := make(chan core.Event, 1)
		go stream.Consume(ctxChild, events)

		var resp core.Response
		var toolCallCount int
		toolResults := make(chan core.ToolResult, 4)
		for event := range events {
			switch event.Type {
			case core.EvToolCall:
				// let's immediately start running the tool call
				toolCallCount++

				tc := event.Call
				go func() {
					toolResult := core.ToolResult{ID: tc.ID}

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
			case core.EvDeltaReason:
				if includeInternals {
					fmt.Fprintf(&out, "\n[Reasoning: %s]\n\n", event.Delta)
				}
			case core.EvResp:
				resp = event.Response

				lastMsg := resp.Messages[len(resp.Messages)-1]
				content, ok := lastMsg.AsContent()
				if ok {
					out.WriteString(content.Text)
				}
			case core.EvError:
				return "", fmt.Errorf("Agent.Run: error during stream: %w", event.Err)
			}
		}

		msgs = append(msgs, resp.Messages...)
		usage.Inc(resp.Usage)

		if toolCallCount == 0 {
			// We only ever need to loop if the agent is generating tool calls instead of an actual
			// response. If no tool calls were collected, there's nothing to loop for.
			break
		}

		// Collect the tool results.
		for range toolCallCount {
			select {
			case <-ctx.Done():
				return "", fmt.Errorf("Agent.Run: context error: %w", ctx.Err())
			case toolResult := <-toolResults:
				msgs = append(msgs, core.NewMsgToolResult(toolResult.ID, toolResult.Result))

				if includeInternals {
					fmt.Fprintf(&out, "\n[Tool Result: %s, %s]\n\n", toolResult.ID, toolResult.Result)
				}
			}
		}
	}

	// Before returning, we need to update the store so that the conversation history persists
	// correctly. We need to store every message starting from `msgsStoreIdx` onwards (the previous
	// ones were already fetched from store, so we don't want duplication).
	//
	// There is an important detail here: we'll skip any reasoning messages. This is because while
	// we /could/ preserve them in the history, both OpenAI and Anthropic will ignore/discard them
	// from the context /unless/ the reasoning block precedes an ongoing tool call loop. I.e. they
	// only readd reasoning to the context if (1) the reasoning block precedes a tool call and (2)
	// no user messages exist after this tool call yet. Since we already carried out the tool call
	// loop above, there's no reason to ever waste resources storing these reasoning loops.
	msgsToStore := make([]*core.Msg, 0, len(msgs)-msgsStoreIdx)
	for _, msg := range msgs[msgsStoreIdx:] {
		if msg.Type != core.MsgTypeReasoning {
			msgsToStore = append(msgsToStore, msg)
		}
	}

	err := a.Store.Extend(sessionID, msgsToStore, usage)
	if err != nil {
		return "", fmt.Errorf("Agent.Run: error extending store: %w", err)
	}

	return out.String(), nil
}

const (
	agentRoundsMax = 4

	toolCallLimitReachedPrompt = `You have reached the maximum number of sequential tool call turns
without an user interaction. Generate a user message this turn. If you need to make further tool
calls, just let the user know and once they respond, you can continue making more tool calls.`
)
