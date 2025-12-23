package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/victhorio/opa/agg/core"
)

type Stream struct {
	stream  io.ReadCloser
	modelID ModelID
}

// OpenStream creates a new stream for the OpenAI API.
// It sends a request to the OpenAI responses endpoint and returns a ResponseStream that can be
// consumed for events.
func (m *Model) OpenStream(
	ctx context.Context,
	client *http.Client,
	messages []core.Message,
	tools []core.Tool,
) (core.ResponseStream, error) {
	payload := requestBody{
		Include: []string{"reasoning.encrypted_content"},
		Input:   fromCoreMessages(messages),
		Model:   m.model,
		Store:   boolPtr(false),
		Stream:  true,
		Tools:   fromCoreTools(tools),
	}

	if m.reasoningEffort != "" {
		payload.Reasoning = &reasoningCfg{
			Effort:  m.reasoningEffort,
			Summary: "concise",
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", responsesEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("OPENAI_API_KEY")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if err != nil {
			return nil, fmt.Errorf("openai responses api error: status=%s (failed to read body: %w)", resp.Status, err)
		}

		return nil, fmt.Errorf("openai responses api error: status=%s, body=%s", resp.Status, string(body))
	}

	return &Stream{
		stream:  resp.Body,
		modelID: m.model,
	}, nil
}

// Consume reads the stream of events from the OpenAI API and emits them to the output channel.
// It reads the stream, emitting events through the channel for:
// - completed tool calls
// - reasoning deltas (complete sentences)
// - output deltas (small chunks)
// - the final response object
//
// This function closes both the stream and the channel at the end of execution.
func (s *Stream) Consume(ctx context.Context, out chan<- core.Event) {
	defer s.stream.Close()
	defer close(out)

	// 10Kb buffer instead of 4Kb default since specially for etRespCompleted when we get a final
	// response of at least 2Kb even with nearly no output
	reader := bufio.NewReaderSize(s.stream, 10*1024)

	// although OpenAI currently only ever sends a full JSON in a single `data:` line per
	// event, let's be SSE spec compliant and nicely handle if they ever break it up into multiple
	// lines per event by accumulating data into this buffer
	var buf bytes.Buffer

	for {
		// in SSE newlines are field delimiters, we're looking for `data:` fields and will ignore
		// the rest
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}

			if !sendEvent(ctx, out, core.NewEvError(err)) {
				return
			}
			continue
		}

		line = strings.TrimRight(line, "\n\r")

		// in SSE empty lines means the event is over, so we can read the data we've collected
		// if we've collected it
		if line == "" {
			if buf.Len() == 0 {
				continue
			}

			dataBytes := buf.Bytes()
			buf.Reset()

			if shouldReturn := s.dispatchRawEvent(ctx, dataBytes, out); shouldReturn {
				return
			}
			continue
		}

		if dataBytes, ok := strings.CutPrefix(line, "data:"); ok {
			buf.WriteString(dataBytes)

			// for full SSE compliance we'd need to also add a newline, but since we're expecting
			// to unmarshall a JSON it shouldn't ever make a difference
		}

		// not a data field, we can ignore it
	}

	// nothing else to read, let's check if we need to handle a last event
	if buf.Len() > 0 {
		// can ignore the return indicator since we're finishing anyway
		_ = s.dispatchRawEvent(ctx, buf.Bytes(), out)
	}
}

func (s *Stream) dispatchRawEvent(ctx context.Context, dataBytes []byte, out chan<- core.Event) bool {
	var event eventRaw
	if err := json.Unmarshal(dataBytes, &event); err != nil {
		_ = sendEvent(ctx, out, core.NewEvError(err))
		return true
	}

	switch event.Type {
	case etRespCreated:
	case etRespProgress:
	case etRespCompleted:
		// convert openai API response object into ag.types.Response object and emit
		r := event.Response
		responsePub := core.Response{
			Model: r.Model,
			Usage: core.Usage{
				Input:     r.Usage.Input,
				Cached:    r.Usage.InputDetails.Cached,
				Output:    r.Usage.Output,
				Reasoning: r.Usage.OutputDetails.Reasoning,
				Total:     r.Usage.Input + r.Usage.Output,
				Cost:      costFromUsage(s.modelID, r.Usage),
			},
			Messages: toComMessages(r.Output),
		}
		if !sendEvent(ctx, out, core.NewEvResp(responsePub)) {
			return true
		}
		// stop listening for more events once we get response completed
		return true
	case etItemAdded:
	case etItemDone:
		if item := event.Item; item.Type == "function_call" {
			if !sendEvent(ctx, out, core.NewEvToolCall(core.ToolCall{
				ID:        item.CallID,
				Name:      item.Name,
				Arguments: item.Arguments,
			})) {
				return true
			}
		}
	case etContentAdded:
	case etContentDone:
	case etTextDelta:
		if !sendEvent(ctx, out, core.NewEvDelta(event.Delta)) {
			return true
		}
	case etTextDone:
	case etToolCallDelta:
	case etToolCallDone:
	case etReasoningAdded:
	case etReasoningDelta:
	case etReasoningDeltaDone:
		// we chose to emit reasoning deltas here
		// point being:
		// - individual deltas for reasoning don't seem nearly as important as individual deltas
		//   for response, and I prefer to have them rendered in their entirety
		// - further, etReasoningDeltaDone will always arrive before etReasoningDone, so no reason
		//   to wait for that one
		if !sendEvent(ctx, out, core.NewEvDeltaReason(event.Text)) {
			return true
		}
	case etReasoningDone:
	case etError:
		_ = sendEvent(ctx, out, core.NewEvError(fmt.Errorf("openai error: %s", string(dataBytes))))
		return true
	default:
		fmt.Printf("\033[31munknown event type:\033[0m %s\n\n%s\n\n", event.Type, string(dataBytes))
	}

	return false
}

