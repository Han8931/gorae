package app

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"gorae/internal/meta"
)

type searchMode string

const (
	searchModeTitle   searchMode = "title"
	searchModeAuthor  searchMode = "author"
	searchModeYear    searchMode = "year"
	searchModeContent searchMode = "content"
)

type searchRequest struct {
	root          string
	mode          searchMode
	query         string
	caseSensitive bool
	wrapWidth     int
	metaStore     *meta.Store
}

type searchResultMsg struct {
	req          searchRequest
	matches      []searchMatch
	warnings     []string
	filesMatched int
	totalMatches int
	summary      string
	err          error
}

type searchMatch struct {
	Path       string
	Mode       searchMode
	MatchCount int
	Snippets   []string
	Meta       pdfMeta
}

type searchAggregate struct {
	matches      []searchMatch
	warnings     []string
	filesMatched int
	totalMatches int
}

const maxSnippetsPerFile = 6

func (m searchMode) label() string {
	if m == "" {
		return "content"
	}
	return string(m)
}

func (m searchMode) displayName() string {
	label := m.label()
	if label == "" {
		return "Content"
	}
	return strings.ToUpper(label[:1]) + label[1:]
}

func newSearchCmd(req searchRequest) tea.Cmd {
	return func() tea.Msg {
		agg, summary, err := performSearch(req)
		return searchResultMsg{
			req:          req,
			matches:      agg.matches,
			warnings:     agg.warnings,
			filesMatched: agg.filesMatched,
			totalMatches: agg.totalMatches,
			summary:      summary,
			err:          err,
		}
	}
}

func performSearch(req searchRequest) (searchAggregate, string, error) {
	if strings.TrimSpace(req.query) == "" {
		return searchAggregate{}, "", fmt.Errorf("empty query")
	}

	info, err := os.Stat(req.root)
	if err != nil {
		return searchAggregate{}, "", fmt.Errorf("search root: %w", err)
	}
	if !info.IsDir() {
		return searchAggregate{}, "", fmt.Errorf("search root %s is not a directory", req.root)
	}

	wrapWidth := req.wrapWidth
	if wrapWidth <= 0 {
		wrapWidth = 80
	}

	files, walkWarnings, err := collectPDFFiles(req.root)
	if err != nil {
		return searchAggregate{}, "", err
	}

	agg := searchAggregate{}
	if len(walkWarnings) > 0 {
		agg.warnings = append(agg.warnings, walkWarnings...)
	}

	if len(files) == 0 {
		summary := fmt.Sprintf("%s search: no PDFs found under %s", req.mode.displayName(), req.root)
		if len(agg.warnings) > 0 {
			summary += fmt.Sprintf(" [%d warning(s)]", len(agg.warnings))
		}
		return agg, summary, nil
	}

	workerCount := runtime.NumCPU()
	if workerCount < 2 {
		workerCount = 2
	}
	jobs := make(chan string, workerCount*2)
	var wg sync.WaitGroup
	var aggMu sync.Mutex

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				match, matched, err := evaluatePath(path, req)
				if err != nil {
					aggMu.Lock()
					agg.warnings = append(agg.warnings, fmt.Sprintf("[WARN] %s: %v", path, err))
					aggMu.Unlock()
					continue
				}
				if matched {
					aggMu.Lock()
					agg.matches = append(agg.matches, match)
					agg.filesMatched++
					agg.totalMatches += match.MatchCount
					aggMu.Unlock()
				}
			}
		}()
	}

	for _, path := range files {
		jobs <- path
	}
	close(jobs)
	wg.Wait()

	summary := formatSearchSummary(req, agg)
	return agg, summary, nil
}

func formatSearchSummary(req searchRequest, agg searchAggregate) string {
	if agg.filesMatched == 0 {
		summary := fmt.Sprintf("%s search: no matches for %q", req.mode.displayName(), req.query)
		if len(agg.warnings) > 0 {
			summary += fmt.Sprintf(" [%d warning(s)]", len(agg.warnings))
		}
		return summary
	}
	summary := fmt.Sprintf("%s search: %d file(s) matched", req.mode.displayName(), agg.filesMatched)
	if req.mode == searchModeContent {
		summary = fmt.Sprintf("%s search: %d file(s), %d match(es)", req.mode.displayName(), agg.filesMatched, agg.totalMatches)
	}
	if len(agg.warnings) > 0 {
		summary += fmt.Sprintf(" [%d warning(s)]", len(agg.warnings))
	}
	return summary
}

