package theme

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/Han8931/gorae/internal/simpletoml"
)

func contrastRatio(a, b string) float64 {
	la, oka := relativeLuminance(a)
	lb, okb := relativeLuminance(b)
	if !oka || !okb {
		return 0
	}
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func relativeLuminance(hex string) (float64, bool) {
	var r, g, b uint8
	if _, err := fmt.Sscanf(strings.TrimSpace(hex), "#%02x%02x%02x", &r, &g, &b); err != nil {
		return 0, false
	}
	linear := func(v uint8) float64 {
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b), true
}

const defaultThemeFile = `# Gorae color theme.
# "Gorae Deep" — vibrant ocean-toned dark default.
# Update the values below to customize the UI.

[meta]
name = "Gorae Deep"
version = 1

[palette]
bg = "#0a0f14"
fg = "#eef1f6"
muted = "#8693a5"
accent = "#5cc8ff"
success = "#a3e635"
warning = "#fbbf24"
danger = "#fb7185"
selection = "#5eead4"

[borders]
style = "rounded"
color = "#2d3a4a"

[icons]
mode = "unicode"
favorite = "★"
toread = "☐"
read = "✓"
reading = "◐"
unread = "○"
folder = "▸"
pdf = "▣"

[components.app_header]
fg = "#eef1f6"
bg = "#0a0f14"
bold = true

[components.tree_header]
fg = "#a3e635"
bg = "#11181f"
bold = true

[components.tree_body]
fg = "#dde3ec"

[components.tree_active]
fg = "#fbbf24"
bold = true

[components.tree_info]
fg = "#8693a5"
italic = true

[components.list_header]
fg = "#eef1f6"
bg = "#161f29"
bold = true

[components.list_body]
fg = "#eef1f6"

[components.list_selected]
fg = "#0a0f14"
bg = "#5eead4"
bold = true

[components.list_cursor]
fg = "#0a0f14"
bg = "#fbbf24"
bold = true

[components.list_cursor_selected]
fg = "#0a0f14"
bg = "#fbbf24"
bold = true

[components.preview_header]
fg = "#5cc8ff"
bg = "#11181f"
bold = true

[components.preview_body]
fg = "#eef1f6"

[components.preview_info]
fg = "#5cc8ff"
bold = true

[components.separator]
fg = "#2d3a4a"

[components.status_bar]
fg = "#eef1f6"
bg = "#070a0f"

[components.status_label]
fg = "#5eead4"
bold = true

[components.status_value]
fg = "#fbbf24"

[components.prompt_label]
fg = "#0a0f14"
bg = "#5cc8ff"
bold = true

[components.prompt_value]
fg = "#eef1f6"
bg = "#0a0f14"

[components.meta_overlay]
fg = "#eef1f6"
bg = "#11181f"

[components.markdown]
[components.markdown.h1]
fg = "#5cc8ff"
bold = true

[components.markdown.h2]
fg = "#a3e635"
bold = true

[components.markdown.h3]
fg = "#fbbf24"
bold = true

[components.markdown.code]
fg = "#fbbf24"

[components.markdown.code_block]
fg = "#eef1f6"

[components.markdown.blockquote]
fg = "#8693a5"
italic = true

[components.markdown.link]
fg = "#5eead4"

[components.markdown.hr]
fg = "#2d3a4a"`

type Meta struct {
	Name    string `toml:"name" json:"name"`
	Version int    `toml:"version" json:"version"`
}

type Palette struct {
	BG        string `toml:"bg" json:"bg"`
	FG        string `toml:"fg" json:"fg"`
	Muted     string `toml:"muted" json:"muted"`
	Accent    string `toml:"accent" json:"accent"`
	Success   string `toml:"success" json:"success"`
	Warning   string `toml:"warning" json:"warning"`
	Danger    string `toml:"danger" json:"danger"`
	Selection string `toml:"selection" json:"selection"`
}

type Borders struct {
	Style string `toml:"style" json:"style"`
	Color string `toml:"color" json:"color"`
}

type StyleSpec struct {
	FG     string `toml:"fg" json:"fg"`
	BG     string `toml:"bg" json:"bg"`
	Bold   bool   `toml:"bold" json:"bold"`
	Italic bool   `toml:"italic" json:"italic"`
	Faint  bool   `toml:"faint" json:"faint"`
}

type MarkdownStyle struct {
	H1         StyleSpec `toml:"h1" json:"h1"`
	H2         StyleSpec `toml:"h2" json:"h2"`
	H3         StyleSpec `toml:"h3" json:"h3"`
	Code       StyleSpec `toml:"code" json:"code"`
	CodeBlock  StyleSpec `toml:"code_block" json:"code_block"`
	Blockquote StyleSpec `toml:"blockquote" json:"blockquote"`
	Link       StyleSpec `toml:"link" json:"link"`
	HR         StyleSpec `toml:"hr" json:"hr"`
}

type ComponentStyles struct {
	AppHeader        StyleSpec     `toml:"app_header" json:"app_header"`
	TreeHeader       StyleSpec     `toml:"tree_header" json:"tree_header"`
	TreeBody         StyleSpec     `toml:"tree_body" json:"tree_body"`
	TreeActive       StyleSpec     `toml:"tree_active" json:"tree_active"`
	TreeInfo         StyleSpec     `toml:"tree_info" json:"tree_info"`
	ListHeader       StyleSpec     `toml:"list_header" json:"list_header"`
	ListBody         StyleSpec     `toml:"list_body" json:"list_body"`
	ListSelected     StyleSpec     `toml:"list_selected" json:"list_selected"`
	ListCursor       StyleSpec     `toml:"list_cursor" json:"list_cursor"`
	ListCursorSelect StyleSpec     `toml:"list_cursor_selected" json:"list_cursor_selected"`
	PreviewHeader    StyleSpec     `toml:"preview_header" json:"preview_header"`
	PreviewBody      StyleSpec     `toml:"preview_body" json:"preview_body"`
	PreviewInfo      StyleSpec     `toml:"preview_info" json:"preview_info"`
	Separator        StyleSpec     `toml:"separator" json:"separator"`
	StatusBar        StyleSpec     `toml:"status_bar" json:"status_bar"`
	StatusLabel      StyleSpec     `toml:"status_label" json:"status_label"`
	StatusValue      StyleSpec     `toml:"status_value" json:"status_value"`
	PromptLabel      StyleSpec     `toml:"prompt_label" json:"prompt_label"`
	PromptValue      StyleSpec     `toml:"prompt_value" json:"prompt_value"`
	MetaOverlay      StyleSpec     `toml:"meta_overlay" json:"meta_overlay"`
	Markdown         MarkdownStyle `toml:"markdown" json:"markdown"`
}

type Icons struct {
	Mode      string `toml:"mode" json:"mode"`
	Favorite  string `toml:"favorite" json:"favorite"`
	ToRead    string `toml:"toread" json:"toread"`
	Read      string `toml:"read" json:"read"`
	Reading   string `toml:"reading" json:"reading"`
	Unread    string `toml:"unread" json:"unread"`
	Folder    string `toml:"folder" json:"folder"`
	PDF       string `toml:"pdf" json:"pdf"`
	Selected  string `toml:"selected" json:"selected"`
	Selection string `toml:"selection" json:"selection"`
}

type IconSet struct {
	Favorite  string
	ToRead    string
	Read      string
	Reading   string
	Unread    string
	Folder    string
	PDF       string
	Selected  string
	Selection string
}

type Theme struct {
	Meta       Meta            `toml:"meta" json:"meta"`
	Palette    Palette         `toml:"palette" json:"palette"`
	Borders    Borders         `toml:"borders" json:"borders"`
	Icons      Icons           `toml:"icons" json:"icons"`
	Components ComponentStyles `toml:"components" json:"components"`
}

func Default() Theme {
	return Theme{
		Meta: Meta{Name: "Gorae Deep", Version: 1},
		Palette: Palette{
			BG:        "#0a0f14",
			FG:        "#eef1f6",
			Muted:     "#8693a5",
			Accent:    "#5cc8ff",
			Success:   "#a3e635",
			Warning:   "#fbbf24",
			Danger:    "#fb7185",
			Selection: "#5eead4",
		},
		Borders: Borders{
			Style: "rounded",
			Color: "#2d3a4a",
		},
		Icons: Icons{
			Mode:      "unicode",
			Favorite:  "★",
			ToRead:    "☐",
			Read:      "✓",
			Reading:   "◐",
			Unread:    "○",
			Folder:    "▸",
			PDF:       "▣",
			Selected:  "✔",
			Selection: "▌",
		},
		Components: ComponentStyles{
			AppHeader:  StyleSpec{FG: "#eef1f6", BG: "#0a0f14", Bold: true},
			TreeHeader: StyleSpec{FG: "#a3e635", BG: "#11181f", Bold: true},
			TreeBody:   StyleSpec{FG: "#dde3ec"},
			TreeActive: StyleSpec{FG: "#fbbf24", Bold: true},
			TreeInfo:   StyleSpec{FG: "#8693a5", Italic: true},

			ListHeader:       StyleSpec{FG: "#eef1f6", BG: "#161f29", Bold: true},
			ListBody:         StyleSpec{FG: "#eef1f6"},
			ListSelected:     StyleSpec{FG: "#0a0f14", BG: "#5eead4", Bold: true},
			ListCursor:       StyleSpec{FG: "#0a0f14", BG: "#fbbf24", Bold: true},
			ListCursorSelect: StyleSpec{FG: "#0a0f14", BG: "#fbbf24", Bold: true},

			PreviewHeader: StyleSpec{FG: "#5cc8ff", BG: "#11181f", Bold: true},
			PreviewBody:   StyleSpec{FG: "#eef1f6"},
			PreviewInfo:   StyleSpec{FG: "#5cc8ff", Bold: true},

			Separator:   StyleSpec{FG: "#2d3a4a"},
			StatusBar:   StyleSpec{FG: "#eef1f6", BG: "#070a0f"},
			StatusLabel: StyleSpec{FG: "#5eead4", Bold: true},
			StatusValue: StyleSpec{FG: "#fbbf24"},
			PromptLabel: StyleSpec{FG: "#0a0f14", BG: "#5cc8ff", Bold: true},
			PromptValue: StyleSpec{FG: "#eef1f6", BG: "#0a0f14"},
			MetaOverlay: StyleSpec{FG: "#eef1f6", BG: "#11181f"},
			Markdown: MarkdownStyle{
				H1:         StyleSpec{FG: "#5cc8ff", Bold: true},
				H2:         StyleSpec{FG: "#a3e635", Bold: true},
				H3:         StyleSpec{FG: "#fbbf24", Bold: true},
				Code:       StyleSpec{FG: "#fbbf24"},
				CodeBlock:  StyleSpec{FG: "#eef1f6"},
				Blockquote: StyleSpec{FG: "#8693a5", Italic: true},
				Link:       StyleSpec{FG: "#5eead4"},
				HR:         StyleSpec{FG: "#2d3a4a"},
			},
		},
	}
}

func LoadActive() (Theme, error) {
	return loadTheme("")
}

// LoadFrom loads the theme from the provided path. When path is empty it falls
// back to the default theme path under the config directory.
func LoadFrom(path string) (Theme, error) {
	return loadTheme(path)
}

func loadTheme(path string) (Theme, error) {
	resolved := strings.TrimSpace(path)
	if resolved == "" {
		var err error
		resolved, err = themePath()
		if err != nil {
			return Theme{}, err
		}
	}
	base := Default()
	data, err := os.ReadFile(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := ensureDefaultTheme(resolved); err != nil {
				return base, err
			}
			data, err = os.ReadFile(resolved)
			if err != nil {
				return base, err
			}
		} else {
			return base, err
		}
	}
	if err := simpletoml.Decode(data, &base); err != nil {
		return base, fmt.Errorf("parse theme: %w", err)
	}
	upgradeLegacyDefaultTheme(&base)
	return base, nil
}

