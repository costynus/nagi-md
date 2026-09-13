package main

import (
	"errors"
	"os"
	"path/filepath"
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
			m := initModel("", "# Hello")
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

func TestEditorAndPreviewScrollTogether(t *testing.T) {
	const (
		width  = 80
		height = 10
	)

	content := strings.Repeat("# Heading\n\nParagraph text.\n\n", 20) // Create enough content to require scrolling
	m := initModel("", content)

	updatedModel, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = updatedModel.(model)

	_, gotHeight := lipgloss.Size(m.View().Content)
	if gotHeight != height {
		t.Errorf("view height = %d, want %d", gotHeight, height)
	}

	if m.preview.YOffset() != 0 {
		t.Errorf("initial preview Y offset = %d, want 0", m.preview.YOffset())
	}

	pageDown := tea.KeyPressMsg{Code: tea.KeyPgDown}
	updateModel, _ := m.Update(pageDown)
	m = updateModel.(model)
	updateModel, _ = m.Update(pageDown)
	m = updateModel.(model)

	if m.editor.ScrollYOffset() == 0 {
		t.Error("editor did not scroll down")
	}

	if m.preview.YOffset() == 0 {
		t.Error("preview did not scroll down")
	}
}

func TestEditingUpdatesPreview(t *testing.T) {
	m := initModel("", "Hello")

	updatedModel, _ := m.Update(tea.WindowSizeMsg{
		Width:  80,
		Height: 24,
	})
	m = updatedModel.(model)

	updatedModel, _ = m.Update(tea.KeyPressMsg{
		Code: 'X',
		Text: "X",
	})
	m = updatedModel.(model)

	if got := m.editor.Value(); got != "XHello" {
		t.Errorf("editor value = %q, want %q", got, "XHello")
	}

	if !strings.Contains(m.preview.GetContent(), "XHello") {
		t.Error("preview was not updated after inserting text")
	}

	updatedModel, _ = m.Update(tea.KeyPressMsg{
		Code: tea.KeyBackspace,
	})
	m = updatedModel.(model)

	if got := m.editor.Value(); got != "Hello" {
		t.Errorf("editor value after deletion = %q, want %q", got, "Hello")
	}

	if strings.Contains(m.preview.GetContent(), "XHello") {
		t.Error("preview was not updated after deleting text")
	}
}

func TestFilterBlockedWheel(t *testing.T) {
	m := initModel("", "")
	m.wheelBlocked = true
	m.wheelDirection = tea.MouseWheelDown

	down := tea.MouseWheelMsg{Button: tea.MouseWheelDown}
	if got := filterBlockedWheel(m, down); got != nil {
		t.Error("blocked wheel direction was not filtered")
	}

	up := tea.MouseWheelMsg{Button: tea.MouseWheelUp}
	if got := filterBlockedWheel(m, up); got == nil {
		t.Error("opposite wheel direction was filtered")
	}
}

func TestEditingMarksNoteDirty(t *testing.T) {
	m := initModel("note.md", "Hello")

	updated, _ := m.Update(tea.KeyPressMsg{
		Code: 'X',
		Text: "X",
	})
	m = updated.(model)

	if !m.dirty {
		t.Error("note was not marked dirty after editing")
	}
}

func TestAutosaveTickStartsSaveOnlyWhenDirty(t *testing.T) {
	tests := []struct {
		name     string
		dirty    bool
		wantSave bool
	}{
		{name: "clean note", dirty: false, wantSave: false},
		{name: "dirty note", dirty: true, wantSave: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := initModel("note.md", "Hello")
			m.dirty = tt.dirty

			updated, cmd := m.Update(autosaveTickMsg{})
			m = updated.(model)

			if m.saving != tt.wantSave {
				t.Errorf("saving = %v, want %v", m.saving, tt.wantSave)
			}

			if cmd == nil {
				t.Errorf("autosave tick did not schedule a command")
			}
		})
	}
}

func TestSaveNoteWritesContent(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "test_note.md")
	const content = "# Hello\n\nAutosaved content."

	cmd := saveNote(filename, content)
	msg := cmd()

	savedMsg, ok := msg.(noteSavedMsg)
	if !ok {
		t.Fatalf("saveNote command returned %T, want noteSavedMsg", msg)
	}

	if savedMsg.err != nil {
		t.Fatalf("saveNote returned error: %v", savedMsg.err)
	}

	if savedMsg.content != content {
		t.Errorf("saved content = %q, want %q", savedMsg.content, content)
	}

	savedContent, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read saved note: %v", err)
	}

	if got := string(savedContent); got != content {
		t.Errorf("saved file content = %q, want %q", got, content)
	}
}

