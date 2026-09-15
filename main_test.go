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
		{name: "single row terminal", width: 40, height: 1},
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
	directory := t.TempDir()
	filename := filepath.Join(directory, "test_note.md")
	if err := os.WriteFile(filename, []byte("Old content"), 0o640); err != nil {
		t.Fatalf("create original note: %v", err)
	}

	originalInfo, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat original note: %v", err)
	}

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

	savedInfo, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat saved note: %v", err)
	}

	if got, want := savedInfo.Mode().Perm(), originalInfo.Mode().Perm(); got != want {
		t.Errorf("saved file permissions = %v, want %v", got, want)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read note directory: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("note directory contains %d files, want 1", len(entries))
	}
}

func TestSaveNoteFollowsSymbolicLink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.md")
	link := filepath.Join(directory, "link.md")

	if err := os.WriteFile(target, []byte("Old content"), 0o644); err != nil {
		t.Fatalf("create target note: %v", err)
	}

	if err := os.Symlink("target.md", link); err != nil {
		t.Skipf("symbolic links are not supported: %v", err)
	}

	const content = "New content"
	savedMsg, ok := saveNote(link, content)().(noteSavedMsg)
	if !ok {
		t.Fatal("save command did not return noteSavedMsg")
	}
	if savedMsg.err != nil {
		t.Fatalf("save through symbolic link: %v", savedMsg.err)
	}

	targetContent, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target note: %v", err)
	}
	if got := string(targetContent); got != content {
		t.Errorf("target content = %q, want %q", got, content)
	}

	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat symbolic link: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Error("saving replaced the symbolic link")
	}
}

