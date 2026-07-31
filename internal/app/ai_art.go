package app

import (
	"github.com/charmbracelet/lipgloss"
)

// goraeArt returns colored lines of a side-profile whale (고래).
func (m Model) goraeArt() []string {
	accent := lipgloss.NewStyle().Foreground(m.styles.Preview.Info.GetForeground())
	wave := lipgloss.NewStyle().Foreground(m.styles.Separator.GetForeground())

	a := accent.Render

	//  GORAE in a block-font wordmark above a wave.
	//
	//  ██████╗  ██████╗ ██████╗  █████╗ ███████╗
	// ██╔════╝ ██╔═══██╗██╔══██╗██╔══██╗██╔════╝
	// ██║  ███╗██║   ██║██████╔╝███████║█████╗
	// ██║   ██║██║   ██║██╔══██╗██╔══██║██╔══╝
	// ╚██████╔╝╚██████╔╝██║  ██║██║  ██║███████╗
	//  ╚═════╝  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝
	//  ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	//      ~      ~      ~      ~      ~
	//
	return []string{
		"",
		"  " + a(" ██████╗  ██████╗ ██████╗  █████╗ ███████╗"),
		"  " + a("██╔════╝ ██╔═══██╗██╔══██╗██╔══██╗██╔════╝"),
		"  " + a("██║  ███╗██║   ██║██████╔╝███████║█████╗  "),
		"  " + a("██║   ██║██║   ██║██╔══██╗██╔══██║██╔══╝  "),
		"  " + a("╚██████╔╝╚██████╔╝██║  ██║██║  ██║███████╗"),
		"  " + a(" ╚═════╝  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝"),
		"  " + wave.Render("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~"),
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
		infoStyle.Render("  Just talk about a paper — \"summarize the paper on X\" or"),
		infoStyle.Render("  \"compare the retrieval paper with the long-context one\" —"),
		infoStyle.Render("  and I'll pull it from your library into the conversation."),
	)
	lines = append(lines, "")
	lines = append(lines,
		mutedStyle.Render("  /load add a paper   /unfocus clear   /sources   /help"),
		mutedStyle.Render("  Esc to return to the file browser."),
	)

	if m.aiClient != nil {
		lines = append(lines, "")
		lines = append(lines, mutedStyle.Render("  model: "+m.aiClient.Model()))
	}

	lines = append(lines, "")
	return lines
}
