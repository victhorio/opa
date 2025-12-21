package openai

import (
	"fmt"

	"github.com/victhorio/opa/agg/com"
)

type msgReasoning struct {
	Type      msgType  `json:"type"`
	Encrypted string   `json:"encrypted_content"`
	Summary   []string `json:"summary"`
}

type msgContent struct {
	Type    msgType `json:"type"`
	Role    string  `json:"role"`
	Content string  `json:"content"`
}

type msgToolCall struct {
	Type      msgType `json:"type"`
	CallID    string  `json:"call_id"`
	Name      string  `json:"name"`
	Arguments string  `json:"arguments"`
}

type msgToolResult struct {
	Type   msgType `json:"type"`
	CallID string  `json:"call_id"`
	Output string  `json:"output"`
}

type msgType string

const (
	msgTypeReasoning  msgType = "reasoning"
	msgTypeContent    msgType = "message"
	msgTypeToolCall   msgType = "function_call"
	msgTypeToolResult msgType = "function_call_output"
)

func newMsgReasoning(encrypted string) msgReasoning {
	return msgReasoning{
		Type:      msgTypeReasoning,
		Encrypted: encrypted,
		Summary:   []string{},
	}
}

func newMsgContent(role, content string) msgContent {
	return msgContent{
		Type:    msgTypeContent,
		Role:    role,
		Content: content,
	}
}

func newMsgToolCall(callID, name, arguments string) msgToolCall {
	return msgToolCall{
		Type:      msgTypeToolCall,
		CallID:    callID,
		Name:      name,
		Arguments: arguments,
	}
}

func newMsgToolResult(callID, result string) msgToolResult {
	return msgToolResult{
		Type:   msgTypeToolResult,
		CallID: callID,
		Output: result,
	}
}

func fromComMessages(messages []com.Message) []any {
	adapted := make([]any, 0, len(messages))
	for _, message := range messages {
		switch message.Type {
		case com.MTReasoning:
			reasoning, _ := message.Reasoning()
			adapted = append(
				adapted,
				newMsgReasoning(reasoning.Encrypted),
			)
		case com.MTContent:
			content, _ := message.Content()
			adapted = append(
				adapted,
				newMsgContent(content.Role, content.Text),
			)
		case com.MTToolCall:
			toolCall, _ := message.ToolCall()
			adapted = append(
				adapted,
				newMsgToolCall(toolCall.ID, toolCall.Name, toolCall.Arguments),
			)
		case com.MTToolResult:
			toolResult, _ := message.ToolResult()
			adapted = append(
				adapted,
				newMsgToolResult(toolResult.ID, toolResult.Result),
			)
		default:
			panic(fmt.Errorf("unknown message type: %d", message.Type))
		}
	}
	return adapted
}
