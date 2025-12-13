package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"gorae/internal/arxiv"
	"gorae/internal/config"
	"gorae/internal/meta"
)

type configEditFinishedMsg struct {
	err error
}

type metadataEditFinishedMsg struct {
	err        error
	tmpPath    string
	targetPath string
}

type noteEditFinishedMsg struct {
	err        error
	targetPath string
}

type arxivUpdateMsg struct {
	arxivID      string
	updatedPaths []string
	err          error
}

type arxivBatch struct {
	id    string
	files []string
}

const arxivRequestTimeout = 30 * time.Second

var (
	arxivModernIDPattern = regexp.MustCompile(`(?i)(\d{4}\.\d{4,5})(v\d+)?`)
	arxivLegacyIDPattern = regexp.MustCompile(`(?i)([a-z-]+(?:\.[a-z-]+)?/[0-9]{7})(v\d+)?`)
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.windowHeight = msg.Height
		m.viewportHeight = msg.Height - 5
		if m.viewportHeight < 1 {
			m.viewportHeight = 1
		}
		m.width = msg.Width
		m.ensureCursorVisible()
		m.clampMetaPopupOffset()
		if m.state == stateSearchResults {
			m.ensureSearchResultVisible()
		}
		return m, nil

	case configEditFinishedMsg:
		if msg.err != nil {
			m.setStatus("Config edit failed: " + msg.err.Error())
		} else {
			m.setStatus("Config edit finished")
		}
		return m, nil

	case metadataEditFinishedMsg:
		m.handleMetadataEditorFinished(msg)
		return m, nil

	case noteEditFinishedMsg:
		m.handleNoteEditorFinished(msg)
		return m, nil

	case arxivUpdateMsg:
		if msg.err != nil {
			m.setStatus("arXiv import failed: " + msg.err.Error())
			m.pendingArxivFiles = nil
			m.pendingArxivActive = ""
			m.pendingArxivQueue = nil
			if m.state == stateArxivPrompt {
				m.state = stateNormal
				m.input.SetValue("")
				m.input.Blur()
			}
			return m, nil
		}
		if len(msg.updatedPaths) > 0 {
			current := m.currentEntryPath()
			m.resortAndPreserveSelection()
			for _, path := range msg.updatedPaths {
				if m.currentMetaPath == path {
					m.currentMetaPath = ""
					m.updateCurrentMetadata(path)
				}
				if current != "" && path == current {
					m.updateTextPreview()
				}
			}
		}
		count := len(msg.updatedPaths)
		summary := ""
		if count == 0 {
			summary = "arXiv import completed, but no files were updated"
		} else {
			summary = fmt.Sprintf("arXiv %s metadata applied to %d file(s)", msg.arxivID, count)
		}

		if len(m.pendingArxivFiles) > 0 {
			if prompt := m.startNextArxivPrompt(); prompt != "" {
				m.setPersistentStatus(fmt.Sprintf("%s. %s", summary, prompt))
				return m, nil
			}
		} else if cmd := m.nextArxivQueueCmd(); cmd != nil {
			m.setPersistentStatus(fmt.Sprintf("%s. Continuing arXiv updates...", summary))
			return m, cmd
		}
		m.setStatus(summary)
		return m, nil

	case searchResultMsg:
		if msg.err != nil {
			m.setStatus("Search failed: " + msg.err.Error())
			return m, nil
		}
		m.clearCommandOutput()
		m.enterSearchResults(msg)
		if msg.summary != "" {
			m.setPersistentStatus(msg.summary + " (Esc/q closes, Enter opens)")
		} else {
			m.setStatus("Search finished")
		}
		return m, nil

	case tea.KeyMsg:
		key := msg.String()

		if m.state == stateSearchResults {
			if handled, cmd := m.handleSearchResultsKey(key); handled {
				return m, cmd
			}
		}

		if m.state != stateCommand && len(m.commandOutput) > 0 && !m.commandOutputPinned {
			m.clearCommandOutput()
		}

		if m.commandOutputPinned && m.state == stateNormal {
			switch key {
			case "esc", "q":
				m.clearCommandOutput()
				m.setStatus("Command output closed")
				return m, nil
			case "j", "down":
				m.scrollCommandOutput(1)
				return m, nil
			case "k", "up":
				m.scrollCommandOutput(-1)
				return m, nil
			case "pgdown", "ctrl+f":
				m.scrollCommandOutput(m.commandOutputViewHeight())
				return m, nil
			case "pgup", "ctrl+b":
				m.scrollCommandOutput(-m.commandOutputViewHeight())
				return m, nil
			}
		}

		if m.state != stateNormal && m.awaitingSort {
			m.awaitingSort = false
		}

		if m.state == stateNormal && m.awaitingSort {
			m.awaitingSort = false
			switch strings.ToLower(key) {
			case "t":
				m.applySortMode(sortByTitle)
			case "y":
				m.applySortMode(sortByYear)
			default:
				m.setStatus("Sort cancelled")
			}
			return m, nil
		}

		// ===========================
		//  NEW DIRECTORY MODE
		// ===========================
		if m.state == stateNewDir {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)

			switch key {
			case "enter":
				name := strings.TrimSpace(m.input.Value())
				m.state = stateNormal
				m.input.SetValue("")

				if name == "" {
					m.setStatus("Directory name cannot be empty")
					return m, cmd
				}
				if strings.HasPrefix(name, ".") {
					m.setStatus("Dot directories are hidden; choose another name")
					return m, cmd
				}

				dst := filepath.Join(m.cwd, name)
				if _, err := os.Stat(dst); err == nil {
					m.setStatus("Already exists")
					return m, cmd
				}

				if err := os.MkdirAll(dst, 0o755); err != nil {
					m.setStatus("Failed: " + err.Error())
					return m, cmd
				}

				m.loadEntries()

				// jump to new folder
				for i, e := range m.entries {
					if e.IsDir() && e.Name() == name {
						m.cursor = i
						break
					}
				}
				m.ensureCursorVisible()
				m.setStatus("Directory created")
				return m, cmd

			case "esc", "q":
				m.state = stateNormal
				m.setStatus("Cancelled")
				m.input.SetValue("")
				return m, cmd
			}

			return m, cmd
		}

		// ===========================
		//  RENAME MODE
		// ===========================
		if m.state == stateRename {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)

			switch key {
			case "enter":
				newName := strings.TrimSpace(m.input.Value())
				oldPath := m.renameTarget

				m.state = stateNormal
				m.input.SetValue("")
				m.renameTarget = ""

				if newName == "" {
					m.setStatus("Name cannot be empty")
					return m, cmd
				}

				if strings.Contains(newName, "/") {
					m.setStatus("Name cannot contain '/'")
					return m, cmd
				}

				dir := filepath.Dir(oldPath)
				newPath := filepath.Join(dir, newName)

				if _, err := os.Stat(newPath); err == nil {
					m.setStatus("Target already exists")
					return m, cmd
				}

				if err := os.Rename(oldPath, newPath); err != nil {
					m.setStatus("Rename failed: " + err.Error())
					return m, cmd
				}

				var metaErr error
				if err := m.moveMetadataPaths(oldPath, newPath, true); err != nil {
					metaErr = err
				}

				m.loadEntries()
				for i, e := range m.entries {
					if e.Name() == newName {
						m.cursor = i
						break
					}
				}
				m.ensureCursorVisible()
				m.updateTextPreview()
				if metaErr != nil {
					m.setStatus("Renamed, but metadata update failed: " + metaErr.Error())
				} else {
					m.setStatus("Renamed")
				}
				return m, cmd

			case "esc":
				m.state = stateNormal
				m.input.SetValue("")
				m.renameTarget = ""
				m.setStatus("Rename cancelled")
				return m, cmd
			}

			return m, cmd
		}

		// ===========================
		//  EDIT METADATA MODE
		// ===========================
		if m.state == stateMetaPreview {
			switch key {
			case "e":
				m.state = stateEditMeta
				m.metaFieldIndex = 0
				m.metaPopupOffset = 0
				m.loadMetaFieldIntoInput()
				m.input.Focus()
				m.setPersistentStatus(metaEditStatus(m.metaFieldIndex))
				return m, nil
			case "v":
				m.metaPopupOffset = 0
				if cmd := m.launchMetadataEditor(); cmd != nil {
					return m, cmd
				}
				return m, nil
			case "n":
				if cmd := m.launchNoteEditor(); cmd != nil {
					return m, cmd
				}
				return m, nil
			case "up", "k":
				m.scrollMetaPopup(-1)
				return m, nil
			case "down", "j":
				m.scrollMetaPopup(1)
				return m, nil
			case "pgup":
				step := m.viewportHeight / 2
				if step < 3 {
					step = 3
				}
				m.scrollMetaPopup(-step)
				return m, nil
			case "pgdown":
				step := m.viewportHeight / 2
				if step < 3 {
					step = 3
				}
				m.scrollMetaPopup(step)
				return m, nil
			case "esc", "q":
				m.state = stateNormal
				m.metaEditingPath = ""
				m.metaPopupOffset = 0
				m.setStatus("Metadata edit cancelled")
				return m, nil
			}
			return m, nil
		}

		if m.state == stateEditMeta {
			var cmd tea.Cmd
			if key != "tab" && key != "shift+tab" {
				m.input, cmd = m.input.Update(msg)
			}

			switch key {
			case "tab":
				val := strings.TrimSpace(m.input.Value())
				setMetadataFieldValue(&m.metaDraft, m.metaFieldIndex, val)
				if m.metaFieldIndex < metaFieldCount()-1 {
					m.metaFieldIndex++
				}
				m.loadMetaFieldIntoInput()
				m.setPersistentStatus(metaEditStatus(m.metaFieldIndex))
				return m, cmd

			case "shift+tab":
				val := strings.TrimSpace(m.input.Value())
				setMetadataFieldValue(&m.metaDraft, m.metaFieldIndex, val)
				if m.metaFieldIndex > 0 {
					m.metaFieldIndex--
				}
				m.loadMetaFieldIntoInput()
				m.setPersistentStatus(metaEditStatus(m.metaFieldIndex))
				return m, cmd

			case "enter":
				val := strings.TrimSpace(m.input.Value())
				setMetadataFieldValue(&m.metaDraft, m.metaFieldIndex, val)

				if m.metaFieldIndex < metaFieldCount()-1 {
					m.metaFieldIndex++
					m.loadMetaFieldIntoInput()
					m.setPersistentStatus(metaEditStatus(m.metaFieldIndex))
					return m, cmd
				}

				if m.metaDraft.Path == "" {
					m.metaDraft.Path = m.metaEditingPath
				}
				if m.meta != nil {
					ctx := context.Background()
					if err := m.meta.Upsert(ctx, &m.metaDraft); err != nil {
						m.setStatus("Failed to save metadata: " + err.Error())
					} else {
						m.setStatus("Metadata saved")
						m.currentMetaPath = ""
						m.resortAndPreserveSelection()
					}
				} else {
					m.setStatus("Metadata store not available")
				}
				m.state = stateNormal
				m.input.SetValue("")
				m.metaEditingPath = ""
				m.metaPopupOffset = 0
				return m, cmd

			case "esc":
				m.state = stateNormal
				m.input.SetValue("")
				m.metaEditingPath = ""
				m.metaPopupOffset = 0
				m.setStatus("Metadata edit cancelled")
				return m, cmd
			}

			return m, cmd
		}

		// ===========================
		//  COMMAND MODE
		// ===========================
		if m.state == stateCommand {
			if key == "tab" {
				if m.handleCommandAutocomplete() {
					return m, nil
				}
			}
			var inputCmd tea.Cmd
			m.input, inputCmd = m.input.Update(msg)

			switch key {
			case "enter":
				line := m.input.Value()
				m.state = stateNormal
				m.input.SetValue("")
				m.input.Blur()
				cmd := m.runCommand(line)
				return m, tea.Batch(inputCmd, cmd)
			case "esc":
				m.state = stateNormal
				m.input.SetValue("")
				m.input.Blur()
				m.setStatus("Command cancelled")
				return m, inputCmd
			default:
				return m, inputCmd
			}
		}

		// ===========================
		//  SEARCH PROMPT MODE
		// ===========================
		if m.state == stateSearchPrompt {
			var inputCmd tea.Cmd
			m.input, inputCmd = m.input.Update(msg)

			switch key {
			case "enter":
				line := strings.TrimSpace(m.input.Value())
				m.input.SetValue("")
				m.input.Blur()
				m.state = stateNormal

				if line == "" {
					m.setStatus("Search query cannot be empty")
					return m, inputCmd
				}

				tokens, err := splitCommandLine(line)
				if err != nil {
					m.setStatus("Search parse failed: " + err.Error())
					return m, inputCmd
				}
				req, err := m.buildSearchRequest(tokens)
				if err != nil {
					m.setStatus(err.Error())
					return m, inputCmd
				}
				cmd := m.runSearch(req)
				return m, tea.Batch(inputCmd, cmd)

			case "esc":
				m.state = stateNormal
				m.input.SetValue("")
				m.input.Blur()
				m.setStatus("Search cancelled")
				return m, inputCmd

			default:
				return m, inputCmd
			}
		}

		// ===========================
		//  ARXIV PROMPT MODE
		// ===========================
		if m.state == stateArxivPrompt {
			var inputCmd tea.Cmd
			m.input, inputCmd = m.input.Update(msg)

			switch key {
			case "enter":
				id := strings.TrimSpace(m.input.Value())
				if id == "" {
					m.setStatus("arXiv ID cannot be empty")
					return m, inputCmd
				}
				target := strings.TrimSpace(m.pendingArxivActive)
				if target == "" {
					m.setStatus("No file selected for arXiv import")
					m.state = stateNormal
					m.input.SetValue("")
					m.input.Blur()
					m.pendingArxivFiles = nil
					return m, inputCmd
				}
				m.pendingArxivActive = ""
				m.state = stateNormal
				m.input.SetValue("")
				m.input.Blur()
				cmd := m.runArxivFetch(id, []string{target})
				return m, tea.Batch(inputCmd, cmd)

			case "esc", "q":
				m.pendingArxivActive = ""
				m.pendingArxivFiles = nil
				m.state = stateNormal
				m.input.SetValue("")
				m.input.Blur()
				m.setStatus("arXiv command cancelled")
				return m, inputCmd

			default:
				return m, inputCmd
			}
		}

		// ===========================
		// DELETE CONFIRMATION MODE
		// ===========================
		if m.state == stateConfirmDelete {
			switch key {
			case "y", "Y", "enter":
				deleted := 0
				var lastErr error

				for _, path := range m.confirmItems {
					if err := os.RemoveAll(path); err != nil {
						lastErr = err
						continue
					}
					deleted++
					delete(m.selected, path)
					m.removeFromCut(path)
				}

				m.confirmItems = nil
				m.state = stateNormal
				m.loadEntries()

				if deleted > 0 {
					m.setStatus(fmt.Sprintf("Deleted %d item(s).", deleted))
				} else if lastErr != nil {
					m.setStatus("Delete failed: " + lastErr.Error())
				} else {
					m.setStatus("Nothing deleted")
				}

				return m, nil

			case "n", "N", "esc":
				m.state = stateNormal
				m.confirmItems = nil
				m.setStatus("Deletion cancelled")
				return m, nil
			}
		}

		// ===========================
		// NORMAL MODE
		// ===========================
		switch key {

		case "q", "ctrl+c":
			return m, tea.Quit

		case "j", "down":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.ensureCursorVisible()
				m.updateTextPreview() // <── NEW
			}

		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				m.ensureCursorVisible()
				m.updateTextPreview()
			}

		case "g":
			m.cursor = 0
			m.ensureCursorVisible()

		case "G":
			if n := len(m.entries); n > 0 {
				m.cursor = n - 1
				m.ensureCursorVisible()
			}

		// case "enter", "l":
		// 	if len(m.entries) == 0 {
		// 		return m, nil
		// 	}
		// 	entry := m.entries[m.cursor]
		// 	full := filepath.Join(m.cwd, entry.Name())

		// 	if entry.IsDir() {
		// 		m.cwd = full
		// 		m.loadEntries()
		// 		m.status = ""
		// 	} else if strings.HasSuffix(strings.ToLower(entry.Name()), ".pdf") {
		// 		_ = exec.Command("zathura", full).Start()
		// 	} else {
		// 		m.status = "Not a PDF"
		// 	}

		case "enter", "l":
			if len(m.entries) == 0 {
				return m, nil
			}
			entry := m.entries[m.cursor]
			full := filepath.Join(m.cwd, entry.Name())

			if entry.IsDir() {
				m.cwd = full
				m.loadEntries()
				m.clearStatus()
				m.updateTextPreview() // <── NEW
			} else if strings.HasSuffix(strings.ToLower(entry.Name()), ".pdf") {
				if err := m.openPDF(full); err != nil {
					m.setStatus("Failed to open PDF: " + err.Error())
				} else {
					m.recordRecentlyOpened(full)
				}
			} else {
				m.setStatus("Not a PDF")
			}

		case "h", "backspace":
			parent := filepath.Dir(m.cwd)

			if parent == m.cwd || !strings.HasPrefix(parent, m.root) {
				m.setStatus("Already at root")
				return m, nil
			}

			m.cwd = parent
			m.loadEntries()
			m.clearStatus()
			m.updateTextPreview() // <── NEW

		case "s":
			m.awaitingSort = true
			m.setStatus("Sort: 't' by title, 'y' by year")
			return m, nil

		case " ":
			if len(m.entries) == 0 {
				return m, nil
			}

			full := filepath.Join(m.cwd, m.entries[m.cursor].Name())

			// toggle
			if m.selected[full] {
				delete(m.selected, full)
			} else {
				m.selected[full] = true
			}

			// MOVE CURSOR DOWN & keep visible
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.ensureCursorVisible()
			}

			return m, nil

		case "d":
			targets := m.selectionOrCurrent()
			if len(targets) == 0 {
				m.setStatus("Nothing to cut")
				return m, nil
			}

			m.cut = append([]string{}, targets...)
			for _, t := range targets {
				delete(m.selected, t)
			}

			m.setStatus(fmt.Sprintf("Cut %d item(s). Paste with 'p'.", len(targets)))

		case "p":
			if len(m.cut) == 0 {
				m.setStatus("Cut buffer empty")
				return m, nil
			}

			moved := 0
			var lastErr error
			var metaErr error

			for _, src := range m.cut {
				info, err := os.Stat(src)
				if err != nil {
					lastErr = err
					continue
				}

				dst := filepath.Join(m.cwd, filepath.Base(src))
				dst = avoidNameClash(dst)

				if err := os.Rename(src, dst); err != nil {
					lastErr = err
					continue
				}

				if err := m.moveMetadataPaths(src, dst, info.IsDir()); err != nil {
					metaErr = err
				}

				moved++
			}

			m.cut = nil
			m.loadEntries()
			m.updateTextPreview()

			if moved > 0 {
				msg := fmt.Sprintf("Moved %d item(s).", moved)
				if metaErr != nil {
					msg += " Metadata update failed: " + metaErr.Error()
				}
				m.setStatus(msg)
			} else if lastErr != nil {
				m.setStatus("Move failed: " + lastErr.Error())
			} else if metaErr != nil {
				m.setStatus("Metadata update failed: " + metaErr.Error())
			}

		case "D":
			targets := m.selectionOrCurrent()
			if len(targets) == 0 {
				m.setStatus("Nothing to delete")
				return m, nil
			}

			m.confirmItems = targets
			m.state = stateConfirmDelete

			if len(targets) == 1 {
				m.setStatus(fmt.Sprintf("Delete '%s'? (y/N)", filepath.Base(targets[0])))
			} else {
				m.setStatus(fmt.Sprintf("Delete %d items? (y/N)", len(targets)))
			}

		case "r":
			if len(m.entries) == 0 {
				m.setStatus("Nothing to rename")
				return m, nil
			}

			entry := m.entries[m.cursor]
			full := filepath.Join(m.cwd, entry.Name())

			if !entry.IsDir() {
				m.setStatus("Not a directory")
				return m, nil
			}

			m.state = stateRename
			m.renameTarget = full
			m.input.SetValue(entry.Name())
			m.input.CursorEnd() // put cursor at end
			m.input.Focus()
			m.setPersistentStatus("Rename: edit name and press Enter")
			return m, nil

		case "a":
			m.state = stateNewDir
			m.input.SetValue("")
			m.input.CursorEnd()
			m.input.Focus()
			m.setPersistentStatus("New directory: type name and press Enter")

		case "e":
			if len(m.entries) == 0 {
				m.setStatus("Nothing to edit")
				return m, nil
			}

			entry := m.entries[m.cursor]
			full := filepath.Join(m.cwd, entry.Name())
			info, err := entry.Info()
			isDir := entry.IsDir()
			if err == nil {
				isDir = info.IsDir()
			}

			// For now: only files (skip dirs)
			if isDir {
				m.setStatus("Metadata editing is for files only")
				return m, nil
			}

			canonical := canonicalPath(full)

			m.state = stateMetaPreview
			m.metaEditingPath = canonical

			// load existing metadata if present
			draft := meta.Metadata{Path: canonical}
			if m.meta != nil {
				ctx := context.Background()
				existing, err := m.meta.Get(ctx, canonical)
				if err != nil {
					m.setStatus("Failed to load metadata: " + err.Error())
				} else if existing != nil {
					draft = *existing
				}
			}
			m.metaDraft = draft
			m.metaFieldIndex = 0
			m.metaPopupOffset = 0
			m.input.SetValue("")
			m.input.Blur()
			m.setPersistentStatus("Metadata preview: 'e' edit fields, 'v' open fields in editor, 'n' edit note, Esc cancel")
			return m, nil

		case ":":
			m.state = stateCommand
			m.input.SetValue(":")
			m.input.CursorEnd()
			m.input.Focus()
			m.setPersistentStatus("Command mode (:help for list, Esc to cancel)")
			return m, nil

		case "/":
			m.openSearchPrompt(m.lastSearchQuery)
			return m, nil

		case "y":
			if err := m.copyBibtexToClipboard(); err != nil {
				m.setStatus("BibTeX copy failed: " + err.Error())
			} else {
				m.setStatus("BibTeX copied to clipboard")
			}
			return m, nil

		case "v":
			if len(m.entries) == 0 {
				m.setStatus("No files to select")
				return m, nil
			}
			files := make([]string, 0, len(m.entries))
			for _, entry := range m.entries {
				isDir := entry.IsDir()
				if info, err := entry.Info(); err == nil {
					isDir = info.IsDir()
				}
				if isDir {
					continue
				}
				full := filepath.Join(m.cwd, entry.Name())
				files = append(files, full)
			}
			if len(files) == 0 {
				m.selected = make(map[string]bool)
				m.setStatus("No files to select")
				return m, nil
			}
			allSelected := len(m.selected) == len(files)
			if allSelected {
				for _, full := range files {
					if !m.selected[full] {
						allSelected = false
						break
					}
				}
			}
			if allSelected {
				for k := range m.selected {
					delete(m.selected, k)
				}
				m.setStatus("Selection cleared")
				return m, nil
			}
			if m.selected == nil {
				m.selected = make(map[string]bool, len(files))
			} else {
				for k := range m.selected {
					delete(m.selected, k)
				}
			}
			for _, full := range files {
				m.selected[full] = true
			}
			m.setStatus(fmt.Sprintf("Selected %d file(s).", len(files)))
			return m, nil

		}
	}

	return m, nil
}

