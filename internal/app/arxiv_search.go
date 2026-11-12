package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Han8931/gorae/internal/arxiv"
	"github.com/Han8931/gorae/internal/meta"
)

// arxivPageSize is how many hits one page of results holds. arXiv allows up to
// arxiv.MaxResultsPerPage, but a page the user pages through beats a long list
// they scroll: the top hits are what matter for a topic query.
const arxivPageSize = 25

// arxivSearchResultMsg carries one page of search hits back to the UI. query
// and start are echoed so a result arriving after the user has moved on can be
// matched against what is currently being shown.
type arxivSearchResultMsg struct {
	query string
	start int
	res   *arxiv.SearchResult
	err   error
}

// arxivImportDoneMsg reports the outcome of importing one hit.
type arxivImportDoneMsg struct {
	title string
	path  string
	err   error
}

// arxivSearchCmd queries arXiv off the UI thread.
func arxivSearchCmd(query string, start int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), arxivRequestTimeout)
		defer cancel()
		res, err := arxiv.Search(ctx, query, start, arxivPageSize)
		return arxivSearchResultMsg{query: query, start: start, res: res, err: err}
	}
}

// arxivImportFilename builds the filename a hit is saved under: a readable
// slug of the title, with the arXiv ID appended so two papers with similar
// titles stay distinguishable and the source stays visible in the library.
func arxivImportFilename(md arxiv.Metadata) string {
	id := strings.TrimSpace(md.ID)
	// IDs contain a "/" in arXiv's pre-2007 scheme, which cannot go in a
	// filename.
	id = strings.ReplaceAll(id, "/", "-")
	slug := slugify(md.Title)
	switch {
	case slug != "" && id != "":
		return slug + "-" + id + ".pdf"
	case slug != "":
		return slug + ".pdf"
	case id != "":
		return id + ".pdf"
	default:
		return "arxiv-paper.pdf"
	}
}

// arxivImportCmd downloads a hit's PDF into destDir and records its metadata.
//
// The download lands first and the metadata follows, so a failure partway
// through leaves a readable paper rather than a metadata row pointing at
// nothing. Metadata is written through applyFetchedMetadata, the same path
// ':arxiv <id>' and ':autofetch' use, so an imported paper is indistinguishable
// from one fetched for a file already in the library.
func arxivImportCmd(md arxiv.Metadata, destDir string, store *meta.Store) tea.Cmd {
	return func() tea.Msg {
		dest := avoidNameClash(filepath.Join(destDir, arxivImportFilename(md)))

		ctx, cancel := context.WithTimeout(context.Background(), arxivDownloadTimeout)
		defer cancel()
		if err := arxiv.DownloadPDF(ctx, md.PDFURL, dest); err != nil {
			return arxivImportDoneMsg{title: md.Title, err: err}
		}

		if store != nil {
			canonical := canonicalPath(dest)
			if canonical == "" {
				canonical = dest
			}
			url := ""
			if id := strings.TrimSpace(md.ID); id != "" {
				url = "https://arxiv.org/abs/" + id
			}
			data := &fetchedPaperMetadata{
				Source:     metadataSourceArxiv,
				Identifier: md.ID,
				Title:      md.Title,
				Authors:    md.Authors,
				Year:       md.Year,
				URL:        url,
				DOI:        md.DOI,
				Abstract:   md.Abstract,
			}
			if err := applyFetchedMetadata(ctx, store, canonical, data); err != nil {
				// The PDF is on disk and usable; only its metadata is missing,
				// so report that rather than calling the import a failure.
				return arxivImportDoneMsg{
					title: md.Title,
					path:  dest,
					err:   fmt.Errorf("saved %s but metadata failed: %w", filepath.Base(dest), err),
				}
			}
		}
		return arxivImportDoneMsg{title: md.Title, path: dest}
	}
}