func toComMessages(output []item) []core.Message {
	messages := make([]core.Message, 0, len(output))

	for _, item := range output {
		switch item.Type {
		case etfReasoning:
			messages = append(messages, core.NewMessageReasoning(item.EncryptedContent, ""))
		case etfMessage:
			if len(item.Content) != 1 {
				panic(fmt.Errorf("expected 1 content item, got %d", len(item.Content)))
			}

			messages = append(messages, core.NewMessageContent(item.Role, item.Content[0].Text))
		case etfFunctionCall:
			messages = append(messages, core.NewMessageToolCall(item.CallID, item.Name, item.Arguments))
		default:
			panic(fmt.Errorf("unknown item type: %s", item.Type))
		}
	}

	return messages
}

func sendEvent(ctx context.Context, out chan<- core.Event, ev core.Event) bool {
	select {
	case <-ctx.Done():
		return false
	case out <- ev:
		return true
	}
}

// requestBody is the body of the request to the OpenAI responses endpoint.
type requestBody struct {
	Include           []string      `json:"include,omitempty"`
	Input             []msg         `json:"input"`
	MaxOutputTokens   int           `json:"max_output_tokens,omitempty"`
	Model             ModelID       `json:"model,omitempty"`
	ParallelToolCalls *bool         `json:"parallel_tool_calls,omitempty"`
	Reasoning         *reasoningCfg `json:"reasoning,omitempty"`
	Store             *bool         `json:"store,omitempty"`
	Stream            bool          `json:"stream,omitempty"`
	Temperature       float64       `json:"temperature,omitempty"`
	Tools             []tool        `json:"tools,omitempty"`
}

type reasoningCfg struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// Message types for OpenAI API requests

type msgType string

const (
	msgTypeReasoning  msgType = "reasoning"
	msgTypeContent    msgType = "message"
	msgTypeToolCall   msgType = "function_call"
	msgTypeToolResult msgType = "function_call_output"
)

type msg struct {
	Type msgType `json:"type"`
	// reasoning fields
	Encrypted string    `json:"encrypted_content,omitempty"`
	Summary   *[]string `json:"summary,omitempty"`
	// content fields
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	// tool call fields
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	// tool result fields
	Output string `json:"output,omitempty"`
}

func newMsgReasoning(encrypted string) msg {
	return msg{
		Type:      msgTypeReasoning,
		Encrypted: encrypted,
		Summary:   &[]string{},
	}
}

func newMsgContent(role, content string) msg {
	return msg{
		Type:    msgTypeContent,
		Role:    role,
		Content: content,
	}
}

func newMsgToolCall(callID, name, arguments string) msg {
	return msg{
		Type:      msgTypeToolCall,
		CallID:    callID,
		Name:      name,
		Arguments: arguments,
	}
}

func newMsgToolResult(callID, result string) msg {
	return msg{
		Type:   msgTypeToolResult,
		CallID: callID,
		Output: result,
	}
}

func fromCoreMessages(messages []core.Message) []msg {
	adapted := make([]msg, 0, len(messages))
	for _, message := range messages {
		switch message.Type {
		case core.MTReasoning:
			reasoning, _ := message.Reasoning()
			adapted = append(adapted, newMsgReasoning(reasoning.Encrypted))
		case core.MTContent:
			content, _ := message.Content()
			adapted = append(adapted, newMsgContent(content.Role, content.Text))
		case core.MTToolCall:
			toolCall, _ := message.ToolCall()
			adapted = append(adapted, newMsgToolCall(toolCall.ID, toolCall.Name, toolCall.Arguments))
		case core.MTToolResult:
			toolResult, _ := message.ToolResult()
			adapted = append(adapted, newMsgToolResult(toolResult.ID, toolResult.Result))
		default:
			panic(fmt.Errorf("unknown message type: %d", message.Type))
		}
	}
	return adapted
}

