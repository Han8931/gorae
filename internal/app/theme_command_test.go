package app

import (
	"strings"
	"testing"

	textinput "github.com/charmbracelet/bubbles/textinput"
)

func newThemeTestModel(value string) *Model {
	ti := textinput.New()
	ti.SetValue(value)
	ti.CursorEnd()
	return &Model{input: ti, state: stateCommand}
}

func TestFirstCommandToken(t *testing.T) {
	cases := map[string]string{
		":theme tokyo": "theme",
		"theme":        "theme",
		"  :Theme  ":   "theme",
		":config show": "config",
		"":             "",
	}
	for in, want := range cases {
		if got := firstCommandToken(in); got != want {
			t.Errorf("firstCommandToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAutocompleteThemeUniquePrefix(t *testing.T) {
	// "tok" uniquely matches "tokyo-night".
	m := newThemeTestModel(":theme tok")
	if !m.autocompleteThemeDirection(":theme tok", false, 1) {
		t.Fatal("expected autocomplete to handle the token")
	}
	if got, want := m.input.Value(), ":theme tokyo-night "; got != want {
		t.Fatalf("input = %q, want %q", got, want)
	}
}

func TestAutocompleteThemeSharedPrefix(t *testing.T) {
	// "catppuccin-" matches two themes; completion should extend to the common
	// prefix without picking one.
	m := newThemeTestModel(":theme catp")
	if !m.autocompleteThemeDirection(":theme catp", false, 1) {
		t.Fatal("expected autocomplete to handle the token")
	}
	if got, want := m.input.Value(), ":theme catppuccin-"; got != want {
		t.Fatalf("input = %q, want %q", got, want)
	}
}

func TestAutocompleteThemeEmptyListsAll(t *testing.T) {
	// ":theme " starts the selectable candidate list.
	m := newThemeTestModel(":theme ")
	if !m.autocompleteThemeDirection(":theme", true, 1) {
		t.Fatal("expected autocomplete to handle the empty token")
	}
	if len(m.completionCandidates) == 0 {
		t.Fatal("expected selectable theme candidates")
	}
	first := themeCompletionCandidates()[0]
	if got, want := m.input.Value(), ":theme "+first; got != want {
		t.Fatalf("input = %q, want %q", got, want)
	}
	// Repeated Tab goes through the shared command-line completion path, which
	// is where an open cycle is advanced regardless of what opened it.
	if handled, _ := m.commandPromptPreKey("tab"); !handled {
		t.Fatal("expected repeated Tab to advance the selection")
	}
	second := themeCompletionCandidates()[1]
	if got, want := m.input.Value(), ":theme "+second; got != want {
		t.Fatalf("input after second Tab = %q, want %q", got, want)
	}
}

func TestThemeCompletionDoesNotIncreaseFrameHeight(t *testing.T) {
	m := newThemeTestModel(":theme ")
	m.width = 120
	m.viewportHeight = 18
	m.cwd = "/tmp"
	before := strings.Count(m.View(), "\n")
	m.autocompleteThemeDirection(":theme", true, 1)
	after := strings.Count(m.View(), "\n")
	if after != before {
		t.Fatalf("theme chooser changed frame height from %d to %d lines", before, after)
	}
	chooser := strings.Join(m.renderCompletionPanel(40, 18), "\n")
	if strings.Contains(chooser, "▸") {
		t.Fatal("theme chooser should use row highlighting without a wedge marker")
	}
}

func TestThemeCompletionResetsWhenTyping(t *testing.T) {
	m := newThemeTestModel(":theme ")
	m.autocompleteThemeDirection(":theme", true, 1)
	m.commandPromptPreKey("x")
	if len(m.completionCandidates) != 0 {
		t.Fatal("expected typing to reset the theme completion cycle")
	}
}

func TestThemeCompletionShiftTabMovesBackward(t *testing.T) {
	m := newThemeTestModel(":theme ")
	if handled, _ := m.commandPromptPreKey("shift+tab"); !handled {
		t.Fatal("expected Shift+Tab to open the theme chooser")
	}
	candidates := themeCompletionCandidates()
	if got, want := m.input.Value(), ":theme "+candidates[len(candidates)-1]; got != want {
		t.Fatalf("initial Shift+Tab = %q, want %q", got, want)
	}
	if handled, _ := m.commandPromptPreKey("shift+tab"); !handled {
		t.Fatal("expected repeated Shift+Tab to move backward")
	}
	if got, want := m.input.Value(), ":theme "+candidates[len(candidates)-2]; got != want {
		t.Fatalf("second Shift+Tab = %q, want %q", got, want)
	}
}

func TestAutocompleteThemeStopsAfterArg(t *testing.T) {
	// A completed argument means there is nothing left to complete.
	m := newThemeTestModel(":theme dracula ")
	if m.autocompleteThemeDirection(":theme dracula", true, 1) {
		t.Fatal("expected no further completion after a full argument")
	}
}

func TestApplyBuiltinThemeUpdatesModel(t *testing.T) {
	m := newThemeTestModel(":theme dracula")
	m.cfg = nil // avoid touching disk via config.Save
	m.applyBuiltinTheme("dracula")
	if got := m.theme.Meta.Name; got != "Dracula" {
		t.Fatalf("active theme = %q, want Dracula", got)
	}
}
