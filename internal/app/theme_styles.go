package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Han8931/gorae/internal/theme"
)

type panelStyles struct {
	Header         lipgloss.Style
	Body           lipgloss.Style
	Info           lipgloss.Style
	Active         lipgloss.Style
	Selected       lipgloss.Style
	Cursor         lipgloss.Style
	CursorSelected lipgloss.Style
	// Border is the style used for this panel's frame. The primary (Files) pane
	// carries a stronger accent border while passive panes stay muted, giving
	// the layout a clear visual hierarchy.
	Border lipgloss.Style
}

type mdStyles struct {
	H1         lipgloss.Style
	H2         lipgloss.Style
	H3         lipgloss.Style
	Code       lipgloss.Style
	CodeBlock  lipgloss.Style
	Blockquote lipgloss.Style
	Link       lipgloss.Style
	HR         lipgloss.Style
	Body       lipgloss.Style
}

type viewStyles struct {
	AppHeader   lipgloss.Style
	Tree        panelStyles
	List        panelStyles
	Preview     panelStyles
	StatusBar   lipgloss.Style
	StatusLabel lipgloss.Style
	StatusValue lipgloss.Style
	PromptLabel lipgloss.Style
	PromptValue lipgloss.Style
	Separator   lipgloss.Style
	MetaOverlay lipgloss.Style
	Border      lipgloss.Style
	SepChar     string
	Markdown    mdStyles
	ChatUser    lipgloss.Style

	// Semantic roles derived from the palette so every view (including the AI
	// chat) follows the active theme instead of hardcoded colours.
	Accent      lipgloss.Style
	Muted       lipgloss.Style // secondary text, hints
	Faint       lipgloss.Style // quieter than Muted: rules, dividers
	Success     lipgloss.Style
	Warning     lipgloss.Style
	Danger      lipgloss.Style
	ModeChip    lipgloss.Style // filled accent chip ( NORMAL )
	WarnChip    lipgloss.Style // filled warning chip (chat NORMAL, cursor badge)
	DangerChip  lipgloss.Style // filled danger chip (delete confirmation)
	InfoChip    lipgloss.Style // filled selection chip (sort/filter/count)
	KeyCap      lipgloss.Style // key in the hint bar ( j )
	KeyLabel    lipgloss.Style // its description
	ModalBorder lipgloss.Color
}

type borderCharset struct {
	Vertical    string
	Horizontal  string
	TopLeft     string
	TopRight    string
	BottomLeft  string
	BottomRight string
}

func newViewStyles(th theme.Theme) viewStyles {
	palette := th.Palette
	// Passive panes (Tree, Details) use the muted theme border color. The Files
	// pane is the primary interactive pane, so it gets a stronger accent border to
	// make the visual hierarchy obvious. Falls back to the muted border when the
	// palette defines no accent.
	mutedBorder := lipgloss.NewStyle().Foreground(lipgloss.Color(resolveColor(th.Borders.Color, palette)))
	primaryBorder := mutedBorder
	if accent := resolveColor(palette.Accent, palette); accent != "" {
		primaryBorder = lipgloss.NewStyle().Foreground(lipgloss.Color(accent)).Bold(true)
	}
	return viewStyles{
		AppHeader: styleFromSpec(palette, th.Components.AppHeader),
		Tree: panelStyles{
			Header: styleFromSpec(palette, th.Components.TreeHeader),
			Body:   styleFromSpec(palette, th.Components.TreeBody),
			Info:   styleFromSpec(palette, th.Components.TreeInfo),
			Active: styleFromSpec(palette, th.Components.TreeActive),
			// The tree cursor (when the pane is focused) reuses the Files-pane
			// cursor styling so the highlighted row is clearly visible.
			Cursor: styleFromSpec(palette, th.Components.ListCursor),
			Border: mutedBorder,
		},
		List: panelStyles{
			Header:         styleFromSpec(palette, th.Components.ListHeader),
			Body:           styleFromSpec(palette, th.Components.ListBody),
			Selected:       styleFromSpec(palette, th.Components.ListSelected),
			Cursor:         styleFromSpec(palette, th.Components.ListCursor),
			CursorSelected: styleFromSpec(palette, th.Components.ListCursorSelect),
			Border:         primaryBorder,
		},
		Preview: panelStyles{
			Header: styleFromSpec(palette, th.Components.PreviewHeader),
			Body:   styleFromSpec(palette, th.Components.PreviewBody),
			Info:   styleFromSpec(palette, th.Components.PreviewInfo),
			Border: mutedBorder,
		},
		StatusBar:   styleFromSpec(palette, th.Components.StatusBar),
		StatusLabel: styleFromSpec(palette, th.Components.StatusLabel),
		StatusValue: styleFromSpec(palette, th.Components.StatusValue),
		PromptLabel: styleFromSpec(palette, th.Components.PromptLabel),
		PromptValue: styleFromSpec(palette, th.Components.PromptValue),
		Separator:   styleFromSpec(palette, th.Components.Separator),
		MetaOverlay: styleFromSpec(palette, th.Components.MetaOverlay),
		Border:      lipgloss.NewStyle().Foreground(lipgloss.Color(resolveColor(th.Borders.Color, palette))),
		SepChar:     borderCharsetFor(th.Borders.Style).Vertical,
		Markdown: mdStyles{
			H1:         styleFromSpec(palette, th.Components.Markdown.H1),
			H2:         styleFromSpec(palette, th.Components.Markdown.H2),
			H3:         styleFromSpec(palette, th.Components.Markdown.H3),
			Code:       styleFromSpec(palette, th.Components.Markdown.Code),
			CodeBlock:  styleFromSpec(palette, th.Components.Markdown.CodeBlock),
			Blockquote: styleFromSpec(palette, th.Components.Markdown.Blockquote),
			Link:       styleFromSpec(palette, th.Components.Markdown.Link),
			HR:         styleFromSpec(palette, th.Components.Markdown.HR),
			Body:       styleFromSpec(palette, th.Components.PreviewBody),
		},
		ChatUser: lipgloss.NewStyle().
			Background(lipgloss.Color(surfaceColor(palette))).
			Foreground(lipgloss.Color(palette.FG)),

		Accent:      fgStyle(palette.Accent),
		Muted:       fgStyle(palette.Muted),
		Faint:       fgStyle(blendHex(palette.Muted, palette.BG, 0.45)),
		Success:     fgStyle(palette.Success),
		Warning:     fgStyle(palette.Warning),
		Danger:      fgStyle(palette.Danger).Bold(true),
		ModeChip:    chipStyle(palette.Accent),
		WarnChip:    chipStyle(palette.Warning),
		DangerChip:  chipStyle(palette.Danger),
		InfoChip:    chipStyle(palette.Selection),
		KeyCap:      lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Accent)).Background(lipgloss.Color(surfaceColor(palette))).Bold(true),
		KeyLabel:    fgStyle(palette.Muted),
		ModalBorder: lipgloss.Color(palette.Accent),
	}
}