func TestSaveResultUpdatesDirtyState(t *testing.T) {
	tests := []struct {
		name          string
		editorContent string
		savedContent  string
		wantDirty     bool
	}{
		{
			name:          "saved content is current",
			editorContent: "Current content",
			savedContent:  "Current content",
			wantDirty:     false,
		},
		{
			name:          "editor changed while saving",
			editorContent: "Newer content",
			savedContent:  "Older content",
			wantDirty:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := initModel("note.md", tt.editorContent)
			m.saving = true
			m.dirty = true

			updated, cmd := m.Update(noteSavedMsg{
				content: tt.savedContent,
			})
			m = updated.(model)

			if m.saving {
				t.Error("model remained in saving state after save completed")
			}

			if m.dirty != tt.wantDirty {
				t.Errorf("dirty = %v, want %v", m.dirty, tt.wantDirty)
			}

			if cmd != nil {
				t.Error("save result unexpectedly returned a command")
			}
		})
	}
}

func TestQuitSavesDirtyNoteBeforeExiting(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "note.md")
	const content = "Changed content"

	m := initModel(filename, content)
	m.dirty = true

	updated, saveCmd := m.Update(tea.KeyPressMsg{
		Code: 'c',
		Mod:  tea.ModCtrl,
	})
	m = updated.(model)

	if !m.saving {
		t.Error("ctrl+c did not start saving")
	}

	if !m.quitAfterSave {
		t.Error("ctrl+c did not defer quitting until save completes")
	}

	if saveCmd == nil {
		t.Fatal("ctrl+c did not return a save command")
	}

	savedMsg, ok := saveCmd().(noteSavedMsg)
	if !ok {
		t.Fatalf("save command returned unexpected message type")
	}

	updated, quitCmd := m.Update(savedMsg)
	m = updated.(model)

	if m.dirty {
		t.Error("note remained dirty after successful save")
	}

	if quitCmd == nil {
		t.Fatal("successful save did not return a quit command")
	}

	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Error("command after successful save was not tea.Quit")
	}

	savedContent, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read saved note: %v", err)
	}

	if got := string(savedContent); got != content {
		t.Errorf("saved file content = %q, want %q", got, content)
	}
}

func TestQuitWaitsForInFlightSave(t *testing.T) {
	const content = "Current content"

	m := initModel("note.md", content)
	m.dirty = true
	m.saving = true

	updated, cmd := m.Update(tea.KeyPressMsg{
		Code: 'c',
		Mod:  tea.ModCtrl,
	})
	m = updated.(model)

	if !m.quitAfterSave {
		t.Error("ctrl+c did not defer quitting")
	}

	if cmd != nil {
		t.Error("ctrl+c started another command while save was in progress")
	}

	updated, quitCmd := m.Update(noteSavedMsg{
		content: content,
	})
	m = updated.(model)

	if m.saving {
		t.Error("model remained in saving state")
	}

	if m.dirty {
		t.Error("note remained dirty after successful save")
	}

	if quitCmd == nil {
		t.Fatal("completed save did not return a quit command")
	}

	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Error("command after completed save was not tea.Quit")
	}
}

func TestSaveFailureRemainsVisibleAndCancelsQuit(t *testing.T) {
	m := initModel("note.md", "Unsaved content")

	updated, _ := m.Update(tea.WindowSizeMsg{
		Width:  80,
		Height: 24,
	})
	m = updated.(model)

	m.saving = true
	m.dirty = true
	m.quitAfterSave = true

	saveErr := errors.New("disk full")

	updated, cmd := m.Update(noteSavedMsg{
		content: "Unsaved content",
		err:     saveErr,
	})
	m = updated.(model)

	if m.saving {
		t.Error("model remained in saving state after failure")
	}

	if !m.dirty {
		t.Error("failed save incorrectly marked note as clean")
	}

	if m.quitAfterSave {
		t.Error("failed save did not cancel deferred quit")
	}

	if !errors.Is(m.saveErr, saveErr) {
		t.Errorf("saveErr = %v, want %v", m.saveErr, saveErr)
	}

	if cmd != nil {
		t.Error("failed save unexpectedly returned a command")
	}

	if !strings.Contains(m.View().Content, "Save failed: disk full") {
		t.Error("save failure is not visible in the view")
	}
}
