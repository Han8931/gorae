package theme

import "sort"

// builtinSpec describes the small set of colors needed to derive a full theme.
// The component styles are generated consistently from the palette so every
// built-in theme covers the whole UI, light or dark.
type builtinSpec struct {
	key      string
	name     string
	border   string // separators / border glyphs
	headerBG string // panel header + overlay background
	statusBG string // status bar background
	palette  Palette
}

// buildTheme expands a builtinSpec into a complete Theme. The mapping mirrors
// Default() so custom on-disk themes and built-ins render identically.
func buildTheme(s builtinSpec) Theme {
	p := s.palette
	selectionFG := readableForeground(p, p.Selection)
	cursorFG := readableForeground(p, p.Warning)
	promptFG := readableForeground(p, p.Accent)
	return Theme{
		Meta:    Meta{Name: s.name, Version: 1},
		Palette: p,
		Borders: Borders{Style: "rounded", Color: s.border},
		Icons:   Icons{Mode: "unicode"},
		Components: ComponentStyles{
			AppHeader:  StyleSpec{FG: p.FG, BG: p.BG, Bold: true},
			TreeHeader: StyleSpec{FG: p.Success, BG: s.headerBG, Bold: true},
			TreeBody:   StyleSpec{FG: p.FG},
			TreeActive: StyleSpec{FG: p.Warning, Bold: true},
			TreeInfo:   StyleSpec{FG: p.Muted, Italic: true},

			ListHeader:       StyleSpec{FG: p.FG, BG: s.headerBG, Bold: true},
			ListBody:         StyleSpec{FG: p.FG},
			ListSelected:     StyleSpec{FG: selectionFG, BG: p.Selection, Bold: true},
			ListCursor:       StyleSpec{FG: cursorFG, BG: p.Warning, Bold: true},
			ListCursorSelect: StyleSpec{FG: cursorFG, BG: p.Warning, Bold: true},

			PreviewHeader: StyleSpec{FG: p.Accent, BG: s.headerBG, Bold: true},
			PreviewBody:   StyleSpec{FG: p.FG},
			PreviewInfo:   StyleSpec{FG: p.Accent, Bold: true},

			Separator:   StyleSpec{FG: s.border},
			StatusBar:   StyleSpec{FG: p.FG, BG: s.statusBG},
			StatusLabel: StyleSpec{FG: p.Selection, Bold: true},
			StatusValue: StyleSpec{FG: p.Warning},
			PromptLabel: StyleSpec{FG: promptFG, BG: p.Accent, Bold: true},
			PromptValue: StyleSpec{FG: p.FG, BG: p.BG},
			MetaOverlay: StyleSpec{FG: p.FG, BG: s.headerBG},
			Markdown: MarkdownStyle{
				H1:         StyleSpec{FG: p.Accent, Bold: true},
				H2:         StyleSpec{FG: p.Success, Bold: true},
				H3:         StyleSpec{FG: p.Warning, Bold: true},
				Code:       StyleSpec{FG: p.Warning},
				CodeBlock:  StyleSpec{FG: p.FG},
				Blockquote: StyleSpec{FG: p.Muted, Italic: true},
				Link:       StyleSpec{FG: p.Selection},
				HR:         StyleSpec{FG: s.border},
			},
		},
	}
}

func readableForeground(p Palette, background string) string {
	if contrastRatio(p.FG, background) >= contrastRatio(p.BG, background) {
		return p.FG
	}
	return p.BG
}