func metaEditStatus(index int) string {
	label := metaFieldLabel(index)
	if index == metaFieldCount()-1 {
		return fmt.Sprintf("Edit %s (Enter to save, Tab/Shift+Tab to move, Esc to cancel)", label)
	}
	return fmt.Sprintf("Edit %s (Enter/Tab to continue, Shift+Tab to go back, Esc to cancel)", label)
}

func (m *Model) handleMetadataEditorFinished(msg metadataEditFinishedMsg) {
	if msg.tmpPath != "" {
		defer os.Remove(msg.tmpPath)
	}
	m.state = stateNormal
	m.metaEditingPath = ""
	m.input.SetValue("")
	m.metaPopupOffset = 0
	m.metaPopupOffset = 0

	if msg.err != nil {
		m.setStatus("Metadata editor failed: " + msg.err.Error())
		return
	}
	if strings.TrimSpace(msg.tmpPath) == "" {
		m.setStatus("Metadata editor failed: no data returned")
		return
	}
	target := strings.TrimSpace(msg.targetPath)
	if target == "" {
		m.setStatus("Metadata editor failed: unknown target")
		return
	}
	data, err := os.ReadFile(msg.tmpPath)
	if err != nil {
		m.setStatus("Failed to read metadata edit: " + err.Error())
		return
	}
	md, err := parseMetadataEditorData(data, target)
	if err != nil {
		m.setStatus("Failed to parse metadata: " + err.Error())
		return
	}
	if m.meta == nil {
		m.setStatus("Metadata store not available")
		return
	}
	ctx := context.Background()
	if err := m.meta.Upsert(ctx, &md); err != nil {
		m.setStatus("Failed to save metadata: " + err.Error())
		return
	}
	m.metaDraft = md
	m.currentMetaPath = ""
	m.resortAndPreserveSelection()
	m.setStatus("Metadata saved")
}

