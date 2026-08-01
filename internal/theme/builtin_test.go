package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinLookup(t *testing.T) {
	if _, ok := Builtin("tokyo-night"); !ok {
		t.Fatal("expected tokyo-night to be a built-in theme")
	}
	// Lookup should be case- and separator-insensitive.
	if _, ok := Builtin("Tokyo_Night"); !ok {
		t.Fatal("expected case/underscore-insensitive lookup to match")
	}
	if _, ok := Builtin("nope"); ok {
		t.Fatal("did not expect unknown theme to match")
	}
}

func TestBuiltinThemesAreComplete(t *testing.T) {
	for _, name := range BuiltinNames() {
		th, ok := Builtin(name)
		if !ok {
			t.Fatalf("BuiltinNames returned %q but Builtin() missed it", name)
		}
		if strings.TrimSpace(th.Meta.Name) == "" {
			t.Errorf("%s: missing display name", name)
		}
		p := th.Palette
		for field, val := range map[string]string{
			"bg": p.BG, "fg": p.FG, "accent": p.Accent, "selection": p.Selection,
		} {
			if !strings.HasPrefix(val, "#") {
				t.Errorf("%s: palette.%s = %q, expected hex color", name, field, val)
			}
		}
		if strings.TrimSpace(th.Components.StatusBar.BG) == "" {
			t.Errorf("%s: status bar has no background", name)
		}
		icons := th.IconSet()
		if icons.ToRead != "☐" || icons.Unread != "○" || icons.Reading != "◐" || icons.Read != "✓" {
			t.Errorf("%s: inconsistent status icons: %#v", name, icons)
		}
		selected := th.Components.ListSelected
		if selected.BG != p.Selection {
			t.Errorf("%s: selection row has no highlight background", name)
		}
	}
}

func TestBuiltinNamesSorted(t *testing.T) {
	names := BuiltinNames()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("BuiltinNames not sorted: %v", names)
		}
	}
}

func TestBuiltinThemeTextContrast(t *testing.T) {
	for _, name := range BuiltinNames() {
		th, _ := Builtin(name)
		assertThemeTextContrast(t, name, th)
	}
}

func TestShippedThemeTextContrast(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "themes", "*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		th, err := LoadFrom(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		assertThemeTextContrast(t, filepath.Base(path), th)
	}
}

func assertThemeTextContrast(t *testing.T, name string, th Theme) {
	t.Helper()
	p := th.Palette
	pairs := []struct {
		label  string
		fg, bg string
	}{
		{"body", th.Components.ListBody.FG, p.BG},
		{"muted tree info", th.Components.TreeInfo.FG, p.BG},
		{"tree header", th.Components.TreeHeader.FG, th.Components.TreeHeader.BG},
		{"list header", th.Components.ListHeader.FG, th.Components.ListHeader.BG},
		{"preview header", th.Components.PreviewHeader.FG, th.Components.PreviewHeader.BG},
		{"selected row", th.Components.ListSelected.FG, th.Components.ListSelected.BG},
		{"cursor row", th.Components.ListCursor.FG, th.Components.ListCursor.BG},
		{"cursor on selected row", th.Components.ListCursorSelect.FG, th.Components.ListCursorSelect.BG},
		{"status text", th.Components.StatusBar.FG, th.Components.StatusBar.BG},
		{"status label", th.Components.StatusLabel.FG, th.Components.StatusBar.BG},
		{"status value", th.Components.StatusValue.FG, th.Components.StatusBar.BG},
		{"prompt label", th.Components.PromptLabel.FG, th.Components.PromptLabel.BG},
	}
	for _, pair := range pairs {
		if ratio := contrastRatio(pair.fg, pair.bg); ratio < 4.5 {
			t.Errorf("%s %s contrast %.2f (%s on %s), want >= 4.5", name, pair.label, ratio, pair.fg, pair.bg)
		}
	}
	if th.Components.ListSelected.BG == th.Components.ListCursorSelect.BG {
		t.Errorf("%s selected and cursor-selected rows use the same background %s", name, th.Components.ListSelected.BG)
	}
}

func TestLoadFromUpgradesLegacyGeneratedDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.toml")
	legacy := `[meta]
name = "Gorae Deep"
version = 1

[palette]
bg = "#0a0f14"
selection = "#5eead4"

[icons]
toread = "•"
reading = "▶"
selected = "✔"

[components.list_selected]
fg = "#5eead4"
bold = true
`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	th, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if th.Icons.ToRead != "☐" || th.Icons.Reading != "◐" {
		t.Fatalf("legacy icons were not upgraded: %#v", th.Icons)
	}
	selected := th.Components.ListSelected
	if selected.FG != th.Palette.BG || selected.BG != th.Palette.Selection {
		t.Fatalf("legacy selection style was not upgraded: %#v", selected)
	}
}
