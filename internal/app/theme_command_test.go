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
	if !m.autocompleteTheme(":theme tok", false) {
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
	if !m.autocompleteTheme(":theme catp", false) {
		t.Fatal("expected autocomplete to handle the token")
	}
	if got, want := m.input.Value(), ":theme catppuccin-"; got != want {
		t.Fatalf("input = %q, want %q", got, want)
	}
}

func TestAutocompleteThemeEmptyListsAll(t *testing.T) {
	// ":theme " starts the selectable candidate list.
	m := newThemeTestModel(":theme ")
	if !m.autocompleteTheme(":theme", true) {
		t.Fatal("expected autocomplete to handle the empty token")
	}
	if len(m.themeCompletionCandidates) == 0 {
		t.Fatal("expected selectable theme candidates")
	}
	first := themeCompletionCandidates()[0]
	if got, want := m.input.Value(), ":theme "+first; got != want {
		t.Fatalf("input = %q, want %q", got, want)
	}
	if !m.autocompleteTheme(m.input.Value(), false) {
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
	m.autocompleteTheme(":theme", true)
	after := strings.Count(m.View(), "\n")
	if after != before {
		t.Fatalf("theme chooser changed frame height from %d to %d lines", before, after)
	}
}

func TestThemeCompletionResetsWhenTyping(t *testing.T) {
	m := newThemeTestModel(":theme ")
	m.autocompleteTheme(":theme", true)
	m.commandPromptPreKey("x")
	if len(m.themeCompletionCandidates) != 0 {
		t.Fatal("expected typing to reset the theme completion cycle")
	}
}

func TestAutocompleteThemeStopsAfterArg(t *testing.T) {
	// A completed argument means there is nothing left to complete.
	m := newThemeTestModel(":theme dracula ")
	if m.autocompleteTheme(":theme dracula", true) {
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
