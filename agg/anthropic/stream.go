package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/victhorio/opa/agg/core"
)

type Stream struct {
	stream  io.ReadCloser
	modelID ModelID
}

func (m *Model) OpenStream(
	ctx context.Context,
	client *http.Client,
	messages []core.Msg,
	tools []core.Tool,
	cfg core.StreamCfg,
) (core.ResponseStream, error) {
	// Anthropic takes the system message separate from the other ones.
	sysPrompt, msgs := m.fromCoreMsgs(messages)

	payload := requestBody{
		MaxToks:   m.maxTok,
		Msgs:      msgs,
		Model:     m.model,
		Stream:    true,
		SysPrompt: sysPrompt,
		Tools:     fromCoreTools(tools),
	}

	if m.maxTokReason > 0 {
		payload.Reason = newReasonCfg(true, m.maxTokReason)
	}

	if len(tools) > 0 {
		if cfg.DisableTools {
			payload.ToolCfg = newToolCfg("none", false)
		} else {
			payload.ToolCfg = newToolCfg("auto", false)
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("anthropic.OpenStream: error marshalling request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", messagesEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic.OpenStream: error creating request: %w", err)
	}

	req.Header.Set("X-Api-Key", os.Getenv("ANTHROPIC_API_KEY"))
	req.Header.Set("anthropic-version", anthropicApiVersion)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic.OpenStream: error sending request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()

		if resp.StatusCode == 400 {
			// Let's save the payload we were sending.
			m, err := json.MarshalIndent(payload, "", "  ")
			if err == nil {
				core.DumpErrorLog("anthropic-400", string(m))
			}
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if err != nil {
			return nil, fmt.Errorf("anthropic.OpenStream: error reading response body: %w", err)
		}
		return nil, fmt.Errorf("anthropic.OpenStream: error response: %s, body=%s", resp.Status, string(body))
	}

	return &Stream{
		stream:  resp.Body,
		modelID: m.model,
	}, nil
}

func (s *Stream) Consume(ctx context.Context, out chan<- core.Event) {
	defer s.stream.Close()
	defer close(out)

	reader := bufio.NewReader(s.stream)

	// We'll store multiple `data:` entries per server side event into this buffer and collect
	// them only when the event is over to be SSE compliant.
	var buf bytes.Buffer

	// We'll need to incrementally build the final resp.
	var resp core.Response

	for {
		// In SSE, newlines are field delimiters.
		line, err := reader.ReadString('\n')
		if err != nil {
			// Getting an EOF /is/ an error, since we should get an stMsgStop first. We
			// intentionally decide to treat it the same as any other error here.
			if !sendEvent(ctx, out, core.NewEvError(err)) {
				return
			}
			continue
		}

		line = strings.TrimRight(line, "\n\r")

		// In SSE, empty lines mean the event is over, so we can read the data we've collected
		// into the buffer (if any).
		if line == "" {
			if buf.Len() == 0 {
				continue
			}

			rawEventBytes := buf.Bytes()
			buf.Reset()

			shouldStop, err := s.dispatchRawEvent(ctx, &resp, rawEventBytes, out)
			if err != nil {
				// if we've had an error dispatching, let's not try dispatching again and just
				// give up
				return
			}

			if shouldStop {
				break
			}

			continue
		}

		// Check if it's a data field. Otherwise we can ignore it.
		if dataBytes, ok := strings.CutPrefix(line, "data:"); ok {
			buf.WriteString(dataBytes)
			// Technically we should add a newline at this point, but since we're expecting a JSON
			// it shouldn't ever make a difference
		}
	}

	// Note that at this point we do not need to worry about doing "one last flush" since EOF is
	// already treated as an error.

	// Finally, we can emit our final event to the listener, which is the complete response.
	_ = sendEvent(ctx, out, core.NewEvResp(resp))
}

// dispatchRawEvent dispatches a raw event from the Anthropic API to the output channel.
// Returns true to indicate the caller should stop consuming, otherwise it's false.
func (s *Stream) dispatchRawEvent(
	ctx context.Context,
	resp *core.Response,
	dataBytes []byte,
	out chan<- core.Event,
) (bool, error) {
	// fmt.Printf("\033[33;1mdispatchRawEvent:\033[0m %s\n", string(dataBytes))

	var ev sse
	if err := json.Unmarshal(dataBytes, &ev); err != nil {
		_ = sendEvent(ctx, out, core.NewEvError(err))
		return true, err
	}

	switch ev.Type {
	case stPing:
		// no-op event from anthropic
	case stMsgStart:
		resp.Model = ev.Message.Model
	case stMsgDelta:
		resp.Usage.Input = ev.Usage.In
		resp.Usage.Cached = ev.Usage.InCacheRead
		resp.Usage.Output = ev.Usage.Out
		resp.Usage.Total = ev.Usage.In + ev.Usage.Out
		resp.Usage.Cost = costFromUsage(s.modelID, ev.Usage)
	case stMsgStop:
		return true, nil
	case stContBlockStart:
		switch ev.ContentBlock.Type {
		case msgContTypeReason:
			resp.Messages = append(resp.Messages, core.NewMsgReasoning("", ""))
		case msgContTypeText:
			resp.Messages = append(resp.Messages, core.NewMsgContent("assistant", ""))
		case msgContTypeTool:
			// in this instance, we get the ID and Name of tools already in this block
			resp.Messages = append(
				resp.Messages,
				core.NewMsgToolCall(ev.ContentBlock.ID, ev.ContentBlock.Name, ""),
			)
		default:
			fmt.Printf("\033[31;1munknown content block type:\033[0m %s\n", ev.ContentBlock.Type)
			// return true, fmt.Errorf("unknown content block type: %s", ev.ContentBlock.Type)
		}
	case stContBlockDelta:
		lastMsg := resp.Messages[len(resp.Messages)-1]
		switch ev.Delta.Type {
		case deltaTypeReasonText:
			reasoning, ok := lastMsg.AsReasoning()
			if !ok {
				// TODO(robust): don't fatal here
				log.Fatalf("last message is not a reasoning message, but got a reasoning delta")
			}

			// TODO(optimize): eventually be smarter here and use a buffer for intermediate building
			reasoning.Text += ev.Delta.Thinking
		case deltaTypeEncrypted:
			reasoning, ok := lastMsg.AsReasoning()
			if !ok {
				// TODO(robust): don't fatal here
				log.Fatalf("last message is not a reasoning message, but got a reasoning delta")
			}

			// TODO(optimize): eventually be smarter here and use a buffer for intermediate building
			reasoning.Encrypted += ev.Delta.Signature
		case deltaTypeText:
			// Besides adding to the response, we also always immediately emit text deltas.
			if ok := sendEvent(ctx, out, core.NewEvDelta(ev.Delta.Text)); !ok {
				return true, fmt.Errorf("context done")
			}

			content, ok := lastMsg.AsContent()
			if !ok {
				// TODO(robust): don't fatal here
				log.Fatalf("last message is not a content message, but got a text delta")
			}

			// TODO(optimize): eventually be smarter here and use a buffer for intermediate building
			content.Text += ev.Delta.Text
		case deltaTypeToolArgs:
			toolCall, ok := lastMsg.AsToolCall()
			if !ok {
				// TODO(robust): don't fatal here
				log.Fatalf("last message is not a tool call message, but got a tool args delta")
			}

			// TODO(optimize): eventually be smarter here and use a buffer for intermediate building
			toolCall.Arguments = string(toolCall.Arguments) + ev.Delta.PartialArgs
		default:
			fmt.Printf("\033[31;1munknown content block delta type:\033[0m %s\n", ev.Delta.Type)
			// return true, fmt.Errorf("unknown content block delta type: %s", ev.Delta.Type)
		}
	case stContBlockStop:
		// Since we're meant to emit reasoning deltas to our listener only when they're complete,
		// this is the correct spot to do it. So, if our last message is a reasoning one, we emit
		// its text. We're also meant to emit tool calls once they're done.
		lastMsg := resp.Messages[len(resp.Messages)-1]
		switch lastMsg.Type {
		case core.MsgTypeToolCall:
			toolCall, _ := lastMsg.AsToolCall()
			if ok := sendEvent(ctx, out, core.NewEvToolCall(*toolCall)); !ok {
				return true, fmt.Errorf("context done")
			}
		case core.MsgTypeReasoning:
			reasoning, _ := lastMsg.AsReasoning()
			if ok := sendEvent(ctx, out, core.NewEvDeltaReason(reasoning.Text)); !ok {
				return true, fmt.Errorf("context done")
			}
		}
	default:
		fmt.Printf("\033[31;1munknown event type:\033[0m %s\n", ev.Type)
	}

	return false, nil
}

// sendEvent sends an event to the output channel while avoiding blocking if context is done.
// Returns true if the event was sent, false if the context is done.
func sendEvent(ctx context.Context, out chan<- core.Event, ev core.Event) bool {
	select {
	case <-ctx.Done():
		return false
	case out <- ev:
		return true
	}
}

// requestBody is the body of the request to the Anthropic messages endpoint
type requestBody struct {
	MaxToks   int        `json:"max_tokens"`
	Msgs      []*msg     `json:"messages"`
	Model     ModelID    `json:"model"`
	Stream    bool       `json:"stream"`
	SysPrompt string     `json:"system,omitempty"`
	Temp      *float64   `json:"temperature,omitempty"`
	Reason    *reasonCfg `json:"thinking,omitempty"`
	ToolCfg   *toolCfg   `json:"tool_choice,omitempty"` // TODO: currently unused
	Tools     []tool     `json:"tools,omitempty"`
}

type reasonCfg struct {
	Type string `json:"type"` // "enabled" or "disabled"
	// must be at least 1024 and less than requestBody.MaxToks, must be set iff Type = "enabled"
	MaxToks *int `json:"budget_tokens,omitempty"`
}

func newReasonCfg(enabled bool, maxToks int) *reasonCfg {
	rc := reasonCfg{Type: "disabled"}
	if enabled && maxToks >= 1024 {
		// TODO(robust): probably would be better to fail than to silently disable reasoning if
		//               maxToks is less than 1024?
		rc.Type = "enabled"
		rc.MaxToks = intPtr(maxToks)
	}
	return &rc
}

type toolCfg struct {
	Type            string `json:"type"`                                // "auto", "any", "none"
	DisableParallel *bool  `json:"disable_parallel_tool_use,omitempty"` // should not be set if Type == "none"
}

func newToolCfg(toolChoice string, disableParallel bool) *toolCfg {
	tc := toolCfg{Type: toolChoice}
	if toolChoice != "none" {
		tc.DisableParallel = boolPtr(disableParallel)
	}
	return &tc
}

type msg struct {
	Role    string        `json:"role"` // "user" or "assistant"
	Content []*msgContent `json:"content"`
}

type msgContent struct {
	Type msgContentType `json:"type"`
	// text fields
	Text string `json:"text,omitempty"`
	// reasoning fields
	Signature string `json:"signature,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	// tool use fields
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name,omitempty"`
	Args json.RawMessage `json:"input,omitempty"`
	// tool result fields
	ToolUseID string `json:"tool_use_id,omitempty"`
	Output    string `json:"content,omitempty"`

	// this are fields used to indicate caching (cannot be used for Reasoning)
	CacheCtrl *cacheCtrl `json:"cache_control,omitempty"`

	// this is only used in SSEvents for tool use, not by us
	PartialArgs string `json:"partial_json,omitempty"`
}

type cacheCtrl struct {
	Type string `json:"type"`          // should always be "ephemeral"
	TTL  string `json:"ttl,omitempty"` // either "5m" or "1h", but cost calculation assumes "5m"
}

type msgContentType string

const (
	msgContTypeText       msgContentType = "text"
	msgContTypeReason     msgContentType = "thinking"
	msgContTypeTool       msgContentType = "tool_use"
	msgContTypeToolResult msgContentType = "tool_result"

	// delta types
	deltaTypeReasonText msgContentType = "thinking_delta"
	deltaTypeEncrypted  msgContentType = "signature_delta"
	deltaTypeText       msgContentType = "text_delta"
	deltaTypeToolArgs   msgContentType = "input_json_delta"
)

func newMsgText(text string) *msgContent {
	return &msgContent{
		Type: msgContTypeText,
		Text: text,
	}
}

func newMsgReason(signature, thinking string) *msgContent {
	return &msgContent{
		Type:      msgContTypeReason,
		Signature: signature,
		Thinking:  thinking,
	}
}

func newMsgToolUse(id, name string, args json.RawMessage) *msgContent {
	return &msgContent{
		Type: msgContTypeTool,
		ID:   id,
		Name: name,
		Args: args,
	}
}

func newMsgToolResult(toolUseID, output string) *msgContent {
	return &msgContent{
		Type:      msgContTypeToolResult,
		ToolUseID: toolUseID,
		Output:    output,
	}
}

func (m *Model) fromCoreMsgs(msgs []core.Msg) (string, []*msg) {
	var sysPrompt string

	// TODO(optimize): len(msgs) is an upper bound, because some messages coalesce we won't reach it
	//                 specially for cases with a lot of tool use.
	r := make([]*msg, 0, len(msgs))

	// For Anthropic messages, sequential messages from the same role must be added together,
	// particularly when both reasoning /and/ tool use are happening. So we need to recall if our
	// last message was from the same role or not.
	var lastRole string

	for _, m := range msgs {
		switch m.Type {
		case core.MsgTypeReasoning:
			reasoning, _ := m.AsReasoning()
			content := newMsgReason(reasoning.Encrypted, reasoning.Text)

			if lastRole == "assistant" {
				// let's just append to the contents
				lastMsg := r[len(r)-1]
				lastMsg.Content = append(lastMsg.Content, content)
			} else {
				r = append(r, &msg{
					Role:    "assistant",
					Content: []*msgContent{content},
				})

				lastRole = "assistant"
			}
		case core.MsgTypeContent:
			contentCore, _ := m.AsContent()
			if contentCore.Role == "system" {
				if sysPrompt == "" {
					sysPrompt = contentCore.Text
				} else {
					// TODO(robust): don't panic here
					// For reference, originally sysPrompt would be a buffer and anytime we found
					// a system message, we would append to the buffer. This has two issues: one,
					// if the placement of the system messages were important, it'd get mangled as
					// everything would be part of a single initial system message. Secondly, this
					// would invalidate all caches.
					panic("multiple system messages not allowed for anthropic")
				}
				continue
			}

			content := newMsgText(contentCore.Text)

			if lastRole == contentCore.Role {
				lastMsg := r[len(r)-1]
				lastMsg.Content = append(lastMsg.Content, content)
			} else {
				r = append(r, &msg{
					Role:    contentCore.Role,
					Content: []*msgContent{content},
				})
				lastRole = contentCore.Role
			}
		case core.MsgTypeToolCall:
			toolCall, _ := m.AsToolCall()
			content := newMsgToolUse(toolCall.ID, toolCall.Name, json.RawMessage(toolCall.Arguments))

			if lastRole == "assistant" {
				lastMsg := r[len(r)-1]
				lastMsg.Content = append(lastMsg.Content, content)
			} else {
				r = append(r, &msg{
					Role:    "assistant",
					Content: []*msgContent{content},
				})
				lastRole = "assistant"
			}
		case core.MsgTypeToolResult:
			toolResult, _ := m.AsToolResult()
			content := newMsgToolResult(toolResult.ID, toolResult.Result)

			if lastRole == "user" {
				lastMsg := r[len(r)-1]
				lastMsg.Content = append(lastMsg.Content, content)
			} else {
				r = append(r, &msg{
					Role:    "user",
					Content: []*msgContent{content},
				})
				lastRole = "user"
			}
		default:
			panic(fmt.Errorf("unknown message type: %d", m.Type))
		}
	}

	if m.shouldCache {
		// Only Reasoning blocks cannot be cached. However, we can safely assume that the last
		// content block in the history will /not/ be a reasoning block.
		lastMsg := r[len(r)-1]
		lastBlock := lastMsg.Content[len(lastMsg.Content)-1]
		if lastBlock.Type == msgContTypeReason {
			panic("assumption violated: last content block is a reasoning block")
		}

		lastBlock.CacheCtrl = &cacheCtrl{Type: "ephemeral"}
	}

	return sysPrompt, r
}

type tool struct {
	Name   string     `json:"name"`
	Desc   string     `json:"description"`
	Schema toolSchema `json:"input_schema"`
}

type toolSchema struct {
	Type       string `json:"type"` // always "object"
	Properties map[string]core.ToolParam
	Required   []string `json:"required"`
	Strict     bool     `json:"strict"`
}

func fromCoreTools(tools []core.Tool) []tool {
	r := make([]tool, 0, len(tools))
	for _, tool := range tools {
		r = append(r, fromCoreTool(tool))
	}
	return r
}

func fromCoreTool(x core.Tool) tool {
	r := tool{
		Name: x.Name,
		Desc: x.Desc,
		Schema: toolSchema{
			Type:       "object",
			Properties: make(map[string]core.ToolParam),
			Required:   make([]string, 0),
			Strict:     true,
		},
	}

	for paramName, param := range x.Params {
		r.Schema.Properties[paramName] = param
		r.Schema.Required = append(r.Schema.Required, paramName)
	}

	return r
}

type sse struct {
	Type sseType `json:"type"`

	Message struct {
		Model string `json:"model"`
	} `json:"message"`

	Usage usage `json:"usage"`

	ContentBlock struct {
		Type msgContentType `json:"type"`
		// tool use ID and Name are included here
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`

	Delta msgContent `json:"delta"`
}

type sseType string

const (
	// requires parsing .message.model (string), used to set response.Model
	stMsgStart sseType = "message_start"
	// requires parsing .usage (usage), used to update (overwrite) response.Usage
	stMsgDelta sseType = "message_delta"
	// does not require parsing, simply indicates the stopping point
	stMsgStop sseType = "message_stop"
	// requires parsing .content_block.type, of which we only need the "type" to append
	// a new message to the response.Messages slice with the appropriate type
	stContBlockStart sseType = "content_block_start"
	// requires parsing .delta (fits msgContent, but with different type /values/), based on the
	// type it will have a specific field that needs updating (e.g. thinking_delta, signature_delta,
	// text_delta, etc).
	stContBlockDelta sseType = "content_block_delta"
	// does not require parsing, a no-op unless we're doing buffering for deltas, which then would
	// mean we can "flush"
	stContBlockStop sseType = "content_block_stop"
	// no-op event from anthropic
	stPing sseType = "ping"
)

type usage struct {
	In           int64 `json:"input_tokens"`
	InCacheWrite int64 `json:"cache_creation_input_tokens"`
	InCacheRead  int64 `json:"cache_read_input_tokens"`
	Out          int64 `json:"output_tokens"`
}

func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}

const (
	messagesEndpoint    = "https://api.anthropic.com/v1/messages"
	anthropicApiVersion = "2023-06-01"
)
