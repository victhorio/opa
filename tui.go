package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/victhorio/opa/agg"
	"github.com/victhorio/opa/agg/core"
)

const (
	minInputHeight = 2
	maxInputHeight = 6
	footerHeight   = 2 // divider + hint
)

var (
	labelUserStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	labelBotStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("34"))
	labelToolStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	labelReasonStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244"))
	bodyToolStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("178"))
	bodyReasonStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	hintStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	dividerStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	assistantBodyStyle = lipgloss.NewStyle()
)

type msgKind int

const (
	msgUser msgKind = iota
	msgAssistant
	msgTool
	msgReasoning
)

type chatMessage struct {
	kind msgKind
	text string
}

// Bubble Tea messages for streaming events. These are sent from the goroutine in startStream
// to the main Update loop via the streamCh channel.
type botDeltaMsg struct{ text string }   // incremental text from the assistant
type botDoneMsg struct{ text string }    // final complete response
type botErrorMsg struct{ err error }     // error during streaming
type streamClosedMsg struct{}            // channel was closed
type toolCallMsg struct{ text string }   // tool call (complete, not streamed)
type reasoningMsg struct{ text string }  // reasoning block (complete, not streamed)

// TUIModel is the Bubble Tea model for the chat interface. It manages both the UI state
// (viewport, textarea, dimensions) and the streaming state (channel, cancel func).
//
// Streaming architecture: when the user submits input, startStream spawns a goroutine that
// calls agent.RunStream and forwards events to streamCh. The Update loop consumes from this
// channel via waitForStream, which returns a tea.Cmd that blocks until the next event.
type TUIModel struct {
	agent     agg.Agent
	client    *http.Client
	sessionID string

	modelUserInput   textarea.Model
	modelChatHistory viewport.Model

	// messages holds the complete chat history (finalized messages only).
	// partialResponse accumulates streaming deltas until botDoneMsg finalizes them.
	messages        []chatMessage
	partialResponse string
	generating      bool
	errMsg          string

	// streamCh receives events from the streaming goroutine. cancelCurrStream cancels the
	// context passed to that goroutine, which will cause it to stop and close the channel.
	streamCh         <-chan tea.Msg
	cancelCurrStream context.CancelFunc

	// stickToBottom controls auto-scroll behavior. When true, the viewport scrolls to bottom
	// on every update. Set to false when user manually scrolls up.
	stickToBottom bool

	// renderedHistory caches the wrapped, rendered finalized messages. Rebuilt only when
	// messages are added (cachedMsgCount changes) or viewport width changes (cachedWidth).
	renderedHistory string
	cachedMsgCount  int
	cachedWidth     int

	width  int
	height int
}

func newTUIModel(agent agg.Agent, sessionID string) TUIModel {
	ta := textarea.New()
	ta.Placeholder = "Ask opa..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetHeight(minInputHeight)
	ta.SetWidth(0)
	ta.ShowLineNumbers = false
	ta.Prompt = ""

	vp := viewport.New(0, 0)

	return TUIModel{
		agent:            agent,
		client:           http.DefaultClient,
		sessionID:        sessionID,
		modelUserInput:   ta,
		modelChatHistory: vp,
		messages:         []chatMessage{},
		stickToBottom:    true,
	}
}

func runTUI(agent agg.Agent, sessionID string) error {
	p := tea.NewProgram(newTUIModel(agent, sessionID), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m TUIModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncSizes()
		m.updateViewport()
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	case botDeltaMsg:
		m.partialResponse += msg.text
		m.updateViewport()
		return m, m.waitForStream()
	case botDoneMsg:
		m.generating = false
		m.errMsg = ""
		m.messages = append(m.messages, chatMessage{kind: msgAssistant, text: msg.text})
		m.partialResponse = ""
		m.stopStream()
		m.updateViewport()
		return m, nil
	case botErrorMsg:
		m.generating = false
		m.errMsg = msg.err.Error()
		m.partialResponse = ""
		m.stopStream()
		m.updateViewport()
		return m, nil
	case streamClosedMsg:
		m.stopStream()
		return m, nil
	case toolCallMsg:
		m.messages = append(m.messages, chatMessage{kind: msgTool, text: msg.text})
		m.updateViewport()
		return m, m.waitForStream()
	case reasoningMsg:
		m.messages = append(m.messages, chatMessage{kind: msgReasoning, text: msg.text})
		m.updateViewport()
		return m, m.waitForStream()
	}

	// Handle non-KeyMsg messages (mouse, focus, etc.) that the textarea might want.
	// KeyMsg is handled separately in updateKey which has its own fallthrough for unhandled keys.
	//
	// TODO(refactor): updateViewport is called in every branch above and also here. Consider
	//                 restructuring so it's called once at the end of Update regardless of path.
	var cmd tea.Cmd
	m.modelUserInput, cmd = m.modelUserInput.Update(msg)
	m.syncInputHeight()
	m.updateViewport()
	return m, cmd
}