func collectPDFFiles(root string) ([]string, []string, error) {
	files := make([]string, 0, 32)
	warnings := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			warnings = append(warnings, fmt.Sprintf("[WARN] %s: %v", path, walkErr))
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		if ext := filepath.Ext(name); ext != "" && strings.EqualFold(ext, ".pdf") {
			files = append(files, path)
		}
		return nil
	})
	return files, warnings, err
}

func evaluatePath(path string, req searchRequest) (searchMatch, bool, error) {
	switch req.mode {
	case searchModeContent:
		return searchPDFContent(path, req.query, req.caseSensitive, req.wrapWidth)
	default:
		return searchPDFMetadata(path, req.mode, req.query, req.caseSensitive, req.metaStore)
	}
}

func searchPDFContent(path, query string, caseSensitive bool, wrapWidth int) (searchMatch, bool, error) {
	text, err := readPDFText(path)
	if err != nil {
		return searchMatch{}, false, err
	}
	positions := findAllMatches(text, query, caseSensitive)
	if len(positions) == 0 {
		return searchMatch{}, false, nil
	}
	maxSnippets := maxSnippetsPerFile
	if maxSnippets > len(positions) {
		maxSnippets = len(positions)
	}
	snippets := make([]string, 0, maxSnippets)
	for i := 0; i < maxSnippets; i++ {
		pos := positions[i]
		snippet := makeSnippet(text, pos, len(query), query, caseSensitive, wrapWidth)
		snippets = append(snippets, snippet)
	}
	if len(positions) > maxSnippetsPerFile {
		extra := len(positions) - maxSnippetsPerFile
		snippets = append(snippets, fmt.Sprintf("(+%d more match(es) in this file)", extra))
	}
	match := searchMatch{
		Path:       path,
		Mode:       searchModeContent,
		MatchCount: len(positions),
		Snippets:   snippets,
	}
	return match, true, nil
}

func searchPDFMetadata(path string, mode searchMode, query string, caseSensitive bool, store *meta.Store) (searchMatch, bool, error) {
	var stored *meta.Metadata
	canonical := canonicalPath(path)
	if store != nil {
		ctx := context.Background()
		data, err := store.Get(ctx, canonical)
		if err != nil {
			return searchMatch{}, false, fmt.Errorf("load metadata: %w", err)
		}
		if data != nil {
			stored = data
		}
	}

	metaInfo := pdfMeta{}
	needPDFInfo := mode == searchModeYear
	if stored != nil {
		metaInfo.Title = strings.TrimSpace(stored.Title)
		metaInfo.Author = strings.TrimSpace(stored.Author)
	}

	var field string
	switch mode {
	case searchModeTitle:
		field = metaInfo.Title
	case searchModeAuthor:
		field = metaInfo.Author
	}

	if strings.TrimSpace(field) == "" || needPDFInfo {
		pdfInfo, err := readPDFInfo(path)
		if err != nil {
			return searchMatch{}, false, err
		}
		if strings.TrimSpace(metaInfo.Title) == "" {
			metaInfo.Title = pdfInfo.Title
		}
		if strings.TrimSpace(metaInfo.Author) == "" {
			metaInfo.Author = pdfInfo.Author
		}
		metaInfo.CreationDate = pdfInfo.CreationDate
		metaInfo.ModDate = pdfInfo.ModDate
		if strings.TrimSpace(field) == "" {
			switch mode {
			case searchModeTitle:
				field = metaInfo.Title
			case searchModeAuthor:
				field = metaInfo.Author
			case searchModeYear:
				field = metaInfo.CreationDate + " " + metaInfo.ModDate
			default:
				field = metaInfo.Title
			}
		}
	} else if mode == searchModeYear {
		field = metaInfo.CreationDate + " " + metaInfo.ModDate
	}

	if strings.TrimSpace(field) == "" {
		return searchMatch{}, false, nil
	}

	target := field
	needle := query
	if !caseSensitive {
		target = strings.ToLower(target)
		needle = strings.ToLower(needle)
	}
	if !strings.Contains(target, needle) {
		return searchMatch{}, false, nil
	}

	lines := []string{
		fmt.Sprintf("Title        : %s", highlightField(metaInfo.Title, query, caseSensitive)),
		fmt.Sprintf("Author       : %s", highlightField(metaInfo.Author, query, caseSensitive)),
		fmt.Sprintf("CreationDate : %s", highlightField(metaInfo.CreationDate, query, caseSensitive)),
		fmt.Sprintf("ModDate      : %s", highlightField(metaInfo.ModDate, query, caseSensitive)),
	}

	match := searchMatch{
		Path:       path,
		Mode:       mode,
		MatchCount: 1,
		Snippets:   lines,
		Meta:       metaInfo,
	}
	return match, true, nil
}

