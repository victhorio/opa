package core

import (
	"encoding/json"
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

// Msg represents a single message in a conversation.
//
// IMPORTANT: Msg instances are NOT safe for concurrent access. A single Msg should never be
// shared across goroutines without external synchronization, as the CachedTransform field may
// be written to during message transformation.
//
// TODO(experiment): Evaluate the performance impact of adding synchronization (e.g., sync.RWMutex
// or atomic operations) to make Msg safe for concurrent use.
type Msg struct {
	Type       MsgType     `json:"type"`
	Reasoning  *Reasoning  `json:"reasoning,omitempty"`
	Content    *Content    `json:"content,omitempty"`
	ToolCall   *ToolCall   `json:"tool_call,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`

	// CachedTransform holds provider-specific transforms for a given Msg. This is done to avoid
	// re-transforming the same message multiple times throughout a conversation. Evidently this
	// has two implications:
	// 1. If the Msg is mutated for whatever reason, the CachedTransform needs to be manually
	//    invalidated.
	// 2. If the Msg is going to be used by a model from a different provider, the CachedTransform
	//    needs to be manually invalidated.
	CachedTransform json.RawMessage
}

func NewMsgReasoning(encrypted, text string) *Msg {
	return &Msg{
		Type:      MsgTypeReasoning,
		Reasoning: &Reasoning{Encrypted: encrypted, Text: text},
	}
}

func NewMsgContent(role, text string) *Msg {
	if role != "assistant" && role != "user" && role != "system" {
		panic(fmt.Errorf("invalid role: %s", role))
	}

	return &Msg{
		Type:    MsgTypeContent,
		Content: &Content{Role: role, Text: text},
	}
}

func NewMsgToolCall(id, name, arguments string) *Msg {
	return &Msg{
		Type:     MsgTypeToolCall,
		ToolCall: &ToolCall{ID: id, Name: name, Arguments: arguments},
	}
}

func NewMsgToolResult(id, result string) *Msg {
	return &Msg{
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

func (m *Msg) ResetCache() {
	m.CachedTransform = nil
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
