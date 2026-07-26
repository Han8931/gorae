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
	searchModeTag     searchMode = "tag"
)

type searchRequest struct {
	root          string
	mode          searchMode
	query         string
	caseSensitive bool
	wrapWidth     int
	metaStore     *meta.Store
	skipDirs      []string
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
	// HitPages holds the 1-based page number for each snippet in Snippets
	// (0 when the page is unknown, e.g. EPUB/Markdown or a fallback snippet).
	// It may be shorter than Snippets when a trailing "(+N more)" line is added.
	HitPages []int
	Meta     pdfMeta
	Title    string
	Year     string
}

type searchAggregate struct {
	matches      []searchMatch
	warnings     []string
	filesMatched int
	totalMatches int
}

// maxHitsPerFile caps how many individual occurrences are turned into navigable
// hits for a single document. It's a safety bound for pathological cases (a
// common word in a whole book); real documents stay well under it.
const maxHitsPerFile = 1000

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

	// For content search, delegate to the FTS index when available.
	if req.mode == searchModeContent && req.metaStore != nil {
		if agg, summary, ok := tryFTSSearch(req); ok {
			return agg, summary, nil
		}
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

	files, walkWarnings, err := collectDocumentFiles(req.root, req.skipDirs)
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

// tryFTSSearch attempts a full-text search via the SQLite FTS5 index.
// Returns (result, summary, true) on success, or (_, _, false) if the index is empty or unusable.
func tryFTSSearch(req searchRequest) (searchAggregate, string, bool) {
	ctx := context.Background()
	count, err := req.metaStore.IndexedCount(ctx)
	if err != nil || count == 0 {
		return searchAggregate{}, "", false
	}

	matches, err := req.metaStore.SearchFTS(ctx, req.query, 200)
	if err != nil {
		// Invalid FTS5 query syntax or other error — fall back to file scan.
		return searchAggregate{}, "", false
	}

	agg := searchAggregate{}
	for _, fm := range matches {
		// FTS5's snippet() only returns one passage per document, so a term that
		// appears many times would show a single hit. Re-scan the indexed body
		// (same pdftotext output that was indexed) to surface every occurrence
		// with its page number, matching the file-scan path's behaviour.
		body, bodyErr := req.metaStore.GetFileContent(ctx, fm.Path)
		if bodyErr == nil && strings.TrimSpace(body) != "" {
			paginated := strings.EqualFold(filepath.Ext(fm.Path), ".pdf")
			if sm, ok := buildContentMatch(fm.Path, body, req.query, false, req.wrapWidth, paginated, req.metaStore); ok {
				agg.matches = append(agg.matches, sm)
				agg.filesMatched++
				agg.totalMatches += sm.MatchCount
				continue
			}
		}
		// Fallback: literal query not found in the body (e.g. a stemmed-only
		// match) or the body is unavailable — keep FTS's single snippet.
		snippet := strings.ReplaceAll(fm.Snippet, "\n", " ")
		sm := searchMatch{
			Path:       fm.Path,
			Mode:       searchModeContent,
			MatchCount: 1,
			Snippets:   []string{snippet},
		}
		populateMatchDisplay(&sm, req.metaStore)
		agg.matches = append(agg.matches, sm)
		agg.filesMatched++
		agg.totalMatches++
	}

	summary := fmt.Sprintf("Content search (FTS): %d file(s), %d match(es) [index: %d files]", agg.filesMatched, agg.totalMatches, count)
	if agg.filesMatched == 0 {
		summary = fmt.Sprintf("Content search (FTS): no matches for %q [index: %d files]", req.query, count)
	}
	return agg, summary, true
}

func collectDocumentFiles(root string, skipDirs []string) ([]string, []string, error) {
	files := make([]string, 0, 32)
	warnings := make([]string, 0)

	rootPath := canonicalPath(root)
	if rootPath == "" {
		rootPath = filepath.Clean(root)
	}
	if rootPath == "" {
		rootPath = "."
	}

	type skipEntry struct {
		path   string
		prefix string
	}
	skipEntries := make([]skipEntry, 0, len(skipDirs))
	for _, dir := range skipDirs {
		dir = canonicalPath(strings.TrimSpace(dir))
		if dir == "" || dir == rootPath {
			continue
		}
		clean := filepath.Clean(dir)
		rel, err := filepath.Rel(rootPath, clean)
		if err != nil {
			continue
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, fmt.Sprintf("..%c", os.PathSeparator)) {
			continue
		}
		skipEntries = append(skipEntries, skipEntry{
			path:   clean,
			prefix: clean + string(os.PathSeparator),
		})
	}

	shouldSkip := func(path string) bool {
		if len(skipEntries) == 0 {
			return false
		}
		clean := filepath.Clean(path)
		for _, entry := range skipEntries {
			if clean == entry.path || strings.HasPrefix(clean, entry.prefix) {
				return true
			}
		}
		return false
	}

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			warnings = append(warnings, fmt.Sprintf("[WARN] %s: %v", path, walkErr))
			return nil
		}
		name := d.Name()
		if shouldSkip(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != rootPath && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		if isBrowsableDocument(name) {
			files = append(files, path)
		}
		return nil
	})
	return files, warnings, err
}