// upgradeLegacyDefaultTheme keeps an old, generated Gorae Deep theme from
// masking newer built-in defaults after the application is reinstalled. Only
// values that exactly match the former defaults are migrated; user-selected
// colors and other custom themes are left alone.
func upgradeLegacyDefaultTheme(t *Theme) {
	if t == nil || t.Meta.Name != "Gorae Deep" || t.Meta.Version != 1 {
		return
	}
	if t.Icons.ToRead == "•" {
		t.Icons.ToRead = "☐"
	}
	if t.Icons.Reading == "▶" {
		t.Icons.Reading = "◐"
	}
	if (t.Components.ListSelected.BG == "" || t.Components.ListSelected.BG == t.Palette.Selection) &&
		t.Components.ListSelected.FG == t.Palette.Selection {
		t.Components.ListSelected.FG = t.Palette.BG
		t.Components.ListSelected.BG = t.Palette.Selection
	}
	if t.Components.ListCursorSelect.BG == t.Palette.Selection {
		t.Components.ListCursorSelect = t.Components.ListCursor
	}
}

// Path returns the resolved path to the active theme file.
func Path() (string, error) {
	return themePath()
}

func themePath() (string, error) {
	cfgHome := os.Getenv("XDG_CONFIG_HOME")
	if cfgHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cfgHome = filepath.Join(home, ".config")
	}
	return filepath.Join(cfgHome, "gorae", "theme.toml"), nil
}

