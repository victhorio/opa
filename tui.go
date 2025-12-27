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
	headerHeight   = 1
	footerHeight   = 2
)

var (
	titleStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
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

type botDeltaMsg struct{ text string }
type botDoneMsg struct{ text string }
type botErrorMsg struct{ err error }
type streamClosedMsg struct{}
type toolCallMsg struct {
	text string
}
type reasoningMsg struct {
	text string
}

type model struct {
	agent     agg.Agent
	client    *http.Client
	sessionID string

	textarea textarea.Model
	viewport viewport.Model

	messages   []chatMessage
	streaming  string
	generating bool
	errMsg     string

	streamCh <-chan tea.Msg
	cancel   context.CancelFunc

	stickToBottom bool

	width  int
	height int
}

func newModel(agent agg.Agent, sessionID string) model {
	ta := textarea.New()
	ta.Placeholder = "Ask opa..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetHeight(minInputHeight)
	ta.SetWidth(0)
	ta.ShowLineNumbers = false
	ta.Prompt = ""

	vp := viewport.New(0, 0)

	return model{
		agent:         agent,
		client:        http.DefaultClient,
		sessionID:     sessionID,
		textarea:      ta,
		viewport:      vp,
		messages:      []chatMessage{},
		stickToBottom: true,
	}
}

func runTUI(agent agg.Agent, sessionID string) error {
	p := tea.NewProgram(newModel(agent, sessionID), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.streaming += msg.text
		m.updateViewport()
		return m, m.waitForStream()
	case botDoneMsg:
		m.generating = false
		m.errMsg = ""
		m.messages = append(m.messages, chatMessage{kind: msgAssistant, text: msg.text})
		m.streaming = ""
		m.stopStream()
		m.updateViewport()
		return m, nil
	case botErrorMsg:
		m.generating = false
		m.errMsg = msg.err.Error()
		m.streaming = ""
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

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.syncInputHeight()
	m.updateViewport()
	return m, cmd
}

func (m model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("opa • chat"))
	b.WriteString("\n\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	b.WriteString(renderDivider(m.width))
	b.WriteString("\n")
	b.WriteString(m.textarea.View())
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

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.stopStream()
		return m, tea.Quit
	case tea.KeyPgUp:
		m.viewport.PageUp()
		m.updateStickiness()
		return m, nil
	case tea.KeyPgDown:
		m.viewport.PageDown()
		m.updateStickiness()
		return m, nil
	case tea.KeyCtrlU:
		m.viewport.HalfPageUp()
		m.updateStickiness()
		return m, nil
	case tea.KeyCtrlD:
		m.viewport.HalfPageDown()
		m.updateStickiness()
		return m, nil
	case tea.KeyShiftUp:
		m.viewport.LineUp(1)
		m.updateStickiness()
		return m, nil
	case tea.KeyShiftDown:
		m.viewport.LineDown(1)
		m.updateStickiness()
		return m, nil
	case tea.KeyEnter:
		if msg.Alt {
			m.textarea.InsertString("\n")
			m.syncInputHeight()
			m.updateViewport()
			return m, nil
		}
		return m.submitInput()
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.syncInputHeight()
	m.updateViewport()
	return m, cmd
}

func (m model) submitInput() (tea.Model, tea.Cmd) {
	if m.generating {
		return m, nil
	}

	input := strings.TrimSpace(m.textarea.Value())
	if input == "" {
		return m, nil
	}

	if input == ":q" {
		m.stopStream()
		return m, tea.Quit
	}

	m.messages = append(m.messages, chatMessage{kind: msgUser, text: input})
	m.textarea.Reset()
	m.streaming = ""
	m.generating = true
	m.errMsg = ""
	m.syncInputHeight()
	m.updateViewport()

	return m, m.startStream(input)
}

func (m *model) startStream(input string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	events := make(chan tea.Msg)
	go func() {
		defer close(events)

		var assembled strings.Builder
		var final string

		_, err := m.agent.RunStream(ctx, m.client, m.sessionID, input, false, func(ev core.Event) {
			switch ev.Type {
			case core.EvDelta:
				assembled.WriteString(ev.Delta)
				select {
				case events <- botDeltaMsg{text: ev.Delta}:
				case <-ctx.Done():
				}
			case core.EvResp:
				if len(ev.Response.Messages) == 0 {
					return
				}
				last := ev.Response.Messages[len(ev.Response.Messages)-1]
				if content, ok := last.AsContent(); ok {
					final = content.Text
				}
			case core.EvToolCall:
				call := fmt.Sprintf("%s (%s): %s", ev.Call.Name, ev.Call.ID, ev.Call.Arguments)
				call = maybeTruncate(call, 300)
				select {
				case events <- toolCallMsg{text: call}:
				case <-ctx.Done():
				}
			case core.EvDeltaReason:
				select {
				case events <- reasoningMsg{text: ev.Delta}:
				case <-ctx.Done():
				}
			case core.EvError:
				select {
				case events <- botErrorMsg{err: ev.Err}:
				case <-ctx.Done():
				}
			}
		})
		if err != nil {
			select {
			case events <- botErrorMsg{err: err}:
			case <-ctx.Done():
			}
			return
		}

		if final == "" {
			final = assembled.String()
		}

		select {
		case events <- botDoneMsg{text: final}:
		case <-ctx.Done():
		}
	}()

	m.streamCh = events
	m.stickToBottom = true
	return m.waitForStream()
}

func (m model) waitForStream() tea.Cmd {
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

func (m *model) stopStream() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.streamCh = nil
}

func (m *model) syncSizes() {
	m.textarea.SetWidth(m.width)
	m.syncInputHeight()

	chatHeight := m.height - headerHeight - footerHeight - m.textarea.Height()
	if chatHeight < 3 {
		chatHeight = 3
	}
	m.viewport.Width = m.width
	m.viewport.Height = chatHeight
}

func (m *model) syncInputHeight() {
	height := clamp(m.textarea.LineCount(), minInputHeight, maxInputHeight)
	m.textarea.SetHeight(height)

	if m.width > 0 && m.height > 0 {
		chatHeight := m.height - headerHeight - footerHeight - height
		if chatHeight < 3 {
			chatHeight = 3
		}
		m.viewport.Height = chatHeight
		m.viewport.Width = m.width
	}
}

func (m *model) updateViewport() {
	var b strings.Builder

	for _, msg := range m.messages {
		var label string
		body := msg.text
		switch msg.kind {
		case msgUser:
			label = labelUserStyle.Render("You")
		case msgAssistant:
			label = labelBotStyle.Render("Assistant")
		case msgTool:
			label = labelToolStyle.Render("Tool")
			body = bodyToolStyle.Render(body)
		case msgReasoning:
			label = labelReasonStyle.Render("Reasoning")
			body = bodyReasonStyle.Render(body)
		}
		fmt.Fprintf(&b, "%s: %s\n\n", label, body)
	}

	if m.streaming != "" {
		fmt.Fprintf(&b, "%s: %s", labelBotStyle.Render("Assistant"), assistantBodyStyle.Render(m.streaming))
	}

	content := strings.TrimRight(b.String(), "\n")
	if m.viewport.Width > 0 {
		content = wrapContent(content, m.viewport.Width)
	}
	m.viewport.SetContent(content)
	if m.stickToBottom {
		m.viewport.GotoBottom()
	}
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
	w := width
	if w < 10 {
		w = 10
	}
	return dividerStyle.Render(strings.Repeat("─", w))
}

func maybeTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func (m *model) updateStickiness() {
	m.stickToBottom = m.viewport.AtBottom()
}

func wrapContent(s string, width int) string {
	if width <= 0 {
		return s
	}
	return wordwrap.String(s, width)
}
