package openai

import (
	"fmt"
	"testing"

	"github.com/victhorio/opa/agg/core"
)

// generateConversationHistory creates a mock conversation history with the specified number of cycles.
// Each cycle follows the pattern:
// [user message, reasoning block, tool call, tool result, tool call, tool result, assistant message]
func generateConversationHistory(cycles int) []*core.Msg {
	msgs := make([]*core.Msg, 0, cycles*7)

	for i := range cycles {
		// User message
		msgs = append(msgs, core.NewMsgContent("user", fmt.Sprintf("User message %d", i)))

		// Reasoning block
		msgs = append(msgs, core.NewMsgReasoning(
			fmt.Sprintf("encrypted_reasoning_%d", i),
			fmt.Sprintf("This is reasoning text for turn %d", i),
		))

		// First tool call
		msgs = append(msgs, core.NewMsgToolCall(
			fmt.Sprintf("call_%d_1", i),
			"tool_alpha",
			fmt.Sprintf(`{"arg1": "value_%d_1", "arg2": %d}`, i, i*10),
		))

		// First tool result
		msgs = append(msgs, core.NewMsgToolResult(
			fmt.Sprintf("call_%d_1", i),
			fmt.Sprintf(`{"result": "data_%d_1"}`, i),
		))

		// Second tool call
		msgs = append(msgs, core.NewMsgToolCall(
			fmt.Sprintf("call_%d_2", i),
			"tool_beta",
			fmt.Sprintf(`{"param": "test_%d_2"}`, i),
		))

		// Second tool result
		msgs = append(msgs, core.NewMsgToolResult(
			fmt.Sprintf("call_%d_2", i),
			fmt.Sprintf(`{"status": "ok_%d_2"}`, i),
		))

		// Assistant message
		msgs = append(msgs, core.NewMsgContent("assistant", fmt.Sprintf("Assistant response %d", i)))
	}

	return msgs
}

func BenchmarkFromCoreMsgs_70Messages(b *testing.B) {
	msgs := generateConversationHistory(10) // 70 messages
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = fromCoreMsgs(msgs)
	}
}

func BenchmarkFromCoreMsgs_350Messages(b *testing.B) {
	msgs := generateConversationHistory(50) // 350 messages
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = fromCoreMsgs(msgs)
	}
}

func BenchmarkFromCoreMsgs_1050Messages(b *testing.B) {
	msgs := generateConversationHistory(150) // 1050 messages
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = fromCoreMsgs(msgs)
	}
}

func BenchmarkFromCoreMsgs_2100Messages(b *testing.B) {
	msgs := generateConversationHistory(300) // 2100 messages
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = fromCoreMsgs(msgs)
	}
}
