package app

import (
	"github.com/charmbracelet/lipgloss"
)

// goraeArt returns colored lines of a side-profile whale (고래).
func (m Model) goraeArt() []string {
	accent := lipgloss.NewStyle().Foreground(m.styles.Preview.Info.GetForeground())
	wave := lipgloss.NewStyle().Foreground(m.styles.Separator.GetForeground())

	a := accent.Render

	//  GORAE in big ASCII letters above a wave.
	//
	//   ____    ___    ____      _      _____
	//  / ___|  / _ \  |  _ \   / \    | ____|
	// | |  _  | | | | | |_) | / _ \   |  _|
	// | |_| | | |_| | |  _ < / ___ \  | |___
	//  \____|  \___/  |_| \_\/_/   \_\|_____|
	//  ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	//      ~      ~      ~      ~      ~
	//
	return []string{
		"",
		"  " + a("  ____    ___    ____      _      _____  "),
		"  " + a(" / ___|  / _ \\  |  _ \\   / \\    | ____|  "),
		"  " + a("| |  _  | | | | | |_) | / _ \\   |  _|   "),
		"  " + a("| |_| | | |_| | |  _ < / ___ \\  | |___  "),
		"  " + a(" \\____|  \\___/  |_| \\_\\/_/   \\_\\|_____|  "),
		"  " + wave.Render("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~"),
		"      " + wave.Render("~      ~      ~      ~      ~"),
		"",
	}
}

// goraeWelcomeLines builds the full welcome screen (art + intro text).
func (m Model) goraeWelcomeLines() []string {
	infoStyle := m.styles.Preview.Info
	mutedStyle := m.styles.Preview.Body

	var lines []string
	lines = append(lines, m.goraeArt()...)

	title := m.styles.StatusLabel.Render("  G O R A E  A I  ")
	lines = append(lines, "  "+title)
	lines = append(lines, "")
	lines = append(lines,
		infoStyle.Render("  고래 (Gorae) — your document library assistant."),
		infoStyle.Render("  Ask anything. I'll find relevant context from your library."),
	)
	lines = append(lines, "")
	lines = append(lines,
		mutedStyle.Render("  /clear   /export   /sources   /help"),
		mutedStyle.Render("  Esc to return to the file browser."),
	)

	if m.aiClient != nil {
		lines = append(lines, "")
		lines = append(lines, mutedStyle.Render("  model: "+m.aiClient.Model()))
	}

	lines = append(lines, "")
	return lines
}
