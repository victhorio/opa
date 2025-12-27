package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/victhorio/opa/agg"
)

func testModel() TUIModel {
	m := newTUIModel(agg.Agent{}, "test", nil) // nil = embeddings already ready
	m.width, m.height = 80, 24
	m.syncSizes()
	return m
}

func TestCacheInvalidation(t *testing.T) {
	m := testModel()

	// Initially cache is empty (zero values)
	if m.cachedMsgCount != 0 || m.cachedWidth != 0 {
		t.Error("cache should start empty")
	}

	// First updateViewport should build cache
	m.updateViewport()
	if m.cachedMsgCount != 0 || m.cachedWidth != m.modelChatHistory.Width {
		t.Errorf("cache should have built: got msgCount=%d width=%d, want msgCount=0 width=%d",
			m.cachedMsgCount, m.cachedWidth, m.modelChatHistory.Width)
	}

	// Add a message, verify cache rebuilds
	m.messages = append(m.messages, chatMessage{kind: msgUser, text: "hello"})
	m.updateViewport()

	if m.cachedMsgCount != 1 {
		t.Errorf("cache should have rebuilt after adding message: got msgCount=%d, want 1", m.cachedMsgCount)
	}
	if m.renderedHistory == "" {
		t.Error("rendered history should not be empty after adding message")
	}

	// Call again without changes, cache should remain valid (no rebuild)
	prevRendered := m.renderedHistory
	prevWidth := m.cachedWidth
	m.updateViewport()

	if m.renderedHistory != prevRendered {
		t.Error("cache should not have rebuilt when nothing changed")
	}
	if m.cachedWidth != prevWidth {
		t.Error("cached width should not have changed")
	}

	// Change viewport width, verify cache rebuilds
	oldWidth := m.cachedWidth
	m.modelChatHistory.Width = 100
	m.updateViewport()

	if m.cachedWidth != 100 {
		t.Errorf("cache should have rebuilt on width change: got %d, want 100", m.cachedWidth)
	}
	if m.cachedWidth == oldWidth {
		t.Error("cached width should have changed")
	}
}

func TestCacheWithPartialResponse(t *testing.T) {
	m := testModel()

	// Add a finalized message
	m.messages = append(m.messages, chatMessage{kind: msgUser, text: "hello"})
	m.updateViewport()

	cachedAfterMessage := m.renderedHistory
	msgCountAfterMessage := m.cachedMsgCount

	// Simulate streaming: add partial response
	m.partialResponse = "I'm thinking..."
	m.updateViewport()

	// Cache should NOT rebuild just because partial changed
	if m.cachedMsgCount != msgCountAfterMessage {
		t.Error("cache should not rebuild when only partial response changes")
	}
	if m.renderedHistory != cachedAfterMessage {
		t.Error("rendered history cache should remain unchanged during streaming")
	}

	// But the viewport content should include the partial
	content := m.modelChatHistory.View()
	if content == "" {
		t.Error("viewport should have content")
	}
}

func TestSubmitInputEmpty(t *testing.T) {
	m := testModel()

	// Empty input should be a no-op
	m.modelUserInput.SetValue("")
	model, cmd := m.submitInput()

	result := model.(TUIModel)
	if len(result.messages) != 0 {
		t.Error("empty input should not add a message")
	}
	if cmd != nil {
		t.Error("empty input should return nil cmd")
	}
}

func TestSubmitInputWhitespaceOnly(t *testing.T) {
	m := testModel()

	// Whitespace-only input should be a no-op
	m.modelUserInput.SetValue("   \n\t  ")
	model, cmd := m.submitInput()

	result := model.(TUIModel)
	if len(result.messages) != 0 {
		t.Error("whitespace-only input should not add a message")
	}
	if cmd != nil {
		t.Error("whitespace-only input should return nil cmd")
	}
}

func TestSubmitInputQuitCommands(t *testing.T) {
	quitCommands := []string{":q", "quit", "exit"}

	for _, quitCmd := range quitCommands {
		t.Run(quitCmd, func(t *testing.T) {
			m := testModel()

			m.modelUserInput.SetValue(quitCmd)
			_, cmd := m.submitInput()

			// Should return tea.Quit
			if cmd == nil {
				t.Errorf("%q should return a quit command", quitCmd)
				return
			}

			// Execute the cmd to verify it's a quit
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); !ok {
				t.Errorf("%q should produce tea.QuitMsg, got %T", quitCmd, msg)
			}
		})
	}
}

func TestSubmitInputWhileGenerating(t *testing.T) {
	m := testModel()
	m.generating = true

	m.modelUserInput.SetValue("this should be ignored")
	model, cmd := m.submitInput()

	result := model.(TUIModel)
	if len(result.messages) != 0 {
		t.Error("input while generating should not add a message")
	}
	if cmd != nil {
		t.Error("input while generating should return nil cmd")
	}
}

func TestRenderMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      chatMessage
		contains string // we check contains rather than exact match to avoid style brittleness
	}{
		{"user message", chatMessage{kind: msgUser, text: "hello"}, "hello"},
		{"assistant message", chatMessage{kind: msgAssistant, text: "hi there"}, "hi there"},
		{"tool message", chatMessage{kind: msgTool, text: "tool output"}, "tool output"},
		{"reasoning message", chatMessage{kind: msgReasoning, text: "thinking..."}, "thinking..."},
	}

	m := testModel()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.renderMessage(tt.msg)
			if result == "" {
				t.Error("renderMessage should not return empty string")
			}
			// Just verify the text content is present; don't assert on styling
			if !strings.Contains(result, tt.contains) {
				t.Errorf("renderMessage result should contain %q, got %q", tt.contains, result)
			}
		})
	}
}