func (m *Model) handleNoteEditorFinished(msg noteEditFinishedMsg) {
	if msg.err != nil {
		m.setStatus("Note edit failed: " + msg.err.Error())
		return
	}
	target := canonicalPath(msg.targetPath)
	if target != "" && target == m.currentMetaPath {
		m.refreshCurrentNote()
	}
	m.setStatus("Note saved")
}

func (m *Model) launchMetadataEditor() tea.Cmd {
	target := strings.TrimSpace(m.metaEditingPath)
	if target == "" {
		m.setStatus("No metadata target selected")
		return nil
	}
	if strings.TrimSpace(m.metaDraft.Path) == "" {
		m.metaDraft.Path = target
	}
	tmp, err := os.CreateTemp("", "gorae-metadata-*.json")
	if err != nil {
		m.setStatus("Failed to create temp file: " + err.Error())
		return nil
	}
	tmpPath := tmp.Name()
	data := metadataEditorFileFromMetadata(m.metaDraft)
	if err := writeMetadataEditorFile(tmp, data); err != nil {
		os.Remove(tmpPath)
		m.setStatus("Failed to prepare metadata for editor: " + err.Error())
		return nil
	}
	editor := m.configEditor()
	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fileName := filepath.Base(target)
	if fileName == "" {
		fileName = target
	}
	m.state = stateNormal
	m.metaEditingPath = ""
	m.input.SetValue("")
	m.setPersistentStatus(fmt.Sprintf("Editing metadata for %s with %s (exit editor to return)", fileName, editor))

	targetPath := target
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return metadataEditFinishedMsg{
			err:        err,
			tmpPath:    tmpPath,
			targetPath: targetPath,
		}
	})
}

