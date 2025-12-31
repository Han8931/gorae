package theme

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorae/internal/simpletoml"
)

const defaultThemeFile = `# Gorae color theme.
# Update the values below to customize the UI.

[meta]
name = "Gorae Default"
version = 1

[palette]
bg = "#1e1e2e"
fg = "#f2d5cf"
muted = "#7f8ca3"
accent = "#cba6f7"
success = "#a6e3a1"
warning = "#f9e2af"
danger = "#f38ba8"
selection = "#89dceb"

[borders]
style = "rounded"
color = "palette.muted"

[icons]
mode = "unicode"
favorite = "★"
toread = "•"
read = "✓"
reading = "▶"
unread = "○"
folder = "▸"
pdf = "▣"
selected = "✔"
selection = "▌"

[components.app_header]
fg = "palette.fg"
bg = "palette.bg"
bold = true

[components.tree_header]
fg = "palette.success"
bg = "palette.bg"
bold = true

[components.tree_body]
fg = "palette.fg"

[components.tree_active]
fg = "palette.warning"
bold = true

[components.tree_info]
fg = "palette.muted"
italic = true

[components.list_header]
fg = "palette.fg"
bg = "palette.bg"
bold = true

[components.list_body]
fg = "palette.fg"

[components.list_selected]
fg = "palette.selection"
bold = true

[components.list_cursor]
fg = "palette.bg"
bg = "palette.warning"
bold = true

[components.list_cursor_selected]
fg = "palette.bg"
bg = "palette.danger"
bold = true

[components.preview_header]
fg = "palette.accent"
bg = "palette.bg"
bold = true

[components.preview_body]
fg = "palette.fg"

[components.preview_info]
fg = "palette.accent"
bold = true

[components.separator]
fg = "palette.muted"

[components.status_bar]
fg = "palette.fg"
bg = "palette.bg"

[components.status_label]
fg = "palette.accent"
bold = true

[components.status_value]
fg = "palette.warning"

[components.prompt_label]
fg = "palette.bg"
bg = "palette.accent"
bold = true

[components.prompt_value]
fg = "palette.fg"
bg = "palette.bg"

[components.meta_overlay]
fg = "palette.fg"
bg = "palette.bg"`

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

type ComponentStyles struct {
	AppHeader        StyleSpec `toml:"app_header" json:"app_header"`
	TreeHeader       StyleSpec `toml:"tree_header" json:"tree_header"`
	TreeBody         StyleSpec `toml:"tree_body" json:"tree_body"`
	TreeActive       StyleSpec `toml:"tree_active" json:"tree_active"`
	TreeInfo         StyleSpec `toml:"tree_info" json:"tree_info"`
	ListHeader       StyleSpec `toml:"list_header" json:"list_header"`
	ListBody         StyleSpec `toml:"list_body" json:"list_body"`
	ListSelected     StyleSpec `toml:"list_selected" json:"list_selected"`
	ListCursor       StyleSpec `toml:"list_cursor" json:"list_cursor"`
	ListCursorSelect StyleSpec `toml:"list_cursor_selected" json:"list_cursor_selected"`
	PreviewHeader    StyleSpec `toml:"preview_header" json:"preview_header"`
	PreviewBody      StyleSpec `toml:"preview_body" json:"preview_body"`
	PreviewInfo      StyleSpec `toml:"preview_info" json:"preview_info"`
	Separator        StyleSpec `toml:"separator" json:"separator"`
	StatusBar        StyleSpec `toml:"status_bar" json:"status_bar"`
	StatusLabel      StyleSpec `toml:"status_label" json:"status_label"`
	StatusValue      StyleSpec `toml:"status_value" json:"status_value"`
	PromptLabel      StyleSpec `toml:"prompt_label" json:"prompt_label"`
	PromptValue      StyleSpec `toml:"prompt_value" json:"prompt_value"`
	MetaOverlay      StyleSpec `toml:"meta_overlay" json:"meta_overlay"`
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
		Meta: Meta{Name: "Gorae Default", Version: 1},
		Palette: Palette{
			BG:        "#1e1e2e",
			FG:        "#f2d5cf",
			Muted:     "#7f8ca3",
			Accent:    "#cba6f7",
			Success:   "#a6e3a1",
			Warning:   "#f9e2af",
			Danger:    "#f38ba8",
			Selection: "#89dceb",
		},
		Borders: Borders{
			Style: "rounded",
			Color: "#585b70",
		},
		Icons: Icons{
			Mode:      "unicode",
			Favorite:  "★",
			ToRead:    "•",
			Read:      "✓",
			Reading:   "▶",
			Unread:    "○",
			Folder:    "▸",
			PDF:       "▣",
			Selected:  "✔",
			Selection: "▌",
		},
		Components: ComponentStyles{
			AppHeader:  StyleSpec{FG: "#f5e0dc", BG: "#1e1e2e", Bold: true},
			TreeHeader: StyleSpec{FG: "#a6e3a1", BG: "#1b2430", Bold: true},
			TreeBody:   StyleSpec{FG: "#b4c2f8"},
			TreeActive: StyleSpec{FG: "#f9e2af", Bold: true},
			TreeInfo:   StyleSpec{FG: "#7f8ca3", Italic: true},

			ListHeader:       StyleSpec{FG: "#f5e0dc", BG: "#2a2438", Bold: true},
			ListBody:         StyleSpec{FG: "#f2d5cf"},
			ListSelected:     StyleSpec{FG: "#89dceb", Bold: true},
			ListCursor:       StyleSpec{FG: "#1e1e2e", BG: "#f9e2af", Bold: true},
			ListCursorSelect: StyleSpec{FG: "#1e1e2e", BG: "#fab387", Bold: true},

			PreviewHeader: StyleSpec{FG: "#cba6f7", BG: "#241f3d", Bold: true},
			PreviewBody:   StyleSpec{FG: "#cdd6f4"},
			PreviewInfo:   StyleSpec{FG: "#f4b8e4", Bold: true},

			Separator:   StyleSpec{FG: "#585b70"},
			StatusBar:   StyleSpec{FG: "#cdd6f4", BG: "#11111b"},
			StatusLabel: StyleSpec{FG: "#94e2d5", Bold: true},
			StatusValue: StyleSpec{FG: "#f9e2af"},
			PromptLabel: StyleSpec{FG: "#1e1e2e", BG: "#cba6f7", Bold: true},
			PromptValue: StyleSpec{FG: "#f5e0dc", BG: "#1e1e2e"},
			MetaOverlay: StyleSpec{FG: "#f2cdcd", BG: "#312244"},
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
	return base, nil
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
			ToRead:    "",
			Read:      "",
			Reading:   "",
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
			Folder:    "[D]",
			PDF:       "[F]",
			Selected:  "*",
			Selection: "|",
		}
	case "off":
		base = IconSet{}
	default:
		// unicode default
		base = IconSet{
			Favorite:  "★",
			ToRead:    "•",
			Read:      "✓",
			Reading:   "▶",
			Unread:    "○",
			Folder:    "",
			PDF:       "▣",
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
