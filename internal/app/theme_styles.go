package app

import (
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
			Background(lipgloss.Color("#2a2a2a")).
			Foreground(lipgloss.Color("#e8e8e8")),
	}
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
