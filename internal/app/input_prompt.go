package app

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// inputPromptConfig describes one of the modal text-input states (new dir,
// rename, command, search prompt, arxiv prompt). handleInputPrompt centralizes
// the boilerplate these states share — intercepting pre-input keys, feeding the
// key to the textinput, and the esc/cancel path — while onSubmit carries each
// prompt's unique Enter handling. The Enter path intentionally does NOT reset
// the prompt; onSubmit owns its own state transitions because they differ per
// prompt (e.g. the arxiv prompt stays open on an empty ID).
type inputPromptConfig struct {
	quitCancels bool                                       // treat "q" as cancel, in addition to "esc"
	blurOnExit  bool                                       // blur the textinput on cancel
	cancelMsg   string                                     // status shown on cancel
	onCancel    func(m *Model)                             // extra cleanup on cancel (may be nil)
	preKey      func(m *Model, key string) (bool, tea.Cmd) // intercept keys before the textinput (may be nil)
	onSubmit    func(m *Model, raw string) tea.Cmd         // handle Enter; returns extra cmd (may be nil)
}

// handleInputPrompt runs the shared update logic for a modal text-input state.
func (m Model) handleInputPrompt(msg tea.KeyMsg, key string, cfg inputPromptConfig) (tea.Model, tea.Cmd) {
	if cfg.preKey != nil {
		if handled, cmd := cfg.preKey(&m, key); handled {
			return m, cmd
		}
	}

	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)

	switch key {
	case "enter":
		var cmd tea.Cmd
		if cfg.onSubmit != nil {
			cmd = cfg.onSubmit(&m, m.input.Value())
		}
		return m, tea.Batch(inputCmd, cmd)
	case "esc":
		m.cancelInputPrompt(cfg)
		return m, inputCmd
	case "q":
		if cfg.quitCancels {
			m.cancelInputPrompt(cfg)
			return m, inputCmd
		}
	}
	return m, inputCmd
}

// cancelInputPrompt performs the common reset shared by every prompt's cancel
// path, then runs any prompt-specific cleanup and sets the cancel status.
func (m *Model) cancelInputPrompt(cfg inputPromptConfig) {
	m.state = stateNormal
	m.input.SetValue("")
	if cfg.blurOnExit {
		m.input.Blur()
	}
	if cfg.onCancel != nil {
		cfg.onCancel(m)
	}
	if cfg.cancelMsg != "" {
		m.setStatus(cfg.cancelMsg)
	}
}

// ── per-prompt submit handlers ────────────────────────────────────────────────

func (m *Model) submitNewDir(raw string) tea.Cmd {
	name := strings.TrimSpace(raw)
	m.state = stateNormal
	m.input.SetValue("")

	if name == "" {
		m.setStatus("Directory name cannot be empty")
		return nil
	}
	if strings.HasPrefix(name, ".") {
		m.setStatus("Dot directories are hidden; choose another name")
		return nil
	}

	dst := filepath.Join(m.cwd, name)
	if _, err := os.Stat(dst); err == nil {
		m.setStatus("Already exists")
		return nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		m.setStatus("Failed: " + err.Error())
		return nil
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
	return nil
}

func (m *Model) submitRename(raw string) tea.Cmd {
	newName := strings.TrimSpace(raw)
	oldPath := m.renameTarget

	m.state = stateNormal
	m.input.SetValue("")
	m.renameTarget = ""

	if newName == "" {
		m.setStatus("Name cannot be empty")
		return nil
	}
	if strings.Contains(newName, "/") {
		m.setStatus("Name cannot contain '/'")
		return nil
	}

	dir := filepath.Dir(oldPath)
	newPath := filepath.Join(dir, newName)

	if _, err := os.Stat(newPath); err == nil {
		m.setStatus("Target already exists")
		return nil
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		m.setStatus("Rename failed: " + err.Error())
		return nil
	}

	var metaErr error
	var recentErr error
	if err := m.moveMetadataPaths(oldPath, newPath, true); err != nil {
		metaErr = err
	}
	if err := m.syncRecentlyOpenedDirectory(); err != nil {
		recentErr = err
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
	} else if recentErr != nil {
		m.setStatus("Renamed, but recently read update failed: " + recentErr.Error())
	} else {
		m.setStatus("Renamed")
	}
	return nil
}

// commandPromptPreKey handles command-history recall and autocomplete, which
// must run before the key reaches the textinput.
func (m *Model) commandPromptPreKey(key string) (bool, tea.Cmd) {
	switch key {
	case "tab":
		if m.handleCommandAutocomplete() {
			return true, nil
		}
	case "up":
		if m.recallPreviousCommand() {
			return true, nil
		}
	case "down":
		if m.recallNextCommand() {
			return true, nil
		}
	}
	return false, nil
}

func (m *Model) submitCommand(raw string) tea.Cmd {
	line := raw // command lines are intentionally not trimmed
	m.state = stateNormal
	m.input.SetValue("")
	m.input.Blur()
	m.rememberCommand(line)
	return m.runCommand(line)
}

func (m *Model) submitSearch(raw string) tea.Cmd {
	line := strings.TrimSpace(raw)
	m.input.SetValue("")
	m.input.Blur()
	m.state = stateNormal

	if line == "" {
		m.setStatus("Search query cannot be empty")
		return nil
	}

	tokens, err := splitCommandLine(line)
	if err != nil {
		m.setStatus("Search parse failed: " + err.Error())
		return nil
	}
	req, err := m.buildSearchRequest(tokens)
	if err != nil {
		m.setStatus(err.Error())
		return nil
	}
	return m.runSearch(req)
}

func (m *Model) submitArxiv(raw string) tea.Cmd {
	id := strings.TrimSpace(raw)
	if id == "" {
		m.setStatus("arXiv ID cannot be empty")
		return nil // stays in stateArxivPrompt
	}
	target := strings.TrimSpace(m.pendingArxivActive)
	if target == "" {
		m.setStatus("No file selected for arXiv import")
		m.state = stateNormal
		m.input.SetValue("")
		m.input.Blur()
		m.pendingArxivFiles = nil
		return nil
	}
	m.pendingArxivActive = ""
	m.state = stateNormal
	m.input.SetValue("")
	m.input.Blur()
	return m.runArxivFetch(id, []string{target})
}