// startArxivSearch kicks off a query and reports progress on the status line.
func (m *Model) startArxivSearch(query string, start int) tea.Cmd {
	query = strings.TrimSpace(query)
	if query == "" {
		m.setStatus("arXiv search query cannot be empty")
		return nil
	}
	m.arxivQuery = query
	m.arxivSearching = true
	m.setStatus("Searching arXiv for " + query + "…")
	return arxivSearchCmd(query, start)
}

// handleArxivSearchResult applies a returned page, opening the results browser.
func (m *Model) handleArxivSearchResult(msg arxivSearchResultMsg) {
	// A page for a query the user has since replaced is stale; dropping it
	// keeps a slow earlier request from overwriting newer results.
	if msg.query != m.arxivQuery {
		return
	}
	m.arxivSearching = false

	if msg.err != nil {
		m.setStatus("arXiv search failed: " + msg.err.Error())
		return
	}
	if msg.res == nil || len(msg.res.Results) == 0 {
		if m.state == stateArxivResults && msg.start > 0 {
			// Paged past the end of a result set that did have hits.
			m.setStatus("No more results")
			return
		}
		m.setStatus("No arXiv results for " + msg.query)
		return
	}

	m.arxivResults = msg.res.Results
	m.arxivTotal = msg.res.Total
	m.arxivStart = msg.start
	m.arxivCursor = 0
	m.state = stateArxivResults
	m.setPersistentStatus("arXiv  ↑/↓ navigate · Enter import · n/p page · Esc close")
}

func (m *Model) exitArxivResults() {
	m.state = stateNormal
	m.arxivResults = nil
	m.arxivCursor = 0
	m.arxivStart = 0
	m.arxivTotal = 0
	m.arxivQuery = ""
	m.arxivSearching = false
	m.setPersistentStatus("")
}

func (m *Model) updateArxivResults(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "esc", "ctrl+c", "q":
		m.exitArxivResults()
		return m, nil
	case "up", "k":
		if m.arxivCursor > 0 {
			m.arxivCursor--
		}
	case "down", "j":
		if m.arxivCursor < len(m.arxivResults)-1 {
			m.arxivCursor++
		}
	case "g":
		m.arxivCursor = 0
	case "G":
		if len(m.arxivResults) > 0 {
			m.arxivCursor = len(m.arxivResults) - 1
		}
	case "n":
		if m.arxivSearching {
			return m, nil
		}
		next := m.arxivStart + arxivPageSize
		if m.arxivTotal > 0 && next >= m.arxivTotal {
			m.setStatus("No more results")
			return m, nil
		}
		return m, m.startArxivSearch(m.arxivQuery, next)
	case "p":
		if m.arxivSearching {
			return m, nil
		}
		if m.arxivStart == 0 {
			m.setStatus("Already on the first page")
			return m, nil
		}
		prev := m.arxivStart - arxivPageSize
		if prev < 0 {
			prev = 0
		}
		return m, m.startArxivSearch(m.arxivQuery, prev)
	case "enter":
		if len(m.arxivResults) == 0 {
			return m, nil
		}
		hit := m.arxivResults[m.arxivCursor]
		destDir := strings.TrimSpace(m.cwd)
		if destDir == "" {
			destDir = m.root
		}
		if destDir == "" {
			m.setStatus("No directory to import into")
			return m, nil
		}
		m.setStatus("Importing " + truncateRunes(hit.Title, 50) + "…")
		return m, arxivImportCmd(hit, destDir, m.meta)
	}
	return m, nil
}