func (m TUIModel) View() string {
	var b strings.Builder

	b.WriteString(m.modelChatHistory.View())
	b.WriteString("\n")
	b.WriteString(renderDivider(m.width))
	b.WriteString("\n")
	b.WriteString(m.modelUserInput.View())
	b.WriteString("\n")

	hint := "Enter to send • Alt+Enter for newline • :q to quit • Ctrl+C"
	if m.generating {
		hint = "Assistant is responding..."
	}
	if m.errMsg != "" {
		hint = errorStyle.Render(fmt.Sprintf("Error: %s", m.errMsg))
	}
	b.WriteString(hintStyle.Render(hint))

	return b.String()
}

func (m TUIModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.stopStream()
		return m, tea.Quit
	case tea.KeyPgUp:
		_ = m.modelChatHistory.PageUp()
		m.updateStickiness()
		return m, nil
	case tea.KeyPgDown:
		_ = m.modelChatHistory.PageDown()
		m.updateStickiness()
		return m, nil
	case tea.KeyShiftUp:
		_ = m.modelChatHistory.ScrollUp(1)
		m.updateStickiness()
		return m, nil
	case tea.KeyShiftDown:
		_ = m.modelChatHistory.ScrollDown(1)
		m.updateStickiness()
		return m, nil
	case tea.KeyEnter:
		if msg.Alt {
			m.modelUserInput.InsertString("\n")
			m.syncInputHeight()
			m.updateViewport()
			return m, nil
		}
		return m.submitInput()
	}

	var cmd tea.Cmd
	m.modelUserInput, cmd = m.modelUserInput.Update(msg)
	m.syncInputHeight()
	m.updateViewport()
	return m, cmd
}

func (m TUIModel) submitInput() (tea.Model, tea.Cmd) {
	if m.generating {
		// We're in the middle of a generation. For now, just ignore and make it a no op.
		return m, nil
	}

	input := strings.TrimSpace(m.modelUserInput.Value())
	if input == "" {
		return m, nil
	}

	if input == ":q" || input == "quit" || input == "exit" {
		m.stopStream()
		return m, tea.Quit
	}

	m.messages = append(m.messages, chatMessage{kind: msgUser, text: input})
	m.modelUserInput.Reset()
	m.partialResponse = ""
	m.generating = true
	m.errMsg = ""
	m.syncInputHeight()
	m.updateViewport()

	// Set up streaming state explicitly here rather than inside the spawning function.
	// This makes the data flow clearer: submitInput owns all state changes.
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelCurrStream = cancel
	m.streamCh = m.runStreamingRequest(ctx, input)
	m.stickToBottom = true

	return m, m.waitForStream()
}

// runStreamingRequest spawns a goroutine that calls agent.RunStream and translates core.Event
// into tea.Msg, sending them through the returned channel. Does not modify TUIModel state.
// The goroutine exits when ctx is cancelled or the stream completes.
func (m TUIModel) runStreamingRequest(ctx context.Context, input string) <-chan tea.Msg {
	// Buffered to avoid blocking HTTP events during TUI rendering hiccups.
	events := make(chan tea.Msg, 16)
	go func() {
		defer close(events)

		sendEvent := func(msg tea.Msg) {
			select {
			case events <- msg:
			case <-ctx.Done():
			}
		}

		_, err := m.agent.RunStream(ctx, m.client, m.sessionID, input, false, func(ev core.Event) {
			switch ev.Type {
			case core.EvDelta:
				sendEvent(botDeltaMsg{text: ev.Delta})
			case core.EvResp:
				if len(ev.Response.Messages) == 0 {
					return
				}
				last := ev.Response.Messages[len(ev.Response.Messages)-1]
				if content, ok := last.AsContent(); ok {
					sendEvent(botDoneMsg{text: content.Text})
				}
			case core.EvToolCall:
				call := fmt.Sprintf("%s (%s): %s", ev.Call.Name, ev.Call.ID, ev.Call.Arguments)
				call = maybeTruncate(call, 300)
				sendEvent(toolCallMsg{text: call})
			case core.EvDeltaReason:
				sendEvent(reasoningMsg{text: ev.Delta})
			case core.EvError:
				sendEvent(botErrorMsg{err: ev.Err})
			}
		})
		if err != nil {
			sendEvent(botErrorMsg{err: err})
			return
		}
	}()

	return events
}

