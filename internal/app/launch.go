package app

import (
	"hash/fnv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// launchQuote is one epigraph shown on the launch screen: the line, who said
// it, and (optionally) the work it is from.
type launchQuote struct {
	Text   string
	Author string
	Work   string
}

// dailyLaunchQuote returns the quote for the day of t. The same date always
// yields the same quote, so it is stable within a session and rotates at
// midnight — the same daily-hash trick meari and page-sage use.
func dailyLaunchQuote(t time.Time) launchQuote {
	if len(launchQuotes) == 0 {
		return launchQuote{}
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.Format("2006-01-02")))
	return launchQuotes[int(h.Sum32())%len(launchQuotes)]
}

// launchQuotes is the curated set — aphorisms on reading, books, curiosity, and
// knowing, chosen to suit a document library that greets you at the start of a
// session.
var launchQuotes = []launchQuote{
	{Text: "A reader lives a thousand lives before he dies. The man who never reads lives only one.", Author: "George R.R. Martin", Work: "A Dance with Dragons"},
	{Text: "The reading of all good books is like a conversation with the finest minds of past centuries.", Author: "René Descartes"},
	{Text: "There is no friend as loyal as a book.", Author: "Ernest Hemingway"},
	{Text: "I cannot live without books.", Author: "Thomas Jefferson"},
	{Text: "A room without books is like a body without a soul.", Author: "Cicero"},
	{Text: "The more that you read, the more things you will know. The more that you learn, the more places you'll go.", Author: "Dr. Seuss"},
	{Text: "Reading is to the mind what exercise is to the body.", Author: "Joseph Addison"},
	{Text: "Somewhere, something incredible is waiting to be known.", Author: "Carl Sagan"},
	{Text: "The important thing is not to stop questioning. Curiosity has its own reason for existing.", Author: "Albert Einstein"},
	{Text: "I have no special talent. I am only passionately curious.", Author: "Albert Einstein"},
	{Text: "Research is what I'm doing when I don't know what I'm doing.", Author: "Wernher von Braun"},
	{Text: "If I have seen further it is by standing on the shoulders of giants.", Author: "Isaac Newton"},
	{Text: "The only true wisdom is in knowing you know nothing.", Author: "Socrates"},
	{Text: "Real knowledge is to know the extent of one's ignorance.", Author: "Confucius"},
	{Text: "Nothing in life is to be feared, it is only to be understood.", Author: "Marie Curie"},
	{Text: "Study without desire spoils the memory, and it retains nothing that it takes in.", Author: "Leonardo da Vinci"},
	{Text: "An investment in knowledge pays the best interest.", Author: "Benjamin Franklin"},
	{Text: "Read not to contradict and confute, nor to believe and take for granted, but to weigh and consider.", Author: "Francis Bacon"},
	{Text: "There is no substitute for reading; it is the foundation of all thought.", Author: "Anonymous"},
	{Text: "Once you learn to read, you will be forever free.", Author: "Frederick Douglass"},
	{Text: "Books are a uniquely portable magic.", Author: "Stephen King", Work: "On Writing"},
	{Text: "We read to know we are not alone.", Author: "C.S. Lewis"},
	{Text: "The whole purpose of education is to turn mirrors into windows.", Author: "Sydney J. Harris"},
	{Text: "Knowing yourself is the beginning of all wisdom.", Author: "Aristotle"},
	{Text: "Wonder is the beginning of wisdom.", Author: "Socrates"},
}

// launchItem is one row of the launch menu: a title, a one-line description, and
// the action it triggers.
type launchItem struct {
	title string
	desc  string
}

// launchItems is the read / open / load menu shown under the epigraph.
func launchItems() []launchItem {
	return []launchItem{
		{title: "Continue reading", desc: "pick up a recently read paper"},
		{title: "Open library", desc: "browse all your documents"},
		{title: "Ask Gorae AI", desc: "load a paper into a conversation"},
	}
}

// updateLaunch handles keys while the launch screen owns the view. Esc or q
// drops into the file browser; the menu rows dispatch to the read/open/load
// flows.
func (m Model) updateLaunch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := launchItems()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		// "Open library" is the default exit — same as the middle menu row.
		m.state = stateNormal
		return m, nil
	case "j", "down", "tab":
		m.launchCursor = (m.launchCursor + 1) % len(items)
		return m, nil
	case "k", "up", "shift+tab":
		m.launchCursor = (m.launchCursor - 1 + len(items)) % len(items)
		return m, nil
	case "g", "home":
		m.launchCursor = 0
		return m, nil
	case "G", "end":
		m.launchCursor = len(items) - 1
		return m, nil
	case "1":
		m.launchCursor = 0
		return m.activateLaunch()
	case "2":
		m.launchCursor = 1
		return m.activateLaunch()
	case "3":
		m.launchCursor = 2
		return m.activateLaunch()
	case "enter", "l", "right", " ":
		return m.activateLaunch()
	}
	return m, nil
}

