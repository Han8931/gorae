package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"pdf-tui/internal/config"
	"pdf-tui/internal/meta"
)

type configEditFinishedMsg struct {
	err error
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.viewportHeight = msg.Height - 5
		if m.viewportHeight < 1 {
			m.viewportHeight = 1
		}
		m.width = msg.Width
		m.ensureCursorVisible()
		return m, nil

	case configEditFinishedMsg:
		if msg.err != nil {
			m.setStatus("Config edit failed: " + msg.err.Error())
		} else {
			m.setStatus("Config edit finished")
		}
		return m, nil

	case tea.KeyMsg:
		key := msg.String()

		if m.state != stateCommand && len(m.commandOutput) > 0 {
			m.clearCommandOutput()
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

			case "esc":
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
				m.loadMetaFieldIntoInput()
				m.input.Focus()
				m.setPersistentStatus(metaEditStatus(m.metaFieldIndex))
				return m, nil
			case "esc":
				m.state = stateNormal
				m.metaEditingPath = ""
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
						m.updateCurrentMetadata(m.metaDraft.Path, false)
						m.updateTextPreview()
						m.refreshEntryTitles()
					}
				} else {
					m.setStatus("Metadata store not available")
				}
				m.state = stateNormal
				m.input.SetValue("")
				m.metaEditingPath = ""
				return m, cmd

			case "esc":
				m.state = stateNormal
				m.input.SetValue("")
				m.metaEditingPath = ""
				m.setStatus("Metadata edit cancelled")
				return m, cmd
			}

			return m, cmd
		}

		// ===========================
		//  COMMAND MODE
		// ===========================
		if m.state == stateCommand {
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
				_ = exec.Command("zathura", full).Start()
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

			// For now: only files (skip dirs)
			if entry.IsDir() {
				m.setStatus("Metadata editing is for files only")
				return m, nil
			}

			m.state = stateMetaPreview
			m.metaEditingPath = full

			// load existing metadata if present
			draft := meta.Metadata{Path: full}
			if m.meta != nil {
				ctx := context.Background()
				existing, err := m.meta.Get(ctx, full)
				if err != nil {
					m.setStatus("Failed to load metadata: " + err.Error())
				} else if existing != nil {
					draft = *existing
				}
			}
			m.metaDraft = draft
			m.metaFieldIndex = 0
			m.input.SetValue("")
			m.input.Blur()
			m.setPersistentStatus("Metadata preview: press 'e' again to edit (Esc to cancel)")
			return m, nil

		case ":":
			m.state = stateCommand
			m.input.SetValue(":")
			m.input.CursorEnd()
			m.input.Focus()
			m.setPersistentStatus("Command mode (:help for list, Esc to cancel)")
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
			"  Metadata   : e preview/edit metadata",
			"  Config     : :config shows/edits the config file",
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
	case "config":
		return m.handleConfigCommand(args)
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
		path, err := config.Path()
		if err != nil {
			m.setStatus("Failed to resolve config path: " + err.Error())
			return nil
		}
		lines := []string{
			"Config file:",
			"  " + path,
			"Use :config edit to open it in your $EDITOR.",
		}
		m.setCommandOutput(lines)
		m.setPersistentStatus("Config path displayed (use :clear to hide)")
		return nil
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "edit":
		path, err := config.Path()
		if err != nil {
			m.setStatus("Failed to resolve config path: " + err.Error())
			return nil
		}
		editor := strings.TrimSpace(os.Getenv("EDITOR"))
		if editor == "" {
			editor = "vi"
		}
		cmd := exec.Command(editor, path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		m.setPersistentStatus(fmt.Sprintf("Editing config with %s (exit editor to return)", editor))
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			return configEditFinishedMsg{err: err}
		})
	default:
		m.setStatus(fmt.Sprintf("Unknown config command: %s", sub))
		return nil
	}
}
