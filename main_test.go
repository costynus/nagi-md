package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

func TestViewFillsTerminal(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{name: "even width", width: 80, height: 24},
		{name: "odd width", width: 81, height: 24},
		{name: "smaller terminal", width: 40, height: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := initModel("# Hello")
			updatedModel, _ := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			m = updatedModel.(model)
			view := m.View()
			gotWidth, gotHeight := lipgloss.Size(view.Content)

			if gotWidth != tt.width {
				t.Errorf("width = %d, want %d", gotWidth, tt.width)
			}
			if gotHeight != tt.height {
				t.Errorf("height = %d, want %d", gotHeight, tt.height)
			}
		})
	}
}

func TestRenderMarkdown(t *testing.T) {
	const width = 30
	content := "# Hello\n\nThis is a test of the markdown rendering."

	rendered, err := renderMarkdown(content, width)
	if err != nil {
		t.Fatalf("renderMarkdown() returned error: %v", err)
	}

	if !strings.Contains(rendered, "Hello") {
		t.Errorf("rendered content does not contain expected header 'Hello'")
	}

	if strings.Contains(rendered, "# Hello") {
		t.Errorf("Markdown heading was not rendered")
	}

	gotWidth, _ := lipgloss.Size(rendered)
	if gotWidth > width {
		t.Errorf("rendered content width = %d, want <= %d", gotWidth, width)
	}
}

func TestPreviewScrollsWithinTerminalHeight(t *testing.T) {
	const (
		width  = 80
		height = 10
	)

	content := strings.Repeat("# Heading\n\nParagraph text.\n\n", 20) // Create enough content to require scrolling
	m := initModel(content)

	updatedModel, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = updatedModel.(model)

	_, gotHeight := lipgloss.Size(m.View().Content)
	if gotHeight != height {
		t.Errorf("view height = %d, want %d", gotHeight, height)
	}

	if m.preview.YOffset() != 0 {
		t.Errorf("initial preview Y offset = %d, want 0", m.preview.YOffset())
	}

	updateModel, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updateModel.(model)

	if m.preview.YOffset() == 0 {
		t.Error("preview did not scroll down")
	}
}
