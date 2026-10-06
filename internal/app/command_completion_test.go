package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	textinput "github.com/charmbracelet/bubbles/textinput"

	"github.com/Han8931/gorae/internal/theme"
)

// newCompletionTestModel returns a command-mode model whose input already holds
// value, with the cursor at the end as it would be after typing.
func newCompletionTestModel(t *testing.T, value string) *Model {
	t.Helper()
	ti := textinput.New()
	ti.SetValue(value)
	ti.CursorEnd()
	return &Model{input: ti, state: stateCommand}
}

func tab(t *testing.T, m *Model) {
	t.Helper()
	if handled, _ := m.commandPromptPreKey("tab"); !handled {
		t.Fatalf("Tab was not handled (input %q)", m.input.Value())
	}
}

func shiftTab(t *testing.T, m *Model) {
	t.Helper()
	if handled, _ := m.commandPromptPreKey("shift+tab"); !handled {
		t.Fatalf("Shift+Tab was not handled (input %q)", m.input.Value())
	}
}

// A prefix shared by several commands cannot be extended, so Tab walks the
// candidates instead of leaving the line untouched.
func TestCommandNameTabCyclesAmbiguousPrefix(t *testing.T) {
	var matches []string
	seen := map[string]bool{}
	for _, name := range commandNames {
		if strings.HasPrefix(name, "t") && !seen[name] {
			seen[name] = true
			matches = append(matches, name)
		}
	}
	if len(matches) < 2 {
		t.Skipf("need at least two commands starting with t, got %v", matches)
	}

	m := newCompletionTestModel(t, "t")
	tab(t, m)
	if got, want := m.input.Value(), matches[0]; got != want {
		t.Fatalf("first Tab = %q, want %q", got, want)
	}
	tab(t, m)
	if got, want := m.input.Value(), matches[1]; got != want {
		t.Fatalf("second Tab = %q, want %q", got, want)
	}
	shiftTab(t, m)
	if got, want := m.input.Value(), matches[0]; got != want {
		t.Fatalf("Shift+Tab = %q, want %q", got, want)
	}
	// Wrapping backwards past the first lands on the last.
	shiftTab(t, m)
	if got, want := m.input.Value(), matches[len(matches)-1]; got != want {
		t.Fatalf("Shift+Tab wrap = %q, want %q", got, want)
	}
}

// Tab on an empty command line offers every command rather than doing nothing.
func TestEmptyCommandLineTabOffersEveryCommand(t *testing.T) {
	m := newCompletionTestModel(t, "")
	tab(t, m)
	if len(m.completionCandidates) == 0 {
		t.Fatal("expected Tab on an empty line to open the command list")
	}
	if m.completionTitle != "Commands" {
		t.Fatalf("completion title = %q, want Commands", m.completionTitle)
	}
	if m.input.Value() != m.completionCandidates[0] {
		t.Fatalf("input = %q, want the first candidate %q", m.input.Value(), m.completionCandidates[0])
	}
}

// A prefix matching exactly one command still completes outright, with the
// trailing space that lets the user type an argument straight away.
func TestCommandNameTabCompletesUniquePrefix(t *testing.T) {
	m := newCompletionTestModel(t, "the")
	tab(t, m)
	if got, want := m.input.Value(), "theme "; got != want {
		t.Fatalf("input = %q, want %q", got, want)
	}
	if len(m.completionCandidates) != 0 {
		t.Fatal("a unique completion should not open a cycle")
	}
}

func TestCommandCompletionCandidatesAreUnique(t *testing.T) {
	m := newCompletionTestModel(t, "")
	tab(t, m)
	seen := map[string]bool{}
	for _, c := range m.completionCandidates {
		if seen[c] {
			t.Fatalf("candidate %q offered twice", c)
		}
		seen[c] = true
	}
}

// Typing invalidates the cycle, so the next Tab completes the new text.
func TestCompletionCycleResetsWhenTyping(t *testing.T) {
	m := newCompletionTestModel(t, "t")
	tab(t, m)
	if len(m.completionCandidates) == 0 {
		t.Fatal("expected a cycle to be open")
	}
	m.commandPromptPreKey("x")
	if len(m.completionCandidates) != 0 {
		t.Fatal("expected typing to close the cycle")
	}
}

// Path arguments cycle on the same key when the common prefix is exhausted.
func TestPathArgumentTabCycles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"notes-a.md", "notes-b.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m := newCompletionTestModel(t, "open notes-")
	m.cwd = dir
	m.root = dir

	tab(t, m)
	if got, want := m.input.Value(), "open notes-a.md"; got != want {
		t.Fatalf("first Tab = %q, want %q", got, want)
	}
	if m.completionTitle != "Paths" {
		t.Fatalf("completion title = %q, want Paths", m.completionTitle)
	}
	tab(t, m)
	if got, want := m.input.Value(), "open notes-b.md"; got != want {
		t.Fatalf("second Tab = %q, want %q", got, want)
	}
}

// The panel keeps the selected candidate on screen even when the list is longer
// than the panel is tall, and stays within the height it was given.
func TestCompletionPanelWindowsAroundSelection(t *testing.T) {
	m := newCompletionTestModel(t, "")
	m.styles = newViewStyles(theme.Default())
	candidates := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		candidates = append(candidates, "candidate-"+string(rune('a'+i%26))+string(rune('0'+i/26)))
	}
	m.completionCandidates = candidates
	m.completionTitle = "Commands"

	for _, idx := range []int{0, 20, 39} {
		m.completionIndex = idx
		lines := m.renderCompletionPanel(40, 10)
		if len(lines) != 10 {
			t.Fatalf("index %d: panel is %d lines, want 10", idx, len(lines))
		}
		if !strings.Contains(strings.Join(lines, "\n"), candidates[idx]) {
			t.Fatalf("index %d: selected candidate %q is not on screen", idx, candidates[idx])
		}
	}
}
