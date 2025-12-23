package core

import (
	"fmt"
)

type MsgType int

const (
	MsgTypeUnknown MsgType = iota
	MsgTypeReasoning
	MsgTypeContent
	MsgTypeToolCall
	MsgTypeToolResult
)

type Msg struct {
	Type       MsgType     `json:"type"`
	Reasoning  *Reasoning  `json:"reasoning,omitempty"`
	Content    *Content    `json:"content,omitempty"`
	ToolCall   *ToolCall   `json:"toolCall,omitempty"`
	ToolResult *ToolResult `json:"toolResult,omitempty"`
}

func NewMsgReasoning(encrypted, text string) Msg {
	return Msg{
		Type:      MsgTypeReasoning,
		Reasoning: &Reasoning{Encrypted: encrypted, Text: text},
	}
}

func NewMsgContent(role, text string) Msg {
	if role != "assistant" && role != "user" && role != "system" {
		panic(fmt.Errorf("invalid role: %s", role))
	}

	return Msg{
		Type:    MsgTypeContent,
		Content: &Content{Role: role, Text: text},
	}
}

func NewMsgToolCall(id, name, arguments string) Msg {
	return Msg{
		Type:     MsgTypeToolCall,
		ToolCall: &ToolCall{ID: id, Name: name, Arguments: arguments},
	}
}

func NewMsgToolResult(id, result string) Msg {
	return Msg{
		Type:       MsgTypeToolResult,
		ToolResult: &ToolResult{ID: id, Result: result},
	}
}

func (m *Msg) AsReasoning() (*Reasoning, bool) {
	if m.Type != MsgTypeReasoning {
		return nil, false
	}
	if m.Reasoning == nil {
		panic("reasoning is nil, even though type is MTReasoning")
	}
	return m.Reasoning, true
}

func (m *Msg) AsContent() (*Content, bool) {
	if m.Type != MsgTypeContent {
		return nil, false
	}
	if m.Content == nil {
		panic("content is nil, even though type is MTContent")
	}
	return m.Content, true
}

func (m *Msg) AsToolCall() (*ToolCall, bool) {
	if m.Type != MsgTypeToolCall {
		return nil, false
	}
	if m.ToolCall == nil {
		panic("tool call is nil, even though type is MTToolCall")
	}
	return m.ToolCall, true
}

func (m *Msg) AsToolResult() (*ToolResult, bool) {
	if m.Type != MsgTypeToolResult {
		return nil, false
	}
	if m.ToolResult == nil {
		panic("tool result is nil, even though type is MTToolResult")
	}
	return m.ToolResult, true
}

type Reasoning struct {
	Encrypted string `json:"encrypted"`
	Text      string `json:"text"`
}

type Content struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolResult struct {
	ID     string `json:"id"`
	Result string `json:"result"`
}