func collectPDFFiles(root string, skipDirs []string) ([]string, []string, error) {
	files, warnings, err := collectDocumentFiles(root, skipDirs)
	if err != nil {
		return nil, warnings, err
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		if strings.EqualFold(filepath.Ext(f), ".pdf") {
			out = append(out, f)
		}
	}
	return out, warnings, nil
}

func evaluatePath(path string, req searchRequest) (searchMatch, bool, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch req.mode {
	case searchModeContent:
		if ext == ".epub" {
			return searchEPUBContent(path, req.query, req.caseSensitive, req.wrapWidth, req.metaStore)
		}
		return searchPDFContent(path, req.query, req.caseSensitive, req.wrapWidth, req.metaStore)
	default:
		if ext == ".epub" {
			return searchEPUBMetadata(path, req.mode, req.query, req.caseSensitive, req.metaStore)
		}
		return searchPDFMetadata(path, req.mode, req.query, req.caseSensitive, req.metaStore)
	}
}

func searchPDFContent(path, query string, caseSensitive bool, wrapWidth int, store *meta.Store) (searchMatch, bool, error) {
	text, err := readPDFText(path)
	if err != nil {
		return searchMatch{}, false, err
	}
	match, ok := buildContentMatch(path, text, query, caseSensitive, wrapWidth, true, store)
	return match, ok, nil
}

func searchEPUBContent(path, query string, caseSensitive bool, wrapWidth int, store *meta.Store) (searchMatch, bool, error) {
	text, err := readEPUBText(path)
	if err != nil {
		return searchMatch{}, false, err
	}
	match, ok := buildContentMatch(path, text, query, caseSensitive, wrapWidth, false, store)
	return match, ok, nil
}

// buildContentMatch turns every occurrence of query within text into its own
// navigable snippet (up to maxHitsPerFile), records each hit's page, and reports
// the true total in MatchCount. When paginated is true the text is treated as
// pdftotext output whose pages are separated by form feeds, so each hit is
// annotated with its 1-based page number. Returns ok=false when query does not
// occur in text.
func buildContentMatch(path, text, query string, caseSensitive bool, wrapWidth int, paginated bool, store *meta.Store) (searchMatch, bool) {
	positions := findAllMatches(text, query, caseSensitive)
	if len(positions) == 0 {
		return searchMatch{}, false
	}
	// The multi-hit box is a scrollable list, so surface every occurrence (up to
	// a generous safety cap) instead of the handful the old static view showed.
	shown := len(positions)
	if shown > maxHitsPerFile {
		shown = maxHitsPerFile
	}
	snippets := make([]string, 0, shown)
	pages := make([]int, 0, shown)
	for i := 0; i < shown; i++ {
		pos := positions[i]
		page := 0
		if paginated {
			page = pageForOffset(text, pos)
		}
		snippet := makeSnippet(text, pos, len(query), query, caseSensitive, wrapWidth)
		if page > 0 {
			snippet = fmt.Sprintf("[p.%d] %s", page, snippet)
		}
		snippets = append(snippets, snippet)
		pages = append(pages, page)
	}
	match := searchMatch{
		Path:       path,
		Mode:       searchModeContent,
		MatchCount: len(positions),
		Snippets:   snippets,
		HitPages:   pages,
	}
	populateMatchDisplay(&match, store)
	return match, true
}

// pageForOffset maps a byte offset within pdftotext output to its 1-based page
// number by counting the form-feed page breaks that precede it. Returns 0 when
// the offset is out of range.
func pageForOffset(text string, offset int) int {
	if offset < 0 || offset > len(text) {
		return 0
	}
	return 1 + strings.Count(text[:offset], "\f")
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
		metaInfo.Tag = strings.TrimSpace(stored.Tag)
	}

	if mode == searchModeTag {
		// Tag searches rely solely on stored metadata; they do not fall back to PDF info.
		if !matchTags(metaInfo.Tag, query, caseSensitive) {
			return searchMatch{}, false, nil
		}
		lines := []string{
			fmt.Sprintf("Title : %s", highlightField(metaInfo.Title, query, caseSensitive)),
			fmt.Sprintf("Author: %s", highlightField(metaInfo.Author, query, caseSensitive)),
			fmt.Sprintf("Tags  : %s", highlightField(metaInfo.Tag, query, caseSensitive)),
		}
		match := searchMatch{
			Path:       path,
			Mode:       mode,
			MatchCount: 1,
			Snippets:   lines,
			Meta:       metaInfo,
		}
		populateMatchDisplay(&match, store)
		return match, true, nil
	}

	var field string
	switch mode {
	case searchModeTitle:
		field = metaInfo.Title
	case searchModeAuthor:
		field = metaInfo.Author
	case searchModeTag:
		field = metaInfo.Tag
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
		if mode == searchModeTitle {
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if strings.TrimSpace(base) != "" {
				field = base
				if strings.TrimSpace(metaInfo.Title) == "" {
					metaInfo.Title = base
				}
			}
		}
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
		fmt.Sprintf("Tag          : %s", highlightField(metaInfo.Tag, query, caseSensitive)),
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
	populateMatchDisplay(&match, store)
	return match, true, nil
}