func (m *Model) launchNoteEditor() tea.Cmd {
	target := strings.TrimSpace(m.metaEditingPath)
	if target == "" {
		m.setStatus("No metadata target selected")
		return nil
	}
	filePath, err := m.noteFilePath(target)
	if err != nil {
		m.setStatus("Failed to resolve note path: " + err.Error())
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		m.setStatus("Failed to prepare note directory: " + err.Error())
		return nil
	}
	if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		initial := []byte("")
		if err := os.WriteFile(filePath, initial, 0o644); err != nil {
			m.setStatus("Failed to initialize note: " + err.Error())
			return nil
		}
	}
	editor := m.configEditor()
	cmd := exec.Command(editor, filePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fileName := filepath.Base(target)
	if fileName == "" {
		fileName = target
	}
	m.setPersistentStatus(fmt.Sprintf("Editing note for %s with %s (exit editor to return)", fileName, editor))

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return noteEditFinishedMsg{
			err:        err,
			targetPath: target,
		}
	})
}

type metadataEditorFile struct {
	Title     string `json:"title"`
	Author    string `json:"author"`
	Year      string `json:"year"`
	Published string `json:"published"`
	URL       string `json:"url"`
	DOI       string `json:"doi"`
	Abstract  string `json:"abstract"`
	Tag       string `json:"tag"`
}