func TestSaveNotePreservesSpecialModeBits(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(filename, []byte("Old content"), 0o640); err != nil {
		t.Fatalf("create original note: %v", err)
	}

	wantMode := os.FileMode(0o640) | os.ModeSetgid
	if err := os.Chmod(filename, wantMode); err != nil {
		t.Skipf("special mode bits are not supported: %v", err)
	}

	originalInfo, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat original note: %v", err)
	}
	if originalInfo.Mode()&os.ModeSetgid == 0 {
		t.Skip("filesystem did not retain the setgid bit")
	}

	savedMsg, ok := saveNote(filename, "New content")().(noteSavedMsg)
	if !ok {
		t.Fatal("save command did not return noteSavedMsg")
	}
	if savedMsg.err != nil {
		t.Fatalf("save note: %v", savedMsg.err)
	}

	savedInfo, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat saved note: %v", err)
	}

	const chmodBits = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	if got, want := savedInfo.Mode()&chmodBits, originalInfo.Mode()&chmodBits; got != want {
		t.Errorf("saved file mode = %v, want %v", got, want)
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
	if err := os.WriteFile(filename, []byte("Original content"), 0o644); err != nil {
		t.Fatalf("create original note: %v", err)
	}

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

func TestMouseClickMovesEditorCursor(t *testing.T) {
	m := initModel("note.md", "Hello\nWorld")

	updated, _ := m.Update(tea.WindowSizeMsg{
		Width:  80,
		Height: 24,
	})
	m = updated.(model)

	updated, _ = m.Update(tea.MouseClickMsg{
		X:      6,
		Y:      1,
		Button: tea.MouseLeft,
	})
	m = updated.(model)

	if got := m.editor.Line(); got != 1 {
		t.Errorf("cursor line = %d, want 1", got)
	}

	if got := m.editor.Column(); got != 3 {
		t.Errorf("cursor column = %d, want 3", got)
	}
}

func TestMouseClickOutsideEditorDoesNotMoveCursor(t *testing.T) {
	m := initModel("", "hello\nworld")

	updated, _ := m.Update(tea.WindowSizeMsg{
		Width:  80,
		Height: 24,
	})
	m = updated.(model)

	updated, _ = m.Update(tea.MouseClickMsg{
		X:      60,
		Y:      1,
		Button: tea.MouseLeft,
	})
	m = updated.(model)

	if got := m.editor.Line(); got != 0 {
		t.Errorf("cursor line = %d, want 0", got)
	}

	if got := m.editor.Column(); got != 0 {
		t.Errorf("cursor column = %d, want 0", got)
	}
}

func TestMouseDragSelectsText(t *testing.T) {
	m := initModel("", "Hello World")

	updated, _ := m.Update(tea.WindowSizeMsg{
		Width:  80,
		Height: 24,
	})
	m = updated.(model)

	updated, _ = m.Update(tea.MouseClickMsg{
		X:      3,
		Y:      0,
		Button: tea.MouseLeft,
	})
	m = updated.(model)

	updated, _ = m.Update(tea.MouseMotionMsg{
		X:      8,
		Y:      0,
		Button: tea.MouseLeft,
	})
	m = updated.(model)

	updated, _ = m.Update(tea.MouseReleaseMsg{
		X:      8,
		Y:      0,
		Button: tea.MouseLeft,
	})
	m = updated.(model)

	if !m.editor.HasSelection() {
		t.Fatal("mouse drag did not create a selection")
	}

	if got := m.editor.SelectedText(); got != "Hello" {
		t.Errorf("selected text = %q, want %q", got, "Hello")
	}
}

func TestEscapeClearsMouseSelection(t *testing.T) {
	m := initModel("", "Hello World")

	m.editor.BeginSelection(3, 0)
	m.editor.ExtendSelection(8, 0)
	m.editor.EndSelection()

	if !m.editor.HasSelection() {
		t.Fatal("expected active selection before pressing escape")
	}

	updated, _ := m.Update(tea.KeyPressMsg{
		Code: tea.KeyEscape,
	})
	m = updated.(model)

	if m.editor.HasSelection() {
		t.Error("escape did not clear selection")
	}

	if got := m.editor.Value(); got != "Hello World" {
		t.Errorf("editor content = %q, want %q", got, "Hello World")
	}

	if m.dirty {
		t.Error("clearing selection incorrectly marked note dirty")
	}
}

func TestCtrlCCopiesAndClearsSelection(t *testing.T) {
	m := initModel("", "Hello World")

	m.editor.BeginSelection(3, 0)
	m.editor.ExtendSelection(8, 0)
	m.editor.EndSelection()

	if !m.editor.HasSelection() {
		t.Fatal("expected selection before copying")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{
		Code: 'c',
		Mod:  tea.ModCtrl,
	})
	m = updated.(model)

	if cmd == nil {
		t.Fatal("ctrl+c did not return a copy command")
	}

	if m.editor.HasSelection() {
		t.Error("ctrl+c did not clear selection")
	}

	if m.quitAfterSave {
		t.Error("ctrl+c attempted to quit while copying selection")
	}
}

func TestCreateNote(t *testing.T) {
	storageDir := t.TempDir()

	hash, filename, err := createNote(storageDir)
	if err != nil {
		t.Fatalf("createNote() returned error: %v", err)
	}

	wantFilename := filepath.Join(storageDir, hash+".md")
	if filename != wantFilename {
		t.Errorf("createNote() returned filename = %q, want %q", filename, wantFilename)
	}

	if _, err := os.Stat(filename); err != nil {
		t.Fatalf("created note file does not exist: %v", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read created note file: %v", err)
	}

	if len(content) != 0 {
		t.Errorf("created note file is not empty, length = %d", len(content))
	}

	secondHash, secondFilename, err := createNote(storageDir)
	if err != nil {
		t.Fatalf("createNote() returned error: %v", err)
	}

	if secondHash == hash {
		t.Errorf("createNote() returned duplicate hash: %q", secondHash)
	}

	if secondFilename == filename {
		t.Errorf("createNote() returned duplicate filename: %q", secondFilename)
	}
}

func TestLoadInitialNoteWithoutArgs(t *testing.T) {
	storageDir := t.TempDir()

	hash, filename, content, err := loadInitialNote([]string{}, storageDir)
	if err != nil {
		t.Fatalf("loadInitialNote() returned error: %v", err)
	}

	if filepath.Dir(filename) != storageDir {
		t.Errorf(
			"note directory = %q, want %q",
			filepath.Dir(filename),
			storageDir,
		)
	}

	if content != "" {
		t.Errorf("content = %q, want empty string", content)
	}

	if hash == "" {
		t.Error("hash is empty, expected a new note hash")
	}

	if filepath.Base(filename) != hash+".md" {
		t.Errorf(
			"filename = %q, want %q",
			filepath.Base(filename),
			hash+".md",
		)
	}
}

func TestLoadInitialNoteWithExistingFile(t *testing.T) {
	storageDir := t.TempDir()
	hash := "0123456789abcdef0123456789abcdef"
	filename := filepath.Join(storageDir, hash+".md")

	if err := os.WriteFile(filename, []byte("Hello"), 0o644); err != nil {
		t.Fatalf("create note file: %v", err)
	}

	_, gotFilename, content, err := loadInitialNote([]string{hash}, storageDir)
	if err != nil {
		t.Fatalf("loadInitialNote() returned error: %v", err)
	}

	if gotFilename != filename {
		t.Errorf("filename = %q, want %q", gotFilename, filename)
	}

	if content != "Hello" {
		t.Errorf("content = %q, want %q", content, "Hello")
	}
}

func TestLoadInitialNoteWithUnknownHash(t *testing.T) {
	storageDir := t.TempDir()
	hash := "0123456789abcdef0123456789abcdef"

	_, _, _, err := loadInitialNote([]string{hash}, storageDir)
	if err == nil {
		t.Fatal("loadInitialNote() did not return error for unknown hash")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error message = %q, want to contain 'not found'", err.Error())
	}
}

func TestLoadInitialNotesRejectsInvalidHash(t *testing.T) {
	storageDir := t.TempDir()

	_, _, _, err := loadInitialNote(
		[]string{".../outside"},
		storageDir,
	)
	if err == nil {
		t.Fatal("loadInitialNote() returned nil error for invalid hash")
	}

	if !strings.Contains(err.Error(), "invalid note hash") {
		t.Errorf("error message = %q, want to contain 'invalid note hash'", err.Error())
	}
}
