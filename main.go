package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
	lipgloss "charm.land/lipgloss/v2"
)

type (
	model struct {
		filename      string
		saving        bool
		dirty         bool
		saveErr       error
		quitAfterSave bool

		editor  textarea.Model
		width   int
		height  int
		preview viewport.Model

		wheelDirection tea.MouseButton
		wheelBlocked   bool
	}

	noteSavedMsg struct {
		content string
		err     error
	}

	autosaveTickMsg struct{}
)

func (m model) Init() tea.Cmd {
	return tea.Batch(
		cursor.Blink,
		scheduleAutosave(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case noteSavedMsg:
		m.saving = false
		if msg.err != nil {
			m.saveErr = msg.err
			m.dirty = true
			m.quitAfterSave = false
			return m, nil
		}

		m.saveErr = nil
		m.dirty = m.editor.Value() != msg.content

		if m.quitAfterSave {
			if m.dirty {
				return m, m.startSave(m.editor.Value())
			}
			return m, tea.Quit
		}
		return m, nil
	case autosaveTickMsg:
		nextTick := scheduleAutosave()
		if !m.dirty || m.saving {
			return m, nextTick
		}
		return m, tea.Batch(
			nextTick,
			m.startSave(m.editor.Value()),
		)
	case tea.KeyPressMsg:
		m.wheelBlocked = false

		switch msg.String() {
		case "esc":
			if m.editor.HasSelection() {
				m.editor.ClearSelection()
				return m, nil
			}
		case "ctrl+c":
			if m.editor.HasSelection() {
				selection := m.editor.CopySelection()
				m.editor.ClearSelection()
				return m, selection
			}
			if m.saving {
				m.quitAfterSave = true
				return m, nil
			}
			if m.dirty {
				m.quitAfterSave = true
				return m, m.startSave(m.editor.Value())
			}
			return m, tea.Quit
		}
	case tea.MouseClickMsg:
		mouse := msg.Mouse()

		leftWidth := m.width / 2
		paneHeight := m.height
		if m.height >= 2 {
			paneHeight--
		}

		clickedInsideEditor :=
			mouse.Button == tea.MouseLeft &&
				mouse.X >= 0 && mouse.X < leftWidth &&
				mouse.Y >= 0 && mouse.Y < paneHeight

		if !clickedInsideEditor {
			return m, nil
		}

		m.editor.BeginSelection(mouse.X, mouse.Y)
		return m, nil

	case tea.MouseMotionMsg:
		mouse := msg.Mouse()

		if mouse.Button != tea.MouseLeft {
			return m, nil
		}

		leftWidth := m.width / 2
		paneHeight := m.height
		if m.height >= 2 {
			paneHeight--
		}
		if leftWidth <= 0 || paneHeight <= 0 {
			return m, nil
		}

		x := min(max(mouse.X, 0), leftWidth-1)
		y := min(max(mouse.Y, 0), paneHeight-1)

		m.editor.ExtendSelection(x, y)
		return m, nil

	case tea.MouseReleaseMsg:
		m.editor.EndSelection()
		return m, nil

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
		m.preview.SetWidth(rightWidth)

		paneHeight := m.height
		if m.height >= 2 {
			paneHeight--
		}
		paneHeight = max(0, paneHeight)

		m.editor.SetHeight(paneHeight)
		m.preview.SetHeight(paneHeight)

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
	if contentChanged {
		m.dirty = true
	}
	return m, editorCmd
}

func (m model) View() tea.View {
	leftPane := m.editor.View()
	rightPane := m.preview.View()

	panes := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPane,
		rightPane,
	)

	status := ".nagi::md"
	if m.saveErr != nil {
		status = fmt.Sprintf("Save failed: %v", m.saveErr)
	}

	content := panes
	if m.height >= 2 {
		statusLine := lipgloss.NewStyle().
			Width(m.width).
			Height(1).
			MaxHeight(1).
			Render(status)

		content = lipgloss.JoinVertical(
			lipgloss.Left,
			panes,
			statusLine,
		)
	}
	if m.height <= 0 {
		content = ""
	} else {
		content = lipgloss.NewStyle().
			MaxHeight(m.height).
			Render(content)
	}

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

func initModel(filename, content string) model {
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
		filename: filename,
		editor:   editor,
		preview:  preview,
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

const autosaveInterval = 500 * time.Millisecond

func scheduleAutosave() tea.Cmd {
	return tea.Tick(autosaveInterval, func(time.Time) tea.Msg {
		return autosaveTickMsg{}
	})
}

func (m *model) startSave(content string) tea.Cmd {
	m.saving = true
	return saveNote(m.filename, content)
}

func writeNoteAtomically(filename, content string) error {
	resolvedFilename, err := filepath.EvalSymlinks(filename)
	if err != nil {
		return fmt.Errorf("resolve note path: %w", err)
	}

	fileInfo, err := os.Stat(resolvedFilename)
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	directory := filepath.Dir(resolvedFilename)
	tempFile, err := os.CreateTemp(directory, "nmd_temp_*.md")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempFileName := tempFile.Name()
	defer func() {
		tempFile.Close()
		os.Remove(tempFileName)
	}()

	if _, err := tempFile.WriteString(content); err != nil {
		return fmt.Errorf("write to temp file: %w", err)
	}

	const chmodBits = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	if err := tempFile.Chmod(fileInfo.Mode() & chmodBits); err != nil {
		return fmt.Errorf("preserve file permissions: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tempFileName, resolvedFilename); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

func saveNote(filename, content string) tea.Cmd {
	return func() tea.Msg {
		return noteSavedMsg{
			content: content,
			err:     writeNoteAtomically(filename, content),
		}
	}
}

func noteStoreDir() string {
	return filepath.Join(os.TempDir(), "nagi-md")
}

func createNote(storageDir string) (hash string, filename string, err error) {
	if err := os.MkdirAll(storageDir, 0o700); err != nil {
		return "", "", fmt.Errorf("create note store dir: %w", err)
	}

	for {
		randomBytes := make([]byte, 16)
		if _, err := rand.Read(randomBytes); err != nil {
			return "", "", fmt.Errorf("generate random bytes: %w", err)
		}
		hash = hex.EncodeToString(randomBytes)
		filename = filepath.Join(storageDir, hash+".md")

		file, err := os.OpenFile(
			filename,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			0o600,
		)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", "", fmt.Errorf("create note file: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(filename)
			return "", "", fmt.Errorf("close note file: %w", err)
		}
		return hash, filename, nil
	}
}

func loadInitialNote(args []string, storageDir string) (hash, filename, content string, err error) {
	if len(args) == 0 {
		hash, filename, err = createNote(storageDir)
		if err != nil {
			return "", "", "", fmt.Errorf("create initial note: %w", err)
		}
	} else {
		hash = args[0]
		if len(hash) != 32 {
			return "", "", "", fmt.Errorf("invalid note hash: %q", hash)
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return "", "", "", fmt.Errorf("invalid note hash: %q", hash)
		}
		hash = strings.ToLower(hash)
		filename = filepath.Join(storageDir, hash+".md")
	}
	contentRaw, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return "", "", "", fmt.Errorf("note %q not found: %w", hash, err)
	}
	if err != nil {
		return "", "", "", fmt.Errorf("read initial note: %w", err)
	}
	return hash, filename, string(contentRaw), nil
}

func main() {
	hash, filename, content, err := loadInitialNote(os.Args[1:], noteStoreDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		initModel(filename, content),
		tea.WithFilter(filterBlockedWheel),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(hash)
}
