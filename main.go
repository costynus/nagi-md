package main

import (
	"fmt"
	"os"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
	lipgloss "charm.land/lipgloss/v2"
)

type model struct {
	content string
	width   int
	height  int
	preview viewport.Model
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		leftWidth := m.width / 2
		rightWidth := m.width - leftWidth
		m.preview.SetWidth(rightWidth)
		m.preview.SetHeight(m.height)

		previewWidth := max(1, rightWidth-m.preview.Style.GetHorizontalFrameSize())
		rendered, err := renderMarkdown(m.content, previewWidth)
		if err != nil {
			rendered = fmt.Sprintf("Error rendering Markdown: %v", err)
		}
		m.preview.SetContent(rendered)
	}
	updatedPreview, cmd := m.preview.Update(msg)
	m.preview = updatedPreview
	return m, cmd
}

func (m model) View() tea.View {
	leftWidth := m.width / 2

	leftPane := lipgloss.NewStyle().
		Width(leftWidth).
		Height(m.height).
		MaxHeight(m.height).
		Render(m.content)

	rightPane := m.preview.View()

	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func initModel(content string) model {
	preview := viewport.New()
	preview.Style = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder())

	return model{
		content: content,
		preview: preview,
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

	p := tea.NewProgram(initModel(string(content)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