type pdfMeta struct {
	Title        string
	Author       string
	CreationDate string
	ModDate      string
}

func highlightField(value, query string, caseSensitive bool) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "(empty)"
	}
	if strings.TrimSpace(query) == "" {
		return trimmed
	}
	return highlight(trimmed, query, caseSensitive)
}

func readPDFInfo(path string) (pdfMeta, error) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		return pdfMeta{}, fmt.Errorf("pdfinfo not installed (install via poppler)")
	}

	cmd := exec.Command("pdfinfo", path)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return pdfMeta{}, fmt.Errorf("pdfinfo: %w (%s)", err, errMsg)
		}
		return pdfMeta{}, fmt.Errorf("pdfinfo: %w", err)
	}
	return parsePDFInfo(stdout.String()), nil
}

func parsePDFInfo(output string) pdfMeta {
	meta := pdfMeta{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		switch key {
		case "title":
			meta.Title = value
		case "author":
			meta.Author = value
		case "creationdate":
			meta.CreationDate = value
		case "moddate":
			meta.ModDate = value
		}
	}
	return meta
}

func readPDFText(path string) (string, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return "", fmt.Errorf("pdftotext not installed (install via poppler)")
	}
	cmd := exec.Command("pdftotext", "-layout", path, "-")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("pdftotext: %w (%s)", err, errMsg)
		}
		return "", fmt.Errorf("pdftotext: %w", err)
	}
	return stdout.String(), nil
}

func findAllMatches(text, query string, caseSensitive bool) []int {
	if query == "" {
		return nil
	}
	var haystack, needle string
	if caseSensitive {
		haystack = text
		needle = query
	} else {
		haystack = strings.ToLower(text)
		needle = strings.ToLower(query)
	}

	var positions []int
	from := 0
	step := len(needle)

	for {
		idx := strings.Index(haystack[from:], needle)
		if idx < 0 {
			break
		}
		positions = append(positions, from+idx)
		from += idx + step
	}
	return positions
}

func makeSnippet(text string, idx, matchLen int, query string, caseSensitive bool, wrapWidth int) string {
	const context = 80
	if idx < 0 {
		return ""
	}
	start := idx - context
	if start < 0 {
		start = 0
	}
	end := idx + matchLen + context
	if end > len(text) {
		end = len(text)
	}
	snippet := text[start:end]
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	snippet = strings.Join(strings.Fields(snippet), " ")
	rePunct := regexp.MustCompile(`([.,;:!?])([^\s])`)
	snippet = rePunct.ReplaceAllString(snippet, "$1 $2")
	reCamel := regexp.MustCompile(`([a-z])([A-Z])`)
	snippet = reCamel.ReplaceAllString(snippet, "$1 $2")
	snippet = highlight(snippet, query, caseSensitive)
	snippet = wrapSnippet(snippet, wrapWidth)
	return snippet
}

func highlight(s, query string, caseSensitive bool) string {
	if query == "" {
		return s
	}
	const (
		startHL = "\033[1;31m"
		endHL   = "\033[0m"
	)
	if !caseSensitive {
		lowerS := strings.ToLower(s)
		lowerQ := strings.ToLower(query)
		var b strings.Builder
		i := 0
		for {
			j := strings.Index(lowerS[i:], lowerQ)
			if j < 0 {
				b.WriteString(s[i:])
				break
			}
			b.WriteString(s[i : i+j])
			b.WriteString(startHL)
			b.WriteString(s[i+j : i+j+len(query)])
			b.WriteString(endHL)
			i += j + len(query)
		}
		return b.String()
	}
	var b strings.Builder
	i := 0
	for {
		j := strings.Index(s[i:], query)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i : i+j])
		b.WriteString(startHL)
		b.WriteString(s[i+j : i+j+len(query)])
		b.WriteString(endHL)
		i += j + len(query)
	}
	return b.String()
}

func wrapSnippet(s string, width int) string {
	if width <= 0 {
		width = 80
	}
	var out []string
	for len(s) > width {
		cut := width
		for cut > 0 && s[cut-1] != ' ' {
			cut--
		}
		if cut == 0 {
			cut = width
		}
		out = append(out, strings.TrimSpace(s[:cut]))
		s = strings.TrimLeft(s[cut:], " ")
	}
	if len(s) > 0 {
		out = append(out, strings.TrimSpace(s))
	}
	return strings.Join(out, "\n  ")
}
