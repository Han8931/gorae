package theme

import (
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