func fgStyle(hex string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(strings.TrimSpace(hex)))
}

// chipStyle is a filled badge whose text colour is picked for legibility
// against the fill, so chips stay readable on both light and dark themes.
func chipStyle(bg string) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(strings.TrimSpace(bg))).
		Foreground(lipgloss.Color(textOn(bg))).
		Bold(true)
}

// textOn returns near-black or white, whichever reads better on bg.
func textOn(bg string) string {
	r, g, b, ok := parseHex(bg)
	if !ok {
		return ""
	}
	if 299*r+587*g+114*b > 140000 {
		return "#0d1117"
	}
	return "#ffffff"
}

// surfaceColor is a subtle raised background: the theme bg nudged toward fg.
func surfaceColor(p theme.Palette) string {
	return blendHex(p.BG, p.FG, 0.12)
}

// blendHex mixes a toward b by t (0..1). If either colour can't be parsed it
// returns a unchanged, so themes with partial palettes still render.
func blendHex(a, b string, t float64) string {
	ar, ag, ab, ok1 := parseHex(a)
	br, bg, bb, ok2 := parseHex(b)
	if !ok1 || !ok2 {
		return strings.TrimSpace(a)
	}
	mix := func(x, y int) int { return int(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", mix(ar, br), mix(ag, bg), mix(ab, bb))
}

func parseHex(hex string) (int, int, int, bool) {
	var r, g, b int
	if _, err := fmt.Sscanf(strings.TrimSpace(hex), "#%02x%02x%02x", &r, &g, &b); err != nil {
		return 0, 0, 0, false
	}
	return r, g, b, true
}

func styleFromSpec(p theme.Palette, spec theme.StyleSpec) lipgloss.Style {
	style := lipgloss.NewStyle()
	if fg := resolveColor(spec.FG, p); fg != "" {
		style = style.Foreground(lipgloss.Color(fg))
	}
	if bg := resolveColor(spec.BG, p); bg != "" {
		style = style.Background(lipgloss.Color(bg))
	}
	if spec.Bold {
		style = style.Bold(true)
	}
	if spec.Italic {
		style = style.Italic(true)
	}
	if spec.Faint {
		style = style.Faint(true)
	}
	return style
}

func resolveColor(value string, palette theme.Palette) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "palette.") {
		key := strings.TrimPrefix(lower, "palette.")
		switch key {
		case "bg":
			return strings.TrimSpace(palette.BG)
		case "fg":
			return strings.TrimSpace(palette.FG)
		case "muted":
			return strings.TrimSpace(palette.Muted)
		case "accent":
			return strings.TrimSpace(palette.Accent)
		case "success":
			return strings.TrimSpace(palette.Success)
		case "warning":
			return strings.TrimSpace(palette.Warning)
		case "danger":
			return strings.TrimSpace(palette.Danger)
		case "selection":
			return strings.TrimSpace(palette.Selection)
		}
	}
	return trimmed
}

func borderCharsetFor(style string) borderCharset {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "rounded":
		return borderCharset{
			Vertical:    "│",
			Horizontal:  "─",
			TopLeft:     "╭",
			TopRight:    "╮",
			BottomLeft:  "╰",
			BottomRight: "╯",
		}
	case "thick":
		return borderCharset{
			Vertical:    "┃",
			Horizontal:  "━",
			TopLeft:     "┏",
			TopRight:    "┓",
			BottomLeft:  "┗",
			BottomRight: "┛",
		}
	case "none":
		return borderCharset{
			Vertical:    " ",
			Horizontal:  " ",
			TopLeft:     " ",
			TopRight:    " ",
			BottomLeft:  " ",
			BottomRight: " ",
		}
	default:
		return borderCharset{
			Vertical:    "┃",
			Horizontal:  "─",
			TopLeft:     "┌",
			TopRight:    "┐",
			BottomLeft:  "└",
			BottomRight: "┘",
		}
	}
}