func metadataEditorFileFromMetadata(md meta.Metadata) metadataEditorFile {
	return metadataEditorFile{
		Title:     md.Title,
		Author:    md.Author,
		Year:      md.Year,
		Published: md.Published,
		URL:       md.URL,
		DOI:       md.DOI,
		Abstract:  md.Abstract,
		Tag:       md.Tag,
	}
}

func writeMetadataEditorFile(f *os.File, data metadataEditorFile) error {
	defer f.Close()
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		return err
	}
	if _, err := f.WriteString("\n"); err != nil {
		return err
	}
	return nil
}

func parseMetadataEditorData(raw []byte, path string) (meta.Metadata, error) {
	var data metadataEditorFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return meta.Metadata{}, fmt.Errorf("parse JSON: %w", err)
	}
	md := meta.Metadata{
		Path:      path,
		Title:     strings.TrimSpace(data.Title),
		Author:    strings.TrimSpace(data.Author),
		Year:      strings.TrimSpace(data.Year),
		Published: strings.TrimSpace(data.Published),
		URL:       strings.TrimSpace(data.URL),
		DOI:       strings.TrimSpace(data.DOI),
		Abstract:  strings.TrimSpace(data.Abstract),
		Tag:       strings.TrimSpace(data.Tag),
	}
	return md, nil
}

func (m *Model) openPDF(path string) error {
	viewer := ""
	if m.cfg != nil {
		viewer = strings.TrimSpace(m.cfg.PDFViewer)
	}
	if viewer == "" {
		viewer = strings.TrimSpace(config.DefaultPDFViewer())
	}
	parts, err := splitCommandLine(viewer)
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return fmt.Errorf("no PDF viewer configured")
	}
	args := append([]string{}, parts[1:]...)
	args = append(args, path)
	cmd := exec.Command(parts[0], args...)
	return cmd.Start()
}

func splitCommandLine(input string) ([]string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}
	var parts []string
	var current strings.Builder
	var quote rune
	escaped := false

	appendCurrent := func() {
		if current.Len() > 0 {
			parts = append(parts, current.String())
			current.Reset()
		}
	}

	for _, r := range input {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case unicode.IsSpace(r):
			appendCurrent()
		default:
			current.WriteRune(r)
		}
	}

	if escaped {
		return nil, fmt.Errorf("unterminated escape in command")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in command")
	}
	appendCurrent()
	return parts, nil
}

func (m *Model) scrollMetaPopup(delta int) {
	if delta == 0 {
		return
	}
	if m.state != stateMetaPreview && m.state != stateEditMeta {
		m.metaPopupOffset = 0
		return
	}
	m.metaPopupOffset += delta
	if m.metaPopupOffset < 0 {
		m.metaPopupOffset = 0
	}
	m.clampMetaPopupOffset()
}

func (m *Model) clampMetaPopupOffset() {
	if m.state != stateMetaPreview && m.state != stateEditMeta {
		m.metaPopupOffset = 0
		return
	}
	_, middleWidth, _ := m.panelWidths()
	if middleWidth <= 0 || m.viewportHeight <= 0 {
		m.metaPopupOffset = 0
		return
	}
	lines := m.metaPopupContentLines(middleWidth)
	if len(lines) == 0 {
		m.metaPopupOffset = 0
		return
	}
	maxOffset := len(lines) - m.viewportHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.metaPopupOffset > maxOffset {
		m.metaPopupOffset = maxOffset
	}
	if m.metaPopupOffset < 0 {
		m.metaPopupOffset = 0
	}
}

func (m *Model) moveMetadataPaths(oldPath, newPath string, isDir bool) error {
	if m.meta == nil {
		return nil
	}
	ctx := context.Background()
	if isDir {
		if err := m.meta.MoveTree(ctx, oldPath, newPath); err != nil {
			return err
		}
		return m.meta.MovePath(ctx, oldPath, newPath)
	}
	return m.meta.MovePath(ctx, oldPath, newPath)
}

func (m *Model) runCommand(raw string) tea.Cmd {
	text := strings.TrimSpace(raw)
	if text == "" {
		m.setStatus("No command entered")
		return nil
	}
	if strings.HasPrefix(text, ":") {
		text = strings.TrimSpace(text[1:])
	}
	if text == "" {
		m.setStatus("No command entered")
		return nil
	}

	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "h", "help":
		help := []string{
			"Command Help:",
			"  Navigation : j/k move, h up, l enter",
			"  Selection  : space toggle, d cut, p paste",
			"  Files      : a mkdir, r rename dir, D delete",
			"  Metadata   : e preview/edit fields, v edit fields in editor, n edit note in editor, y copy BibTeX, :arxiv [-v] <id> fetch from arXiv (omit <id> to be prompted)",
			"  Search     : / opens search prompt; :search or / accept -t/-a/-c/-y flags, j/k navigate results, Enter opens, Esc/q exits",
			"  Recently Added : :recent rebuilds the Recently Added directory (names show metadata titles when available)",
			"  Recently Opened: open a PDF to refresh the Recently Opened directory (keeps last 20)",
			"  Config     : :config edits config, :config show displays info, :config editor <cmd> sets editor",
			"  Commands   : :h help, :pwd show directory, :clear hide pane, :q quit",
		}
		m.setCommandOutput(help)
		m.setPersistentStatus("Help displayed (use :clear to hide)")
	case "pwd":
		output := []string{
			"Current directory:",
			"  " + m.cwd,
		}
		m.setCommandOutput(output)
		m.setStatus("Printed working directory")
	case "clear":
		m.clearCommandOutput()
		m.setStatus("Command output cleared")
	case "recent":
		if err := m.maybeSyncRecentlyAddedDir(true); err != nil {
			m.setStatus("Recently added sync failed: " + err.Error())
		} else {
			m.setStatus("Recently added directory updated")
		}
	case "config":
		return m.handleConfigCommand(args)
	case "arxiv":
		return m.handleArxivCommand(args)
	case "search":
		return m.handleSearchCommand(args)
	case "q", "quit":
		m.setStatus("Quitting...")
		return tea.Quit
	default:
		if len(args) > 0 {
			m.setStatus(fmt.Sprintf("Unknown command: %s (args: %s)", cmd, strings.Join(args, " ")))
		} else {
			m.setStatus(fmt.Sprintf("Unknown command: %s", cmd))
		}
	}
	return nil
}

func (m *Model) handleConfigCommand(args []string) tea.Cmd {
	if len(args) == 0 {
		return m.launchConfigEditor()
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "edit":
		return m.launchConfigEditor()
	case "show":
		m.displayConfigSummary()
		return nil
	case "editor":
		return m.handleEditorConfigCommand(args[1:])
	default:
		m.setStatus(fmt.Sprintf("Unknown config command: %s", sub))
		return nil
	}
}

