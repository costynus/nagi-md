package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
	lipgloss "charm.land/lipgloss/v2"
)

type model struct {
	editor  textarea.Model
	width   int
	height  int
	preview viewport.Model

	wheelDirection tea.MouseButton
	wheelBlocked   bool
}

func (m model) Init() tea.Cmd {
	return cursor.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		m.wheelBlocked = false

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
	case tea.MouseWheelMsg:
		if msg.Button != tea.MouseWheelDown && msg.Button != tea.MouseWheelUp {
			return m, nil
		}

		if msg.Button != m.wheelDirection {
			m.wheelDirection = msg.Button
			m.wheelBlocked = false
		}

		if m.wheelBlocked {
			return m, nil
		}

		previousLine := m.editor.Line()
		previousColumn := m.editor.Column()
		previousOffset := m.editor.ScrollYOffset()

		switch msg.Button {
		case tea.MouseWheelDown:
			m.editor.CursorDown()
		case tea.MouseWheelUp:
			m.editor.CursorUp()
		default:
			return m, nil
		}

		currentOffset := m.editor.ScrollYOffset()
		cursorDidNotMove := m.editor.Line() == previousLine && m.editor.Column() == previousColumn

		if cursorDidNotMove {
			m.wheelBlocked = true
			if msg.Button == tea.MouseWheelDown {
				m.preview.GotoBottom()
			} else {
				m.preview.GotoTop()
			}
			return m, nil
		}

		if currentOffset != previousOffset {
			m.syncPreviewScroll()
		}

		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		leftWidth := m.width / 2
		rightWidth := m.width - leftWidth

		m.editor.SetWidth(leftWidth)
		m.editor.SetHeight(m.height)

		m.preview.SetWidth(rightWidth)
		m.preview.SetHeight(m.height)

		previewWidth := max(1, rightWidth-m.preview.Style.GetHorizontalFrameSize())
		rendered, err := renderMarkdown(m.editor.Value(), previewWidth)
		if err != nil {
			rendered = fmt.Sprintf("Error rendering Markdown: %v", err)
		}
		m.preview.SetContent(rendered)
	}

	mayChangeContent := mayChangeEditorContent(msg, m.editor.KeyMap)
	var previousContent string
	if mayChangeContent {
		previousContent = m.editor.Value()
	}

	editorWasAtEnd := false
	if mayChangeContent {
		lastLineStart := strings.LastIndexByte(previousContent, '\n') + 1
		lastLine := previousContent[lastLineStart:]

		editorWasAtEnd = m.editor.Line() == m.editor.LineCount()-1 &&
			m.editor.Column() == utf8.RuneCountInString(lastLine)
	}
	previousScrollOffset := m.editor.ScrollYOffset()

	updatedEditor, editorCmd := m.editor.Update(msg)
	m.editor = updatedEditor

	var currentContent string
	contentChanged := false

	if mayChangeContent {
		currentContent = m.editor.Value()
		contentChanged = currentContent != previousContent
	}

	scrollChanged := m.editor.ScrollYOffset() != previousScrollOffset

	if contentChanged && m.width > 0 {
		rightWidth := m.width - m.width/2
		previewWidth := max(1, rightWidth-m.preview.Style.GetHorizontalFrameSize())

		rendered, err := renderMarkdown(currentContent, previewWidth)
		if err != nil {
			rendered = fmt.Sprintf("Error rendering Markdown: %v", err)
		}
		m.preview.SetContent(rendered)
	}
	if contentChanged && editorWasAtEnd {
		m.preview.GotoBottom()
	} else if contentChanged || scrollChanged {
		m.syncPreviewScroll()
	}
	return m, editorCmd
}

func (m model) View() tea.View {
	leftPane := m.editor.View()

	rightPane := m.preview.View()

	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *model) syncPreviewScroll() {
	if m.editor.ScrollYOffset() == 0 {
		m.preview.GotoTop()
		return
	}

	scrollPercent := m.editor.ScrollPercent()
	if scrollPercent >= 1 {
		m.preview.GotoBottom()
		return
	}

	maxOffset := max(0, m.preview.TotalLineCount()-m.preview.Height()+m.preview.Style.GetVerticalFrameSize())

	m.preview.SetYOffset(int(math.Round(scrollPercent * float64(maxOffset))))
}

func initModel(content string) model {
	editor := textarea.New()
	editor.Prompt = ""
	editor.MaxHeight = 0 // No limit on height
	editor.SetValue(content)
	editor.MoveToBegin()
	editor.Focus()

	preview := viewport.New()
	preview.Style = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder())

	return model{
		editor:  editor,
		preview: preview,
	}
}

func mayChangeEditorContent(msg tea.Msg, keyMap textarea.KeyMap) bool {
	switch msg := msg.(type) {
	case tea.MouseMsg, cursor.BlinkMsg:
		return false
	case tea.KeyPressMsg:
		return msg.Text != "" ||
			key.Matches(
				msg,
				keyMap.DeleteAfterCursor,
				keyMap.DeleteBeforeCursor,
				keyMap.DeleteCharacterBackward,
				keyMap.DeleteCharacterForward,
				keyMap.DeleteWordBackward,
				keyMap.DeleteWordForward,
				keyMap.InsertNewline,
				keyMap.Paste,
				keyMap.UppercaseWordForward,
				keyMap.LowercaseWordForward,
				keyMap.CapitalizeWordForward,
				keyMap.TransposeCharacterBackward,
			)
	default:
		return true
	}
}

func renderMarkdown(content string, width int) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", fmt.Errorf("create Markdown renderer: %w", err)
	}
	rendered, err := renderer.Render(content)
	if err != nil {
		return "", fmt.Errorf("render Markdown: %w", err)
	}
	return rendered, nil
}

func filterBlockedWheel(current tea.Model, msg tea.Msg) tea.Msg {
	wheelMsg, ok := msg.(tea.MouseWheelMsg)
	if !ok {
		return msg
	}

	m, ok := current.(model)
	if !ok {
		return msg
	}

	if m.wheelBlocked && wheelMsg.Button == m.wheelDirection {
		return nil
	}

	return msg
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: nmd <file>")
		os.Exit(1)
	}
	filename := os.Args[1]
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		initModel(string(content)),
		tea.WithFilter(filterBlockedWheel),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