func searchEPUBMetadata(path string, mode searchMode, query string, caseSensitive bool, store *meta.Store) (searchMatch, bool, error) {
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
	if stored != nil {
		metaInfo.Title = strings.TrimSpace(stored.Title)
		metaInfo.Author = strings.TrimSpace(stored.Author)
		metaInfo.Tag = strings.TrimSpace(stored.Tag)
		metaInfo.CreationDate = strings.TrimSpace(stored.Year)
	}
	if strings.TrimSpace(metaInfo.Title) == "" || strings.TrimSpace(metaInfo.Author) == "" || strings.TrimSpace(metaInfo.CreationDate) == "" {
		if parsed, err := parseEPUBMetadata(path); err == nil {
			if metaInfo.Title == "" && parsed.Title != "" {
				metaInfo.Title = parsed.Title
			}
			if metaInfo.Author == "" && parsed.Author != "" {
				metaInfo.Author = parsed.Author
			}
			if metaInfo.CreationDate == "" && parsed.Year != "" {
				metaInfo.CreationDate = parsed.Year
			}
		}
	}

	var field string
	switch mode {
	case searchModeTitle:
		field = metaInfo.Title
	case searchModeAuthor:
		field = metaInfo.Author
	case searchModeYear:
		field = metaInfo.CreationDate
	case searchModeTag:
		field = metaInfo.Tag
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
		fmt.Sprintf("Year         : %s", highlightField(metaInfo.CreationDate, query, caseSensitive)),
		fmt.Sprintf("Tag          : %s", highlightField(metaInfo.Tag, query, caseSensitive)),
	}

	match := searchMatch{
		Path:       path,
		Mode:       mode,
		MatchCount: 1,
		Snippets:   lines,
		Meta:       metaInfo,
	}
	populateMatchDisplay(&match, store)
	return match, true, nil
}

type pdfMeta struct {
	Title        string
	Author       string
	Tag          string
	CreationDate string
	ModDate      string
	Year         string
}

func populateMatchDisplay(match *searchMatch, store *meta.Store) {
	if match == nil {
		return
	}
	title := strings.TrimSpace(match.Title)
	if title == "" {
		title = strings.TrimSpace(match.Meta.Title)
	}
	year := strings.TrimSpace(match.Year)
	if year == "" {
		year = firstYearFromMeta(match.Meta)
	}
	if store != nil && (title == "" || year == "") {
		ctx := context.Background()
		data, err := store.Get(ctx, canonicalPath(match.Path))
		if err == nil && data != nil {
			if title == "" {
				title = strings.TrimSpace(data.Title)
			}
			if year == "" {
				year = strings.TrimSpace(data.Year)
			}
		}
	}
	if title == "" {
		if match.Path != "" {
			if base := filepath.Base(match.Path); base != "" {
				title = base
			}
		}
		if title == "" {
			title = untitledPlaceholder
		}
	}
	match.Title = title
	match.Year = year
}

func firstYearFromMeta(meta pdfMeta) string {
	candidates := []string{meta.Year, meta.CreationDate, meta.ModDate}
	for _, candidate := range candidates {
		if year := extractMatchYear(candidate); year != "" {
			return year
		}
	}
	return ""
}

var searchYearPattern = regexp.MustCompile(`\b(19|20)\d{2}\b`)

func extractMatchYear(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	loc := searchYearPattern.FindString(value)
	return loc
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

func matchTags(stored, query string, caseSensitive bool) bool {
	storedTags := splitTags(stored)
	queryTags := splitTags(query)
	if len(storedTags) == 0 || len(queryTags) == 0 {
		return false
	}
	for _, q := range queryTags {
		qNorm := q
		if !caseSensitive {
			qNorm = strings.ToLower(q)
		}
		for _, t := range storedTags {
			tNorm := t
			if !caseSensitive {
				tNorm = strings.ToLower(t)
			}
			// Exact match or prefix match: query "ml" matches stored "ml/transformers".
			if tNorm == qNorm || strings.HasPrefix(tNorm, qNorm+"/") || strings.Contains(tNorm, qNorm) {
				return true
			}
		}
	}
	return false
}

func splitTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
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

var (
	snippetPunctPattern = regexp.MustCompile(`([.,;:!?])([^\s])`)
	snippetCamelPattern = regexp.MustCompile(`([a-z])([A-Z])`)
)

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
	snippet = snippetPunctPattern.ReplaceAllString(snippet, "$1 $2")
	snippet = snippetCamelPattern.ReplaceAllString(snippet, "$1 $2")
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