// waitForStream returns a tea.Cmd that blocks until the next message arrives on streamCh.
// This is how Bubble Tea integrates with our async streaming: the cmd blocks in a separate
// goroutine, and when a message arrives, Bubble Tea delivers it to Update.
func (m TUIModel) waitForStream() tea.Cmd {
	if m.streamCh == nil {
		return nil
	}

	return func() tea.Msg {
		msg, ok := <-m.streamCh
		if !ok {
			return streamClosedMsg{}
		}
		return msg
	}
}

func (m *TUIModel) stopStream() {
	if m.cancelCurrStream != nil {
		m.cancelCurrStream()
		m.cancelCurrStream = nil
	}
	m.streamCh = nil
}

// syncSizes updates all component widths and delegates height calculation to syncInputHeight.
// Called on window resize.
func (m *TUIModel) syncSizes() {
	m.modelUserInput.SetWidth(m.width)
	m.modelChatHistory.Width = m.width
	m.syncInputHeight()
}

// syncInputHeight adjusts the textarea height based on content (clamped to min/max), then
// recalculates the viewport height to fill the remaining space. Called whenever the textarea
// content might have changed (typing, submit, resize).
func (m *TUIModel) syncInputHeight() {
	height := clamp(m.modelUserInput.LineCount(), minInputHeight, maxInputHeight)
	m.modelUserInput.SetHeight(height)

	if m.width > 0 && m.height > 0 {
		chatHeight := max(m.height-footerHeight-height, 3)
		m.modelChatHistory.Height = chatHeight
	}
}

// updateViewport composes the chat history content and updates the viewport. Uses cached
// rendered history for finalized messages, only rebuilding when messages or width change.
// During streaming, only the partial response is re-rendered per delta.
func (m *TUIModel) updateViewport() {
	cacheValid := len(m.messages) == m.cachedMsgCount &&
		m.modelChatHistory.Width == m.cachedWidth

	if !cacheValid {
		m.rebuildHistoryCache()
	}

	// Compose: cached history + live partial (only partial needs work per delta)
	content := m.renderedHistory
	if m.partialResponse != "" {
		partial := fmt.Sprintf("%s: %s",
			labelBotStyle.Render("Assistant"),
			assistantBodyStyle.Render(m.partialResponse))
		if m.cachedWidth > 0 {
			partial = wrapContent(partial, m.cachedWidth)
		}
		if content != "" {
			content += "\n\n" + partial
		} else {
			content = partial
		}
	}

	m.modelChatHistory.SetContent(content)
	if m.stickToBottom {
		m.modelChatHistory.GotoBottom()
	}
}

// rebuildHistoryCache renders all finalized messages with wrapping and stores the result.
func (m *TUIModel) rebuildHistoryCache() {
	var b strings.Builder
	for _, msg := range m.messages {
		b.WriteString(renderMessage(msg))
		b.WriteString("\n\n")
	}
	content := strings.TrimRight(b.String(), "\n")
	if m.modelChatHistory.Width > 0 {
		content = wrapContent(content, m.modelChatHistory.Width)
	}
	m.renderedHistory = content
	m.cachedMsgCount = len(m.messages)
	m.cachedWidth = m.modelChatHistory.Width
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func renderDivider(width int) string {
	w := max(width, 10)
	return dividerStyle.Render(strings.Repeat("─", w))
}

// renderMessage renders a single chat message with appropriate styling.
func renderMessage(msg chatMessage) string {
	var label, body string
	switch msg.kind {
	case msgUser:
		label = labelUserStyle.Render("You")
		body = msg.text
	case msgAssistant:
		label = labelBotStyle.Render("Assistant")
		body = assistantBodyStyle.Render(msg.text)
	case msgTool:
		label = labelToolStyle.Render("Tool")
		body = bodyToolStyle.Render(msg.text)
	case msgReasoning:
		label = labelReasonStyle.Render("Reasoning")
		body = bodyReasonStyle.Render(msg.text)
	}
	return fmt.Sprintf("%s: %s", label, body)
}

func maybeTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// updateStickiness checks if user has scrolled away from bottom. If so, disables auto-scroll
// so new content doesn't jump the viewport. Called after manual scroll actions.
func (m *TUIModel) updateStickiness() {
	m.stickToBottom = m.modelChatHistory.AtBottom()
}

func wrapContent(s string, width int) string {
	if width <= 0 {
		return s
	}
	return wordwrap.String(s, width)
}
