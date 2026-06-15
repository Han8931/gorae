package app

import (
	"os"
	"path/filepath"
	"testing"

	textinput "github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// These tests characterize the behavior of the five modal text-input states
// (new dir, rename, command, search prompt, arxiv prompt) as they were before
// the shared handleInputPrompt refactor, so the refactor can be verified to
// preserve behavior exactly.

func newPromptModel(t *testing.T, st uiState, value string) Model {
	t.Helper()
	ti := textinput.New()
	ti.Focus()
	ti.SetValue(value)
	m := Model{
		state:          st,
		input:          ti,
		viewportHeight: 20,
		width:          80,
	}
	return m
}

func sendKey(t *testing.T, m Model, k tea.KeyType) Model {
	t.Helper()
	updated, _ := m.Update(tea.KeyMsg{Type: k})
	out, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	return out
}

func TestNewDirPromptCreatesDirectoryOnEnter(t *testing.T) {
	root := t.TempDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root: %v", err)
	}
	m := newPromptModel(t, stateNewDir, "newfolder")
	m.root = root
	m.cwd = root
	m.entries = entries

	out := sendKey(t, m, tea.KeyEnter)

	if out.state != stateNormal {
		t.Fatalf("expected stateNormal after create, got %v", out.state)
	}
	if out.status != "Directory created" {
		t.Fatalf("expected 'Directory created' status, got %q", out.status)
	}
	if info, err := os.Stat(filepath.Join(root, "newfolder")); err != nil || !info.IsDir() {
		t.Fatalf("expected newfolder directory to exist, err=%v", err)
	}
}

func TestNewDirPromptRejectsEmptyName(t *testing.T) {
	m := newPromptModel(t, stateNewDir, "   ")
	m.cwd = t.TempDir()

	out := sendKey(t, m, tea.KeyEnter)

	if out.state != stateNormal {
		t.Fatalf("expected stateNormal, got %v", out.state)
	}
	if out.status != "Directory name cannot be empty" {
		t.Fatalf("unexpected status: %q", out.status)
	}
}

func TestNewDirPromptEscCancels(t *testing.T) {
	m := newPromptModel(t, stateNewDir, "whatever")
	m.cwd = t.TempDir()

	out := sendKey(t, m, tea.KeyEsc)

	if out.state != stateNormal {
		t.Fatalf("expected stateNormal, got %v", out.state)
	}
	if out.status != "Cancelled" {
		t.Fatalf("expected 'Cancelled', got %q", out.status)
	}
	if out.input.Value() != "" {
		t.Fatalf("expected input cleared, got %q", out.input.Value())
	}
}

func TestSearchPromptRejectsEmptyQuery(t *testing.T) {
	m := newPromptModel(t, stateSearchPrompt, "")

	out := sendKey(t, m, tea.KeyEnter)

	if out.state != stateNormal {
		t.Fatalf("expected stateNormal, got %v", out.state)
	}
	if out.status != "Search query cannot be empty" {
		t.Fatalf("unexpected status: %q", out.status)
	}
}

func TestSearchPromptEscCancels(t *testing.T) {
	m := newPromptModel(t, stateSearchPrompt, "transformer")

	out := sendKey(t, m, tea.KeyEsc)

	if out.state != stateNormal {
		t.Fatalf("expected stateNormal, got %v", out.state)
	}
	if out.status != "Search cancelled" {
		t.Fatalf("unexpected status: %q", out.status)
	}
}

// arXiv prompt intentionally stays open (does not reset to normal) when the
// submitted ID is empty — unlike the new-dir prompt.
func TestArxivPromptEmptyIDStaysOpen(t *testing.T) {
	m := newPromptModel(t, stateArxivPrompt, "")
	m.pendingArxivActive = "/tmp/paper.pdf"

	out := sendKey(t, m, tea.KeyEnter)

	if out.state != stateArxivPrompt {
		t.Fatalf("expected to stay in stateArxivPrompt, got %v", out.state)
	}
	if out.status != "arXiv ID cannot be empty" {
		t.Fatalf("unexpected status: %q", out.status)
	}
}

func TestArxivPromptEscClearsPendingState(t *testing.T) {
	m := newPromptModel(t, stateArxivPrompt, "1234.5678")
	m.pendingArxivActive = "/tmp/paper.pdf"
	m.pendingArxivFiles = []string{"/tmp/paper.pdf"}

	out := sendKey(t, m, tea.KeyEsc)

	if out.state != stateNormal {
		t.Fatalf("expected stateNormal, got %v", out.state)
	}
	if out.status != "arXiv command cancelled" {
		t.Fatalf("unexpected status: %q", out.status)
	}
	if out.pendingArxivActive != "" || out.pendingArxivFiles != nil {
		t.Fatalf("expected pending arxiv state cleared, got active=%q files=%v",
			out.pendingArxivActive, out.pendingArxivFiles)
	}
}

func TestCommandPromptEscCancels(t *testing.T) {
	m := newPromptModel(t, stateCommand, ":index")

	out := sendKey(t, m, tea.KeyEsc)

	if out.state != stateNormal {
		t.Fatalf("expected stateNormal, got %v", out.state)
	}
	if out.status != "Command cancelled" {
		t.Fatalf("unexpected status: %q", out.status)
	}
	if out.input.Focused() {
		t.Fatalf("expected command input to be blurred after cancel")
	}
}