func (t Theme) IconSet() IconSet {
	mode := strings.ToLower(strings.TrimSpace(t.Icons.Mode))
	var base IconSet
	switch mode {
	case "nerd":
		base = IconSet{
			Favorite:  "",
			ToRead:    "☐",
			Read:      "✓",
			Reading:   "◐",
			Unread:    "○",
			Folder:    "",
			PDF:       "",
			Selected:  "✔",
			Selection: "▌",
		}
	case "ascii":
		base = IconSet{
			Favorite:  "*",
			ToRead:    "t",
			Read:      "v",
			Reading:   ">",
			Unread:    "o",
			Folder:    "d",
			PDF:       "p",
			Selected:  "*",
			Selection: "|",
		}
	case "off":
		base = IconSet{
			PDF: " ",
		}
	default:
		// unicode default
		base = IconSet{
			Favorite:  "★",
			ToRead:    "☐",
			Read:      "✓",
			Reading:   "◐",
			Unread:    "○",
			Folder:    "",
			PDF:       "§",
			Selected:  "✔",
			Selection: "▌",
		}
	}
	if t.Icons.Favorite != "" {
		base.Favorite = t.Icons.Favorite
	}
	if t.Icons.ToRead != "" {
		base.ToRead = t.Icons.ToRead
	}
	if t.Icons.Read != "" {
		base.Read = t.Icons.Read
	}
	if t.Icons.Reading != "" {
		base.Reading = t.Icons.Reading
	}
	if t.Icons.Unread != "" {
		base.Unread = t.Icons.Unread
	}
	if t.Icons.Folder != "" {
		base.Folder = t.Icons.Folder
	}
	if t.Icons.PDF != "" {
		base.PDF = t.Icons.PDF
	}
	if t.Icons.Selected != "" {
		base.Selected = t.Icons.Selected
	}
	if t.Icons.Selection != "" {
		base.Selection = t.Icons.Selection
	}
	return base
}

func ensureDefaultTheme(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	data := []byte(defaultThemeFile + "\n")
	return os.WriteFile(path, data, 0o644)
}