func (m *Model) launchConfigEditor() tea.Cmd {
	path, err := config.Path()
	if err != nil {
		m.setStatus("Failed to resolve config path: " + err.Error())
		return nil
	}
	editor := m.configEditor()
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	m.setPersistentStatus(fmt.Sprintf("Editing config with %s (exit editor to return)", editor))
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return configEditFinishedMsg{err: err}
	})
}

func (m *Model) displayConfigSummary() {
	path, err := config.Path()
	if err != nil {
		m.setStatus("Failed to resolve config path: " + err.Error())
		return
	}
	editor := m.configEditor()
	viewer := ""
	if m.cfg != nil {
		viewer = strings.TrimSpace(m.cfg.PDFViewer)
	}
	if viewer == "" {
		viewer = strings.TrimSpace(config.DefaultPDFViewer())
	}
	lines := []string{
		"Config file:",
		"  " + path,
		"Configured editor:",
		"  " + editor,
		"Configured PDF viewer:",
		"  " + viewer,
		"Use :config to edit it or :config editor <cmd> to change the editor.",
	}
	m.setCommandOutput(lines)
	m.setPersistentStatus("Config info displayed (use :clear to hide)")
}

func (m *Model) handleEditorConfigCommand(args []string) tea.Cmd {
	if len(args) == 0 {
		m.setStatus(fmt.Sprintf("Current editor: %s (use :config editor <cmd> to change)", m.configEditor()))
		return nil
	}
	editor := strings.TrimSpace(strings.Join(args, " "))
	if editor == "" {
		m.setStatus("Editor command cannot be empty")
		return nil
	}
	if m.cfg == nil {
		cfg, err := config.LoadOrInit()
		if err != nil {
			m.setStatus("Failed to load config: " + err.Error())
			return nil
		}
		m.cfg = cfg
	}
	m.cfg.Editor = editor
	if err := config.Save(m.cfg); err != nil {
		m.setStatus("Failed to update editor: " + err.Error())
		return nil
	}
	m.setStatus(fmt.Sprintf("Configured editor set to %s", editor))
	return nil
}

func (m *Model) configEditor() string {
	if m.cfg != nil {
		if editor := strings.TrimSpace(m.cfg.Editor); editor != "" {
			return editor
		}
	}
	if env := strings.TrimSpace(os.Getenv("EDITOR")); env != "" {
		return env
	}
	return "vi"
}

func (m *Model) handleArxivCommand(args []string) tea.Cmd {
	if m.meta == nil {
		m.setStatus("Metadata store not available")
		return nil
	}

	useSelectionOnly := false
	var arxivID string
	fileSpecs := make([]string, 0)

	if len(args) == 0 {
		useSelectionOnly = true
	} else {
		for _, raw := range args {
			arg := strings.TrimSpace(raw)
			if arg == "" {
				continue
			}
			lower := strings.ToLower(arg)
			switch lower {
			case "-v", "--visual", "--selected":
				useSelectionOnly = true
				continue
			}
			if strings.HasPrefix(lower, "-") && (lower != "-v" && lower != "--visual" && lower != "--selected") {
				m.setStatus(fmt.Sprintf("Unknown arXiv option: %s", arg))
				return nil
			}
			if arxivID == "" {
				arxivID = arg
				continue
			}
			fileSpecs = append(fileSpecs, arg)
		}
	}

	var files []string
	if useSelectionOnly || len(fileSpecs) == 0 {
		var targets []string
		if useSelectionOnly {
			targets = m.selectedPaths()
			if len(targets) == 0 {
				m.setStatus("Select at least one file before using :arxiv -v")
				return nil
			}
		} else {
			targets = m.selectionOrCurrent()
			if len(targets) == 0 {
				m.setStatus("No files selected")
				return nil
			}
		}
		files = make([]string, 0, len(targets))
		for _, path := range targets {
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			files = append(files, path)
		}
		if len(files) == 0 {
			m.setStatus("arXiv import works on files only; select at least one PDF")
			return nil
		}
	} else {
		files = make([]string, 0, len(fileSpecs))
		for _, spec := range fileSpecs {
			resolved, err := m.resolveCommandFilePath(spec)
			if err != nil {
				m.setStatus(err.Error())
				return nil
			}
			files = append(files, resolved)
		}
	}

	if len(files) == 0 {
		m.setStatus("arXiv import works on files only; specify or select at least one PDF")
		return nil
	}

	files = uniquePaths(files)
	if len(files) == 0 {
		m.setStatus("arXiv import works on files only; specify or select at least one PDF")
		return nil
	}

	if strings.TrimSpace(arxivID) == "" {
		grouped, missing := detectArxivIDsFromFilenames(files)
		detected := 0
		for _, paths := range grouped {
			detected += len(paths)
		}
		if detected == len(files) && detected > 0 {
			m.setPersistentStatus(fmt.Sprintf("Fetching detected arXiv IDs for %d file(s)...", detected))
			return m.startDetectedArxivQueue(grouped)
		}
		if len(missing) > 0 {
			display := append([]string{}, missing...)
			if len(display) > 3 {
				display = append(display[:3], "...")
			}
			m.setStatus(fmt.Sprintf("No arXiv ID detected in: %s (enter manually)", strings.Join(display, ", ")))
		}
		m.promptArxivID(files)
		return nil
	}
	return m.runArxivFetch(arxivID, files)
}

func detectArxivIDsFromFilenames(files []string) (map[string][]string, []string) {
	grouped := make(map[string][]string)
	var missing []string
	for _, path := range files {
		id := extractArxivIDFromFilename(path)
		if id == "" {
			missing = append(missing, filepath.Base(path))
			continue
		}
		grouped[id] = append(grouped[id], path)
	}
	return grouped, missing
}

func extractArxivIDFromFilename(path string) string {
	name := filepath.Base(path)
	if name == "" {
		return ""
	}
	if match := arxivModernIDPattern.FindStringSubmatch(name); len(match) > 0 {
		return normalizeArxivMatch(match)
	}
	if match := arxivLegacyIDPattern.FindStringSubmatch(name); len(match) > 0 {
		return normalizeArxivMatch(match)
	}
	return ""
}

func normalizeArxivMatch(match []string) string {
	if len(match) < 2 {
		return ""
	}
	id := strings.TrimSpace(match[1])
	if id == "" {
		return ""
	}
	if len(match) >= 3 {
		version := strings.TrimSpace(match[2])
		if version != "" {
			id += strings.ToLower(version)
		}
	}
	return id
}

func (m *Model) handleSearchCommand(args []string) tea.Cmd {
	if len(args) == 0 {
		m.setStatus("Usage: :search [-mode title|author|year|content] [-case] [-root PATH] <query>")
		return nil
	}

	req, err := m.buildSearchRequest(args)
	if err != nil {
		m.setStatus(err.Error())
		return nil
	}
	return m.runSearch(req)
}