// builtinSpecs lists the modern themes bundled with Gorae.
var builtinSpecs = []builtinSpec{
	{
		key: "gorae-deep", name: "Gorae Deep",
		border: "#2d3a4a", headerBG: "#11181f", statusBG: "#070a0f",
		palette: Palette{
			BG: "#0a0f14", FG: "#eef1f6", Muted: "#8693a5", Accent: "#5cc8ff",
			Success: "#a3e635", Warning: "#fbbf24", Danger: "#fb7185", Selection: "#5eead4",
		},
	},
	{
		key: "catppuccin-mocha", name: "Catppuccin Mocha",
		border: "#313244", headerBG: "#181825", statusBG: "#11111b",
		palette: Palette{
			BG: "#1e1e2e", FG: "#cdd6f4", Muted: "#9399b2", Accent: "#89b4fa",
			Success: "#a6e3a1", Warning: "#f9e2af", Danger: "#f38ba8", Selection: "#94e2d5",
		},
	},
	{
		key: "catppuccin-latte", name: "Catppuccin Latte",
		border: "#ccd0da", headerBG: "#e6e9ef", statusBG: "#dce0e8",
		palette: Palette{
			BG: "#eff1f5", FG: "#4c4f69", Muted: "#5c5f77", Accent: "#174fc4",
			Success: "#28751f", Warning: "#805200", Danger: "#d20f39", Selection: "#0d655f",
		},
	},
	{
		key: "tokyo-night", name: "Tokyo Night",
		border: "#292e42", headerBG: "#16161e", statusBG: "#13131a",
		palette: Palette{
			BG: "#1a1b26", FG: "#c0caf5", Muted: "#8992b8", Accent: "#7aa2f7",
			Success: "#9ece6a", Warning: "#e0af68", Danger: "#f7768e", Selection: "#2ac3de",
		},
	},
	{
		key: "dracula", name: "Dracula",
		border: "#44475a", headerBG: "#21222c", statusBG: "#191a21",
		palette: Palette{
			BG: "#282a36", FG: "#f8f8f2", Muted: "#8994c6", Accent: "#bd93f9",
			Success: "#50fa7b", Warning: "#f1fa8c", Danger: "#ff5555", Selection: "#8be9fd",
		},
	},
	{
		key: "nord", name: "Nord",
		border: "#3b4252", headerBG: "#272c36", statusBG: "#21262e",
		palette: Palette{
			BG: "#2e3440", FG: "#eceff4", Muted: "#9aa7bd", Accent: "#88c0d0",
			Success: "#a3be8c", Warning: "#ebcb8b", Danger: "#bf616a", Selection: "#8fbcbb",
		},
	},
	{
		key: "gruvbox", name: "Gruvbox Dark",
		border: "#3c3836", headerBG: "#1d2021", statusBG: "#1d2021",
		palette: Palette{
			BG: "#282828", FG: "#ebdbb2", Muted: "#a89984", Accent: "#83a598",
			Success: "#b8bb26", Warning: "#fabd2f", Danger: "#fb4934", Selection: "#8ec07c",
		},
	},
	{
		key: "rose-pine", name: "Rosé Pine",
		border: "#26233a", headerBG: "#1f1d2e", statusBG: "#16141f",
		palette: Palette{
			BG: "#191724", FG: "#e0def4", Muted: "#908caa", Accent: "#c4a7e7",
			Success: "#9ccfd8", Warning: "#f6c177", Danger: "#eb6f92", Selection: "#ebbcba",
		},
	},
	{
		key: "solarized-dark", name: "Solarized Dark",
		border: "#073642", headerBG: "#073642", statusBG: "#002028",
		palette: Palette{
			BG: "#002b36", FG: "#b8c4c4", Muted: "#839496", Accent: "#4aa3d8",
			Success: "#a3b500", Warning: "#d0a000", Danger: "#ef5350", Selection: "#35b9aa",
		},
	},
}

// builtinThemes maps each theme key to its fully-derived Theme.
var builtinThemes = func() map[string]Theme {
	m := make(map[string]Theme, len(builtinSpecs))
	for _, s := range builtinSpecs {
		m[s.key] = buildTheme(s)
	}
	return m
}()

// Builtin returns the named built-in theme. Lookup is case-insensitive on the
// theme key (e.g. "tokyo-night"). The second return value reports whether the
// name matched a known theme.
func Builtin(name string) (Theme, bool) {
	th, ok := builtinThemes[normalizeKey(name)]
	return th, ok
}

// IsBuiltin reports whether name refers to a bundled theme.
func IsBuiltin(name string) bool {
	_, ok := builtinThemes[normalizeKey(name)]
	return ok
}

// BuiltinNames returns the theme keys in a stable, sorted order.
func BuiltinNames() []string {
	names := make([]string, 0, len(builtinSpecs))
	for _, s := range builtinSpecs {
		names = append(names, s.key)
	}
	sort.Strings(names)
	return names
}

// BuiltinDisplayName returns the human-readable name for a theme key, or the
// key itself when it is not a known built-in.
func BuiltinDisplayName(name string) string {
	key := normalizeKey(name)
	for _, s := range builtinSpecs {
		if s.key == key {
			return s.name
		}
	}
	return name
}

func normalizeKey(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		case r == '_' || r == ' ':
			out = append(out, '-')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}
