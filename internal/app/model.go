package app

import (
	"io/fs"
	"path/filepath"
	"strings"

	textinput "github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"pdf-tui/internal/meta"
)

type uiState int

const (
	stateNormal uiState = iota
	stateNewDir
	stateConfirmDelete
	stateRename
	stateMetaPreview
	stateEditMeta
)

type Model struct {
	root    string
	cwd     string
	entries []fs.DirEntry
	cursor  int
	err     error

	selected map[string]bool
	cut      []string
	status   string

	viewportStart  int
	viewportHeight int
	width          int

	state        uiState
	input        textinput.Model
	confirmItems []string

	renameTarget string

	meta            *meta.Store   // <── sqlite store
	metaEditingPath string        // path of file being edited
	metaFieldIndex  int           // 0:title,1:author,2:venue,3:year
	metaDraft       meta.Metadata // draft being edited

	previewText []string
	previewPath string

	currentMeta     *meta.Metadata
	currentMetaPath string
}

var metaFieldLabels = []string{
	"Title",
	"Author",
	"Journal/Conference",
	"Year",
}

func metaFieldLabel(index int) string {
	if index >= 0 && index < len(metaFieldLabels) {
		return metaFieldLabels[index]
	}
	return ""
}

func metaFieldCount() int {
	return len(metaFieldLabels)
}

func metadataFieldValue(data meta.Metadata, index int) string {
	switch index {
	case 0:
		return data.Title
	case 1:
		return data.Author
	case 2:
		return data.Venue
	case 3:
		return data.Year
	default:
		return ""
	}
}

func setMetadataFieldValue(data *meta.Metadata, index int, value string) {
	switch index {
	case 0:
		data.Title = value
	case 1:
		data.Author = value
	case 2:
		data.Venue = value
	case 3:
		data.Year = value
	}
}

func (m *Model) loadMetaFieldIntoInput() {
	value := metadataFieldValue(m.metaDraft, m.metaFieldIndex)
	m.input.SetValue(value)
	m.input.CursorEnd()
}

func (m *Model) updateCurrentMetadata(path string, isDir bool) {
	if path == "" || isDir || m.meta == nil {
		m.currentMeta = nil
		m.currentMetaPath = ""
		return
	}
	if m.currentMetaPath == path {
		return
	}
	md, err := m.meta.Get(path)
	if err != nil {
		m.currentMeta = nil
		m.currentMetaPath = ""
		m.status = "Failed to load metadata: " + err.Error()
		return
	}
	m.currentMetaPath = path
	m.currentMeta = md
}

func NewModel(root string, store *meta.Store) Model {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.CharLimit = 200
	ti.Cursor.Style = ti.Cursor.Style.Bold(true)
	ti.Focus()

	m := Model{
		root:           root,
		cwd:            root,
		selected:       make(map[string]bool),
		input:          ti,
		viewportHeight: 20,
		meta:           store,
	}
	m.loadEntries()
	m.updateTextPreview()
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m *Model) ensureCursorVisible() {
	if m.cursor < m.viewportStart {
		m.viewportStart = m.cursor
	}
	if m.cursor >= m.viewportStart+m.viewportHeight {
		m.viewportStart = m.cursor - m.viewportHeight + 1
	}
	if m.viewportStart < 0 {
		m.viewportStart = 0
	}
}

func (m *Model) updateTextPreview() {
	m.previewText = nil

	if len(m.entries) == 0 {
		m.previewPath = ""
		m.updateCurrentMetadata("", true)
		return
	}

	e := m.entries[m.cursor]
	full := filepath.Join(m.cwd, e.Name())
	m.updateCurrentMetadata(full, e.IsDir())

	// Directories: simple info
	if e.IsDir() {
		m.previewPath = full
		m.previewText = []string{
			"Directory:",
			"  " + e.Name() + "/",
		}
		return
	}

	// Non-PDFs: no text preview
	if !strings.HasSuffix(strings.ToLower(e.Name()), ".pdf") {
		m.previewPath = full
		m.previewText = []string{
			"No preview (not a PDF)",
			"",
			e.Name(),
		}
		return
	}

	// If we already have preview for this file, keep it
	if m.previewPath == full && len(m.previewText) > 0 {
		return
	}

	m.previewPath = full

	// approximate how many lines we can show
	maxLines := m.viewportHeight - 2
	if maxLines < 5 {
		maxLines = 5
	}

	lines, err := extractFirstPageText(full, maxLines)
	if err != nil {
		m.previewText = []string{
			"Preview error:",
			"  " + err.Error(),
		}
		return
	}

	m.previewText = lines
}