func detectPrefixedSearchMode(token string) (searchMode, string, bool) {
	lower := strings.ToLower(strings.TrimSpace(token))
	if lower == "" {
		return "", "", false
	}
	candidates := []searchMode{
		searchModeTitle,
		searchModeAuthor,
		searchModeYear,
		searchModeContent,
	}
	for _, candidate := range candidates {
		prefix := string(candidate) + ":"
		if strings.HasPrefix(lower, prefix) {
			trimmed := strings.TrimSpace(token[len(prefix):])
			return candidate, trimmed, true
		}
	}
	return "", "", false
}

func (m *Model) openSearchResultAtCursor() {
	match := m.currentSearchMatch()
	if match == nil {
		m.setStatus("No search result selected")
		return
	}
	if err := m.openPDF(match.Path); err != nil {
		m.setStatus("Failed to open PDF: " + err.Error())
		return
	}
	m.recordRecentlyOpened(match.Path)
	m.setStatus(fmt.Sprintf("Opened %s", filepath.Base(match.Path)))
}

func (m *Model) runSearch(req searchRequest) tea.Cmd {
	m.setPersistentStatus(fmt.Sprintf("%s search for %q...", req.mode.displayName(), req.query))
	return newSearchCmd(req)
}

func (m *Model) buildSearchRequest(tokens []string) (searchRequest, error) {
	root := canonicalPath(m.cwd)
	if root == "" {
		root = m.cwd
	}
	watchRoot := canonicalPath(m.root)
	if watchRoot == "" {
		watchRoot = m.root
	}

	req := searchRequest{
		root:          root,
		mode:          searchModeContent,
		caseSensitive: false,
		wrapWidth:     m.width,
		metaStore:     m.meta,
	}

	var queryParts []string
	for i := 0; i < len(tokens); i++ {
		token := strings.TrimSpace(tokens[i])
		if token == "" {
			continue
		}
		lower := strings.ToLower(token)
		switch {
		case lower == "-mode" || lower == "--mode":
			if i+1 >= len(tokens) {
				return searchRequest{}, fmt.Errorf("Missing value for -mode")
			}
			i++
			modeVal := strings.ToLower(strings.TrimSpace(tokens[i]))
			mode, ok := parseSearchModeValue(modeVal)
			if !ok {
				return searchRequest{}, fmt.Errorf("Unknown mode: %s", modeVal)
			}
			req.mode = mode
		case strings.HasPrefix(lower, "-mode=") || strings.HasPrefix(lower, "--mode="):
			parts := strings.SplitN(token, "=", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
				return searchRequest{}, fmt.Errorf("Missing value for -mode")
			}
			modeVal := strings.ToLower(strings.TrimSpace(parts[1]))
			mode, ok := parseSearchModeValue(modeVal)
			if !ok {
				return searchRequest{}, fmt.Errorf("Unknown mode: %s", modeVal)
			}
			req.mode = mode
		case lower == "-t" || lower == "--title":
			req.mode = searchModeTitle
		case lower == "-a" || lower == "--author":
			req.mode = searchModeAuthor
		case lower == "-c" || lower == "--content":
			req.mode = searchModeContent
		case lower == "-y" || lower == "--year":
			req.mode = searchModeYear
		case lower == "-case" || lower == "--case":
			req.caseSensitive = true
		case lower == "-root" || lower == "--root":
			if i+1 >= len(tokens) {
				return searchRequest{}, fmt.Errorf("Missing value for -root")
			}
			i++
			rootVal := strings.TrimSpace(tokens[i])
			if err := m.applySearchRoot(&req, rootVal, watchRoot); err != nil {
				return searchRequest{}, err
			}
		case strings.HasPrefix(lower, "-root=") || strings.HasPrefix(lower, "--root="):
			parts := strings.SplitN(token, "=", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
				return searchRequest{}, fmt.Errorf("Missing value for -root")
			}
			if err := m.applySearchRoot(&req, strings.TrimSpace(parts[1]), watchRoot); err != nil {
				return searchRequest{}, err
			}
		default:
			if req.mode == searchModeContent && len(queryParts) == 0 {
				if prefMode, trimmed, ok := detectPrefixedSearchMode(token); ok {
					req.mode = prefMode
					if trimmed != "" {
						queryParts = append(queryParts, trimmed)
					}
					continue
				}
			}
			queryParts = append(queryParts, token)
		}
	}

	query := strings.TrimSpace(strings.Join(queryParts, " "))
	if query == "" {
		return searchRequest{}, fmt.Errorf("Search query cannot be empty")
	}
	req.query = query
	return req, nil
}

func (m *Model) runArxivFetch(id string, files []string) tea.Cmd {
	if len(files) == 0 {
		m.setStatus("No files selected for arXiv import")
		return nil
	}
	m.setPersistentStatus(fmt.Sprintf("Fetching arXiv %s for %d file(s)...", id, len(files)))
	return m.fetchArxivMetadata(id, files)
}

func (m *Model) startDetectedArxivQueue(groups map[string][]string) tea.Cmd {
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	batches := make([]arxivBatch, 0, len(ids))
	for _, id := range ids {
		batches = append(batches, arxivBatch{id: id, files: groups[id]})
	}
	if len(batches) == 0 {
		return nil
	}
	m.pendingArxivQueue = append([]arxivBatch{}, batches...)
	return m.nextArxivQueueCmd()
}

func (m *Model) nextArxivQueueCmd() tea.Cmd {
	if len(m.pendingArxivQueue) == 0 {
		m.pendingArxivQueue = nil
		return nil
	}
	next := m.pendingArxivQueue[0]
	m.pendingArxivQueue = m.pendingArxivQueue[1:]
	if len(m.pendingArxivQueue) == 0 {
		m.pendingArxivQueue = nil
	}
	return m.runArxivFetch(next.id, next.files)
}

func parseSearchModeValue(value string) (searchMode, bool) {
	switch searchMode(strings.ToLower(value)) {
	case searchModeTitle:
		return searchModeTitle, true
	case searchModeAuthor:
		return searchModeAuthor, true
	case searchModeYear:
		return searchModeYear, true
	case searchModeContent:
		return searchModeContent, true
	default:
		return "", false
	}
}

