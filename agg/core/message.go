package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type MessageType int

const (
	MTUninitialized MessageType = iota
	MTReasoning
	MTContent
	MTToolCall
	MTToolResult
)

type Message struct {
	Type       MessageType
	reasoning  *Reasoning
	content    *Content
	toolCall   *ToolCall
	toolResult *ToolResult
}

func NewMessageReasoning(encrypted, text string) Message {
	return Message{
		Type:      MTReasoning,
		reasoning: &Reasoning{Encrypted: encrypted, Text: text},
	}
}

func NewMessageContent(role, text string) Message {
	if role != "assistant" && role != "user" && role != "system" {
		panic(fmt.Errorf("invalid role: %s", role))
	}

	return Message{
		Type:    MTContent,
		content: &Content{Role: role, Text: text},
	}
}

func NewMessageToolCall(id, name, arguments string) Message {
	return Message{
		Type:     MTToolCall,
		toolCall: &ToolCall{ID: id, Name: name, Arguments: arguments},
	}
}

func NewMessageToolResult(id, result string) Message {
	return Message{
		Type:       MTToolResult,
		toolResult: &ToolResult{ID: id, Result: result},
	}
}

func (m *Message) Reasoning() (*Reasoning, bool) {
	if m.Type != MTReasoning {
		return nil, false
	}
	if m.reasoning == nil {
		panic("reasoning is nil, even though type is MTReasoning")
	}
	return m.reasoning, true
}

func (m *Message) Content() (*Content, bool) {
	if m.Type != MTContent {
		return nil, false
	}
	if m.content == nil {
		panic("content is nil, even though type is MTContent")
	}
	return m.content, true
}

func (m *Message) ToolCall() (*ToolCall, bool) {
	if m.Type != MTToolCall {
		return nil, false
	}
	if m.toolCall == nil {
		panic("tool call is nil, even though type is MTToolCall")
	}
	return m.toolCall, true
}

func (m *Message) ToolResult() (*ToolResult, bool) {
	if m.Type != MTToolResult {
		return nil, false
	}
	if m.toolResult == nil {
		panic("tool result is nil, even though type is MTToolResult")
	}
	return m.toolResult, true
}

func (m Message) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer

	b.WriteString(`{"type":`)
	fmt.Fprintf(&b, "%d", m.Type)
	b.WriteByte(',')

	switch m.Type {
	case MTReasoning:
		reasoning, err := json.Marshal(m.reasoning)
		if err != nil {
			return nil, err
		}
		b.WriteString(`"reasoning":`)
		b.Write(reasoning)
	case MTContent:
		content, err := json.Marshal(m.content)
		if err != nil {
			return nil, err
		}
		b.WriteString(`"content":`)
		b.Write(content)
	case MTToolCall:
		toolCall, err := json.Marshal(m.toolCall)
		if err != nil {
			return nil, err
		}
		b.WriteString(`"toolCall":`)
		b.Write(toolCall)
	case MTToolResult:
		toolResult, err := json.Marshal(m.toolResult)
		if err != nil {
			return nil, err
		}
		b.WriteString(`"toolResult":`)
		b.Write(toolResult)
	default:
		panic(fmt.Sprintf("unknown message type: %d", m.Type))
	}

	b.WriteString(`}`)
	return b.Bytes(), nil
}

type Reasoning struct {
	Encrypted string
	Text      string
}

type Content struct {
	Role string
	Text string
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type ToolResult struct {
	ID     string
	Result string
}
