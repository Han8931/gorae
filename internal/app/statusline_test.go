package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestTextOnPicksReadableContrast(t *testing.T) {
	if got := textOn("#fbbf24"); got != "#0d1117" {
		t.Fatalf("textOn(light) = %q, want dark text", got)
	}
	if got := textOn("#1e1e2e"); got != "#ffffff" {
		t.Fatalf("textOn(dark) = %q, want white text", got)
	}
	if got := textOn("not-a-colour"); got != "" {
		t.Fatalf("textOn(invalid) = %q, want empty", got)
	}
}

func TestBlendHex(t *testing.T) {
	if got := blendHex("#000000", "#ffffff", 0.5); got != "#808080" {
		t.Fatalf("blendHex midpoint = %q", got)
	}
	if got := blendHex("#123456", "", 0.5); got != "#123456" {
		t.Fatalf("blendHex with bad input should return a unchanged, got %q", got)
	}
}

func TestIsErrorStatus(t *testing.T) {
	for _, msg := range []string{"Delete failed: boom", "BibTeX copy failed", "Invalid DOI"} {
		if !isErrorStatus(msg) {
			t.Errorf("expected %q to be an error", msg)
		}
	}
	for _, msg := range []string{"Deleted 2 item(s).", "Sorting by title", "Ready"} {
		if isErrorStatus(msg) {
			t.Errorf("expected %q not to be an error", msg)
		}
	}
}

func TestHintBarFitsWidth(t *testing.T) {
	m := Model{width: 40}
	line := m.renderHintBar(m.browserHints(), 40)
	if w := lipgloss.Width(line); w != 40 {
		t.Fatalf("hint bar width = %d, want 40", w)
	}
	if !strings.Contains(line, "move") {
		t.Fatalf("expected first hint to be shown, got %q", line)
	}
}

func TestFooterShowsPendingPrefix(t *testing.T) {
	m := Model{width: 100, awaitingSort: true}
	footer := m.renderBrowserFooter("")
	if !strings.Contains(footer, "title") || !strings.Contains(footer, "year") {
		t.Fatalf("expected sort continuations in footer, got %q", footer)
	}
}

func TestModeLabelCoversFullScreenStates(t *testing.T) {
	for _, st := range []uiState{stateLaunch, stateGorae, stateHelp, stateSessionList, stateConfirmDelete} {
		if got := (Model{state: st}).currentModeLabel(); got == "Normal" {
			t.Errorf("state %d reported as Normal", st)
		}
	}
}

func TestDeleteConfirmIgnoresEnterAndStrayKeys(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Model{state: stateConfirmDelete, confirmItems: []string{target}, cwd: dir}

	for _, k := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyRunes, Runes: []rune("j")}} {
		next, _ := m.Update(k)
		m = next.(Model)
		if m.state != stateConfirmDelete {
			t.Fatalf("key %q left the confirmation", k.String())
		}
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("key %q deleted the file", k.String())
		}
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if next.(Model).state != stateNormal {
		t.Fatal("esc should cancel the confirmation")
	}
}

func TestTruncateLeftKeepsTail(t *testing.T) {
	if got := truncateLeft("/a/very/long/path/Papers", 10); !strings.HasSuffix(got, "Papers") || lipgloss.Width(got) > 10 {
		t.Fatalf("truncateLeft = %q, want tail within 10 cols", got)
	}
	if got := truncateLeft("short", 10); got != "short" {
		t.Fatalf("truncateLeft(short) = %q", got)
	}
}