func (m *Model) applySearchRoot(req *searchRequest, value string, watchRoot string) error {
	if value == "" {
		return fmt.Errorf("Search root cannot be empty")
	}
	rootPath := value
	if !filepath.IsAbs(rootPath) {
		rootPath = filepath.Join(m.cwd, rootPath)
	}
	rootPath = filepath.Clean(rootPath)
	if resolved := canonicalPath(rootPath); resolved != "" {
		rootPath = resolved
	}
	if watchRoot != "" && !strings.HasPrefix(rootPath, watchRoot) {
		return fmt.Errorf("Search root must stay within the watched directory")
	}
	info, err := os.Stat(rootPath)
	if err != nil {
		return fmt.Errorf("Search root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("Search root is not a directory")
	}
	req.root = rootPath
	return nil
}

func (m *Model) handleSearchResultsKey(key string) (bool, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.exitSearchResults()
		m.setStatus("Search results closed")
		return true, nil
	// case "q", "Q":
	// 	m.exitSearchResults()
	// 	m.setStatus("Search results closed")
	// 	return true, nil
	case "enter":
		m.openSearchResultAtCursor()
		return true, nil
	case "j", "down":
		m.moveSearchCursor(1)
		return true, nil
	case "k", "up":
		m.moveSearchCursor(-1)
		return true, nil
	case "pgdown", "ctrl+f":
		m.pageSearchCursor(1)
		return true, nil
	case "pgup", "ctrl+b":
		m.pageSearchCursor(-1)
		return true, nil
	case "g":
		m.searchResultCursor = 0
		m.ensureSearchResultVisible()
		return true, nil
	case "G":
		if len(m.searchResults) > 0 {
			m.searchResultCursor = len(m.searchResults) - 1
			m.ensureSearchResultVisible()
		}
		return true, nil
	case ":":
		m.clearSearchResults()
		m.state = stateCommand
		m.input.SetValue(":")
		m.input.CursorEnd()
		m.input.Focus()
		m.setPersistentStatus("Command mode (:help for list, Esc to cancel)")
		return true, nil
	case "/":
		m.openSearchPrompt(m.lastSearchQuery)
		return true, nil
	default:
		return true, nil
	}
}

func (m *Model) fetchArxivMetadata(id string, files []string) tea.Cmd {
	store := m.meta
	paths := append([]string{}, files...)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), arxivRequestTimeout)
		defer cancel()

		metadata, err := arxiv.Fetch(ctx, id)
		if err != nil {
			return arxivUpdateMsg{err: err}
		}

		authorStr := strings.Join(metadata.Authors, ", ")
		yearStr := ""
		if metadata.Year > 0 {
			yearStr = strconv.Itoa(metadata.Year)
		}

		baseCtx := context.Background()
		updated := make([]string, 0, len(paths))
		for _, path := range paths {
			existing, err := store.Get(baseCtx, path)
			if err != nil {
				return arxivUpdateMsg{err: fmt.Errorf("load metadata for %s: %w", filepath.Base(path), err)}
			}
			md := meta.Metadata{Path: path}
			if existing != nil {
				md = *existing
			}
			md.Title = metadata.Title
			md.Author = authorStr
			md.Year = yearStr
			if metadata.DOI != "" {
				md.DOI = metadata.DOI
			}
			md.Abstract = metadata.Abstract
			if err := store.Upsert(baseCtx, &md); err != nil {
				return arxivUpdateMsg{err: fmt.Errorf("save metadata for %s: %w", filepath.Base(path), err)}
			}
			updated = append(updated, path)
		}

		return arxivUpdateMsg{arxivID: metadata.ID, updatedPaths: updated}
	}
}

type pathCompletion struct {
	value string
	isDir bool
}

func (m *Model) resolveCommandFilePath(spec string) (string, error) {
	resolved := spec
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(m.cwd, spec)
	}
	resolved = filepath.Clean(resolved)
	if !strings.HasPrefix(resolved, m.root) {
		return "", fmt.Errorf("Path not under root: %s", spec)
	}
	info, err := os.Stat(resolved)
	if err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("Cannot fetch arXiv metadata for directory: %s", spec)
		}
		return resolved, nil
	}
	if filepath.Ext(resolved) != "" {
		return "", fmt.Errorf("File not found: %s", spec)
	}
	dir := filepath.Dir(resolved)
	base := filepath.Base(resolved)
	entries, dirErr := os.ReadDir(dir)
	if dirErr != nil {
		return "", fmt.Errorf("File not found: %s", spec)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if !strings.EqualFold(ext, ".pdf") {
			continue
		}
		if strings.EqualFold(strings.TrimSuffix(name, ext), base) {
			full := filepath.Join(dir, name)
			info, statErr := os.Stat(full)
			if statErr == nil && !info.IsDir() {
				return full, nil
			}
		}
	}
	return "", fmt.Errorf("File not found: %s", spec)
}

func uniquePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func (m *Model) handleCommandAutocomplete() bool {
	if m.state != stateCommand {
		return false
	}
	value := m.input.Value()
	runes := []rune(value)
	cursor := m.input.Position()
	if cursor != len(runes) {
		return false
	}
	current := string(runes[:cursor])
	trimmed := strings.TrimRight(current, " \t")
	if !strings.ContainsAny(trimmed, " \t") {
		return false
	}
	lastSep := strings.LastIndexAny(trimmed, " \t")
	if lastSep == -1 || lastSep == len(trimmed)-1 {
		return false
	}
	token := trimmed[lastSep+1:]
	if token == "" {
		return false
	}
	completions := m.commandPathCompletions(token)
	if len(completions) == 0 {
		m.setStatus("No completions")
		return true
	}
	values := make([]string, len(completions))
	for i, c := range completions {
		values[i] = c.value
	}
	lcp := longestCommonPrefix(values)
	if lcp == token {
		lines := []string{"Completions:"}
		for _, c := range completions {
			lines = append(lines, "  "+c.value)
		}
		m.setCommandOutput(lines)
		m.setPersistentStatus("Multiple completions (type more letters)")
		return true
	}
	appendSpace := false
	if len(completions) == 1 && lcp == completions[0].value && !completions[0].isDir {
		appendSpace = true
	}
	prefix := trimmed[:lastSep+1]
	newValue := prefix + lcp
	if appendSpace {
		newValue += " "
	}
	m.input.SetValue(newValue)
	m.input.CursorEnd()
	return true
}

func (m *Model) commandPathCompletions(token string) []pathCompletion {
	if token == "" {
		return nil
	}
	if filepath.IsAbs(token) {
		return nil
	}
	dirPart, partial := filepath.Split(token)
	searchDir := m.cwd
	origDirPart := dirPart
	if dirPart != "" {
		candidate := filepath.Join(m.cwd, dirPart)
		candidate = filepath.Clean(candidate)
		if !strings.HasPrefix(candidate, m.root) {
			return nil
		}
		info, err := os.Stat(candidate)
		if err != nil || !info.IsDir() {
			return nil
		}
		searchDir = candidate
	}
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil
	}
	sep := string(os.PathSeparator)
	comps := make([]pathCompletion, 0)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, partial) {
			continue
		}
		display := name
		if !e.IsDir() {
			ext := filepath.Ext(name)
			if strings.EqualFold(ext, ".pdf") {
				display = strings.TrimSuffix(name, ext)
			}
			if display == "" {
				display = name
			}
		}
		completion := display
		if origDirPart != "" {
			base := strings.TrimSuffix(origDirPart, sep)
			if base == "" {
				completion = display
			} else {
				completion = filepath.Join(base, display)
			}
		}
		if e.IsDir() && !strings.HasSuffix(completion, sep) {
			completion += sep
		}
		comps = append(comps, pathCompletion{value: completion, isDir: e.IsDir()})
	}
	sort.Slice(comps, func(i, j int) bool {
		return comps[i].value < comps[j].value
	})
	return comps
}

func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	prefix := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(s, prefix) {
			if prefix == "" {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}