// Tool types for OpenAI API requests

type tool struct {
	Type        string     `json:"type"` // always "function"
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  toolParams `json:"parameters"`
	Strict      bool       `json:"strict"`
}

type toolParams struct {
	Type                 core.JSType          `json:"type"` // always "object"
	Properties           map[string]paramProp `json:"properties,omitempty"`
	Required             []string             `json:"required,omitempty"`
	AdditionalProperties *bool                `json:"additionalProperties,omitempty"`
}

type paramProp struct {
	Type        core.JSType `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`

	// structural
	Items                *paramProp `json:"items,omitempty"`
	AdditionalProperties *bool      `json:"additionalProperties,omitempty"`

	// validation / constraints
	Enum     []string `json:"enum,omitempty"`
	Nullable *bool    `json:"nullable,omitempty"`
}

func fromCoreTools(tools []core.Tool) []tool {
	adapted := make([]tool, 0, len(tools))
	for _, tool := range tools {
		adapted = append(adapted, fromCoreTool(tool))
	}
	return adapted
}

func fromCoreTool(x core.Tool) tool {
	r := tool{
		Type:        "function",
		Name:        x.Name,
		Description: x.Desc,
		Parameters: toolParams{
			Type:                 "object",
			Properties:           make(map[string]paramProp),
			Required:             make([]string, 0),
			AdditionalProperties: boolPtr(false),
		},
		Strict: true,
	}

	for paramName, param := range x.Params {
		r.Parameters.Required = append(r.Parameters.Required, paramName)

		var items *paramProp
		if param.Items != nil {
			items = &paramProp{
				Type: param.Items.Type,
				Enum: param.Items.Enum,
			}
		}

		r.Parameters.Properties[paramName] = paramProp{
			Type:        param.Type,
			Description: param.Desc,
			Nullable:    param.Nullable,
			Items:       items,
			Enum:        param.Enum,
		}
	}

	return r
}

// Response and SSE types from OpenAI API

// eventRaw is the raw event from the OpenAI API.
type eventRaw struct {
	Type     string   `json:"type"`
	Response response `json:"response"`
	Item     item     `json:"item"`
	Delta    string   `json:"delta"`
	Text     string   `json:"text"`
}

// response represents the complete response
type response struct {
	Model  string `json:"model"`
	Output []item `json:"output"`
	Usage  usage  `json:"usage"`
}

// item represents a single item in the response.output array, this appears in the complete
// response but also can be received from etItemDone events
type item struct {
	Type             string    `json:"type"`
	Role             string    `json:"role"`
	EncryptedContent string    `json:"encrypted_content,omitempty"`
	Content          []content `json:"content,omitempty"`
	Arguments        string    `json:"arguments,omitempty"`
	CallID           string    `json:"call_id,omitempty"`
	Name             string    `json:"name,omitempty"`
}

type content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type usage struct {
	Input        int64 `json:"input_tokens"`
	InputDetails struct {
		Cached int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	Output        int64 `json:"output_tokens"`
	OutputDetails struct {
		Reasoning int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

// Event types from the OpenAI API
const (
	etRespCreated        = "response.created"
	etRespProgress       = "response.in_progress"
	etRespCompleted      = "response.completed"
	etItemAdded          = "response.output_item.added"
	etItemDone           = "response.output_item.done"
	etContentAdded       = "response.content_part.added"
	etContentDone        = "response.content_part.done"
	etTextDelta          = "response.output_text.delta"
	etTextDone           = "response.output_text.done"
	etToolCallDelta      = "response.function_call_arguments.delta"
	etToolCallDone       = "response.function_call_arguments.done"
	etReasoningAdded     = "response.reasoning_summary_part.added"
	etReasoningDelta     = "response.reasoning_summary_text.delta"
	etReasoningDeltaDone = "response.reasoning_summary_text.done"
	etReasoningDone      = "response.reasoning_summary_part.done"
	etError              = "error"
)

// Event type field values from the OpenAI API
const (
	etfReasoning    = "reasoning"
	etfMessage      = "message"
	etfFunctionCall = "function_call"
)

func boolPtr(b bool) *bool {
	return &b
}

const responsesEndpoint = "https://api.openai.com/v1/responses"