func (m Model) renderArxivResults() string {
	width := m.width
	height := m.viewportHeight
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	labelStyle := m.styles.StatusLabel
	valueStyle := m.styles.StatusValue
	bodyStyle := m.styles.Preview.Body
	sepStyle := m.styles.Separator

	var b strings.Builder

	header := fmt.Sprintf(" ARXIV · %s ", truncateRunes(m.arxivQuery, 40))
	b.WriteString(labelStyle.Render(header))
	if m.arxivTotal > 0 {
		shown := m.arxivStart + len(m.arxivResults)
		b.WriteString(bodyStyle.Render(fmt.Sprintf("  %d–%d of %d", m.arxivStart+1, shown, m.arxivTotal)))
	}
	b.WriteString("\n\n")

	// The abstract of the highlighted hit occupies a fixed block at the bottom,
	// so deciding whether to import does not require opening anything.
	const abstractLines = 4
	listHeight := height - 9 - abstractLines
	if listHeight < 3 {
		listHeight = 3
	}

	offset := 0
	if m.arxivCursor >= listHeight {
		offset = m.arxivCursor - listHeight + 1
	}
	visible := m.arxivResults
	if offset > 0 && offset < len(visible) {
		visible = visible[offset:]
	}
	if len(visible) > listHeight {
		visible = visible[:listHeight]
	}

	const yearCol = 6
	authorCol := width / 4
	if authorCol < 14 {
		authorCol = 14
	}
	titleCol := width - authorCol - yearCol - 8
	if titleCol < 20 {
		titleCol = 20
	}

	for i, hit := range visible {
		idx := i + offset
		authors := formatArxivAuthors(hit.Authors)
		year := ""
		if hit.Year > 0 {
			year = fmt.Sprintf("%d", hit.Year)
		}
		line := fmt.Sprintf("%-*s  %-*s  %*s",
			titleCol, truncateRunes(hit.Title, titleCol-1),
			authorCol, truncateRunes(authors, authorCol-1),
			yearCol, year)
		if idx == m.arxivCursor {
			b.WriteString(valueStyle.Render("▶ " + line))
		} else {
			b.WriteString(bodyStyle.Render("  " + line))
		}
		b.WriteString("\n")
	}
	for i := len(visible); i < listHeight; i++ {
		b.WriteString("\n")
	}

	b.WriteString(sepStyle.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	if len(m.arxivResults) > 0 && m.arxivCursor < len(m.arxivResults) {
		sel := m.arxivResults[m.arxivCursor]
		b.WriteString(bodyStyle.Render("  " + truncateRunes(sel.ID+" · "+sel.Title, width-4)))
		b.WriteString("\n")
		for _, line := range wrapArxivAbstract(sel.Abstract, width-4, abstractLines-1) {
			b.WriteString(bodyStyle.Render("  " + line))
			b.WriteString("\n")
		}
	}

	b.WriteString(sepStyle.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(bodyStyle.Render("  Enter import   n/p page   Esc close"))
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())

	return b.String()
}

// formatArxivAuthors renders an author list for a single column: papers with
// long author lists are common on arXiv, and the first name plus "et al." is
// what identifies the paper.
func formatArxivAuthors(authors []string) string {
	switch len(authors) {
	case 0:
		return ""
	case 1:
		return authors[0]
	case 2:
		return authors[0] + ", " + authors[1]
	default:
		return authors[0] + " et al."
	}
}

// wrapArxivAbstract word-wraps text to width, returning at most maxLines lines
// with an ellipsis when the abstract is longer than the space available.
func wrapArxivAbstract(text string, width, maxLines int) []string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" || width <= 0 || maxLines <= 0 {
		return nil
	}
	var lines []string
	var cur strings.Builder
	for _, word := range strings.Fields(text) {
		switch {
		case cur.Len() == 0:
			cur.WriteString(word)
		case cur.Len()+1+len(word) <= width:
			cur.WriteString(" ")
			cur.WriteString(word)
		default:
			lines = append(lines, cur.String())
			if len(lines) == maxLines {
				// Mark the last line as continuing rather than ending.
				lines[maxLines-1] = truncateRunes(lines[maxLines-1], width-1)
				return lines
			}
			cur.Reset()
			cur.WriteString(word)
		}
	}
	if cur.Len() > 0 && len(lines) < maxLines {
		lines = append(lines, cur.String())
	}
	return lines
}