// activateLaunch runs the currently selected launch-menu action.
func (m Model) activateLaunch() (tea.Model, tea.Cmd) {
	switch m.launchCursor {
	case 0: // Continue reading — list the recently read papers.
		cmd := m.showQuickFilter(quickFilterRecentlyOpened)
		return m, cmd
	case 1: // Open library — the file browser.
		m.state = stateNormal
		return m, nil
	case 2: // Load a new paper — Gorae AI with the /load finder open.
		enter := m.enterGoraeChat()
		find := m.enterFindMode("")
		return m, tea.Batch(enter, find)
	}
	m.state = stateNormal
	return m, nil
}

// renderLaunchView draws the startup splash: the whale wordmark, a daily
// epigraph, and the read/open/load menu, all centered in the window.
func (m Model) renderLaunchView() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	height := m.windowHeight
	if height <= 0 {
		height = 24
	}

	var sections []string
	sections = append(sections, padLaunchBlock(m.goraeArt()))
	sections = append(sections, m.styles.Preview.Info.Render("고래 · your reading companion"))

	quoteW := width - 6
	if quoteW > 64 {
		quoteW = 64
	}
	if q := m.launchQuoteBlock(quoteW); q != "" {
		sections = append(sections, "", q)
	}

	sections = append(sections, "", m.launchMenuBlock())
	sections = append(sections, "", m.styles.Separator.Render("↑/↓ move · enter choose · 1-3 jump · esc library · ctrl+c quit"))

	body := lipgloss.JoinVertical(lipgloss.Center, sections...)

	centerArea := height - 1
	if centerArea < 1 {
		centerArea = 1
	}
	placed := lipgloss.Place(width, centerArea, lipgloss.Center, lipgloss.Center, body)
	return placed + "\n" + m.renderStatusBar()
}

// launchQuoteBlock renders the daily epigraph as an italic, gutter-barred block
// with a dim attribution — the page-sage / meari style.
func (m Model) launchQuoteBlock(width int) string {
	q := m.launchQuote
	if strings.TrimSpace(q.Text) == "" {
		return ""
	}
	if width < 24 {
		width = 24
	}
	italic := m.styles.Markdown.Blockquote.Italic(true)
	bar := m.styles.StatusValue.Render("▌ ")

	var lines []string
	for _, ln := range wrapTextToWidth("❝ "+q.Text+" ❞", width) {
		lines = append(lines, bar+italic.Render(ln))
	}
	attribution := "— " + q.Author
	if q.Work != "" {
		attribution += ", " + q.Work
	}
	lines = append(lines, "  "+m.styles.Preview.Body.Render(attribution))
	return padLaunchBlock(lines)
}

// launchMenuBlock renders the read/open/load menu with the selected row
// highlighted. Rows are padded to a common width so the highlight bar and the
// left edge stay aligned when the block is centered.
func (m Model) launchMenuBlock() string {
	items := launchItems()
	sel := m.styles.List.Cursor
	title := m.styles.Preview.Info
	muted := m.styles.Preview.Body

	// Widest plain "title  —  desc" so every row lines up.
	plains := make([]string, len(items))
	maxw := 0
	for i, it := range items {
		p := it.title
		if it.desc != "" {
			p += "  —  " + it.desc
		}
		plains[i] = p
		if w := runewidth.StringWidth(p); w > maxw {
			maxw = w
		}
	}

	var lines []string
	for i, it := range items {
		if i == m.launchCursor {
			padded := plains[i]
			if pad := maxw - runewidth.StringWidth(padded); pad > 0 {
				padded += strings.Repeat(" ", pad)
			}
			lines = append(lines, sel.Render("▸ "+padded+" "))
			continue
		}
		row := "  " + title.Render(it.title)
		if it.desc != "" {
			row += muted.Render("  —  " + it.desc)
		}
		// Pad to match the selected row's total width (2 marker + maxw + 1).
		cur := 2 + runewidth.StringWidth(plains[i])
		if pad := (2 + maxw + 1) - cur; pad > 0 {
			row += strings.Repeat(" ", pad)
		}
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}

// padLaunchBlock right-pads every line to the block's widest line so the block
// is a rectangle. Centering a rectangle keeps its internal left alignment,
// which lipgloss.JoinVertical(Center) would otherwise break by centering each
// line individually.
func padLaunchBlock(lines []string) string {
	maxw := 0
	for _, ln := range lines {
		if w := lipgloss.Width(ln); w > maxw {
			maxw = w
		}
	}
	out := make([]string, len(lines))
	for i, ln := range lines {
		if pad := maxw - lipgloss.Width(ln); pad > 0 {
			ln += strings.Repeat(" ", pad)
		}
		out[i] = ln
	}
	return strings.Join(out, "\n")
}
