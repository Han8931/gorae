package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Han8931/gorae/internal/arxiv"
	"github.com/Han8931/gorae/internal/crossref"
	"github.com/Han8931/gorae/internal/meta"
)

const (
	autoMetadataMaxPages             = 4
	autoMetadataScanInterval         = 10 * time.Minute
	autoMetadataInitialScanDelay     = 3 * time.Second
	autoMetadataRetrySuccessInterval = 24 * time.Hour
	autoMetadataRetryFailureInterval = time.Hour
	autoMetadataBatchSize            = 0 // 0 = no limit; process all eligible
)

type metadataSource string

const (
	metadataSourceDOI   metadataSource = "doi"
	metadataSourceArxiv metadataSource = "arxiv"
)

type autoMetadataMsg struct {
	Results []autoMetadataResult
}

type autoMetadataScanMsg struct{}

type autoMetadataResult struct {
	Path       string
	Identifier string
	Source     metadataSource
	Err        error
}

type fetchedPaperMetadata struct {
	Source     metadataSource
	Identifier string
	Title      string
	Authors    []string
	Published  string
	Year       int
	URL        string
	DOI        string
	Abstract   string
}

type paperIdentifiers struct {
	DOI   string
	Arxiv string
}

var (
	doiURLPattern      = regexp.MustCompile(`(?i)https?://(?:dx\.)?doi\.org/(10\.\d{4,9}/[-._;()/:a-z0-9]+)`)
	doiPrefixedPattern = regexp.MustCompile(`(?i)\bdoi[:\s]+(10\.\d{4,9}/[-._;()/:a-z0-9]+)`)
	doiBarePattern     = regexp.MustCompile(`(?i)\b10\.\d{4,9}/[-._;()/:a-z0-9]+\b`)
)

func (m *Model) handleAutoMetadataCommand(args []string) tea.Cmd {
	if m.meta == nil {
		m.setStatus("Metadata store not available")
		return nil
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		m.setStatus("pdftotext required for auto metadata (install via poppler)")
		return nil
	}
	files, err := m.resolveAutoMetadataTargets(args)
	if err != nil {
		m.setStatus(err.Error())
		return nil
	}
	if len(files) == 0 {
		m.setStatus("Auto metadata works on PDF files only; select or specify a PDF")
		return nil
	}
	m.setPersistentStatus(fmt.Sprintf("Detecting metadata for %d file(s)...", len(files)))
	return m.runAutoMetadata(files)
}

func (m *Model) resolveAutoMetadataTargets(args []string) ([]string, error) {
	useSelection := false
	fileSpecs := make([]string, 0, len(args))
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		if arg == "" {
			continue
		}
		lower := strings.ToLower(arg)
		switch lower {
		case "-v", "--visual", "--selected":
			useSelection = true
			continue
		}
		if strings.HasPrefix(lower, "-") {
			return nil, fmt.Errorf("Unknown option: %s", arg)
		}
		fileSpecs = append(fileSpecs, arg)
	}

	var files []string
	if len(fileSpecs) > 0 {
		files = make([]string, 0, len(fileSpecs))
		for _, spec := range fileSpecs {
			resolved, err := m.resolveCommandFilePath(spec)
			if err != nil {
				return nil, err
			}
			if !isPDF(resolved) {
				return nil, fmt.Errorf("%s is not a PDF", filepath.Base(resolved))
			}
			files = append(files, resolved)
		}
	} else {
		var targets []string
		if useSelection {
			targets = m.selectedPaths()
			if len(targets) == 0 {
				return nil, fmt.Errorf("Select at least one PDF first")
			}
		} else {
			targets = m.selectionOrCurrent()
			if len(targets) == 0 {
				return nil, fmt.Errorf("No files selected")
			}
		}
		files = filterPDFPaths(targets)
		if len(files) == 0 {
			return nil, fmt.Errorf("Auto metadata works on PDF files only; select a PDF")
		}
	}
	return uniquePaths(files), nil
}

func (m *Model) runAutoMetadata(files []string) tea.Cmd {
	store := m.meta
	paths := append([]string{}, files...)
	return func() tea.Msg {
		ctx := context.Background()
		results := make([]autoMetadataResult, 0, len(paths))
		for _, path := range paths {
			data, err := detectMetadataForFile(path)
			res := autoMetadataResult{Path: path}
			if err != nil {
				res.Err = err
				results = append(results, res)
				continue
			}
			if err := applyFetchedMetadata(ctx, store, path, data); err != nil {
				res.Err = err
			} else {
				res.Identifier = data.Identifier
				res.Source = data.Source
			}
			results = append(results, res)
		}
		return autoMetadataMsg{Results: results}
	}
}

func scheduleAutoMetadataScan(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg { return autoMetadataScanMsg{} })
}

func (m *Model) autoMetadataCmdForMissing() tea.Cmd {
	if m == nil || m.meta == nil {
		return nil
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return nil
	}
	root := m.root
	if strings.TrimSpace(root) == "" {
		root = m.cwd
	}

	skipDirs := m.autoMetadataSkipDirs()

	files, _, err := collectPDFFiles(root, skipDirs)
	if err != nil || len(files) == 0 {
		return nil
	}

	now := time.Now()
	limit := autoMetadataBatchSize
	if limit <= 0 || limit > len(files) {
		limit = len(files)
	}
	targets := make([]string, 0, limit)

	for _, full := range files {
		if len(targets) >= limit {
			break
		}
		canonical := canonicalPath(full)
		if canonical == "" {
			continue
		}
		if !m.canAttemptAutoMetadata(canonical, now) {
			continue
		}
		md, err := m.meta.Get(context.Background(), canonical)
		if err != nil {
			continue
		}
		if md != nil && strings.TrimSpace(md.Title) != "" {
			continue
		}

		targets = append(targets, full)
	}

	if len(targets) == 0 {
		return nil
	}

	return m.runAutoMetadata(targets)
}

func (m *Model) canAttemptAutoMetadata(path string, now time.Time) bool {
	if m.autoMetadataAttempts == nil {
		return true
	}
	if ts, ok := m.autoMetadataAttempts[path]; ok {
		return now.After(ts)
	}
	return true
}

func (m *Model) recordAutoMetadataAttempt(path string, ts time.Time) {
	if m.autoMetadataAttempts == nil {
		m.autoMetadataAttempts = make(map[string]time.Time)
	}
	m.autoMetadataAttempts[path] = ts
}

func (m *Model) recordAutoMetadataResult(path string, success bool, now time.Time) {
	if strings.TrimSpace(path) == "" {
		return
	}
	dur := autoMetadataRetryFailureInterval
	if success {
		dur = autoMetadataRetrySuccessInterval
	}
	m.recordAutoMetadataAttempt(path, now.Add(dur))
}

func (m *Model) autoMetadataSkipDirs() []string {
	var skip []string
	if m.recentlyOpenedDir != "" {
		skip = append(skip, m.recentlyOpenedDir)
	}
	if m.recentlyAddedDir != "" {
		skip = append(skip, m.recentlyAddedDir)
	}
	if m.favoritesDir != "" {
		skip = append(skip, m.favoritesDir)
	}
	if m.toReadDir != "" {
		skip = append(skip, m.toReadDir)
	}
	if m.notesDir != "" {
		skip = append(skip, m.notesDir)
	}
	return skip
}

func detectMetadataForFile(path string) (*fetchedPaperMetadata, error) {
	text, err := samplePDFText(path, autoMetadataMaxPages)
	if err != nil {
		return nil, err
	}
	ids := extractIdentifiersFromText(text)
	if ids.Arxiv == "" {
		if id := extractArxivIDFromFilename(path); id != "" {
			ids.Arxiv = id
		}
	}
	if ids.DOI == "" && ids.Arxiv == "" {
		return nil, fmt.Errorf("no DOI or arXiv identifier detected")
	}

	var lastErr error
	if ids.DOI != "" {
		meta, err := fetchDOIMetadata(ids.DOI)
		if err == nil {
			return meta, nil
		}
		lastErr = fmt.Errorf("DOI %s: %w", ids.DOI, err)
	}
	if ids.Arxiv != "" {
		meta, err := fetchArxivMetadata(ids.Arxiv)
		if err == nil {
			return meta, nil
		}
		if lastErr != nil {
			return nil, fmt.Errorf("%v; arXiv %s: %w", lastErr, ids.Arxiv, err)
		}
		return nil, fmt.Errorf("arXiv %s: %w", ids.Arxiv, err)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no DOI or arXiv identifier detected")
}

func samplePDFText(path string, maxPages int) (string, error) {
	limit := maxPages
	if limit <= 0 {
		limit = 3
	}
	args := []string{
		"-f", "1",
		"-l", strconv.Itoa(limit),
		"-layout",
		path,
		"-",
	}
	cmd := exec.Command("pdftotext", args...)
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

func fetchDOIMetadata(doi string) (*fetchedPaperMetadata, error) {
	ctx, cancel := context.WithTimeout(context.Background(), arxivRequestTimeout)
	defer cancel()
	meta, err := crossref.Fetch(ctx, doi)
	if err != nil {
		return nil, err
	}
	return &fetchedPaperMetadata{
		Source:     metadataSourceDOI,
		Identifier: meta.DOI,
		Title:      meta.Title,
		Authors:    meta.Authors,
		Published:  meta.Published,
		Year:       meta.Year,
		URL:        meta.URL,
		DOI:        meta.DOI,
		Abstract:   meta.Abstract,
	}, nil
}

func fetchArxivMetadata(id string) (*fetchedPaperMetadata, error) {
	ctx, cancel := context.WithTimeout(context.Background(), arxivRequestTimeout)
	defer cancel()
	meta, err := arxiv.Fetch(ctx, id)
	if err != nil {
		return nil, err
	}
	url := ""
	if meta.ID != "" {
		url = fmt.Sprintf("https://arxiv.org/abs/%s", strings.TrimSpace(meta.ID))
	}
	return &fetchedPaperMetadata{
		Source:     metadataSourceArxiv,
		Identifier: meta.ID,
		Title:      meta.Title,
		Authors:    meta.Authors,
		Year:       meta.Year,
		DOI:        meta.DOI,
		URL:        url,
		Abstract:   meta.Abstract,
	}, nil
}

func applyFetchedMetadata(ctx context.Context, store *meta.Store, path string, data *fetchedPaperMetadata) error {
	existing, err := store.Get(ctx, path)
	if err != nil {
		return fmt.Errorf("load metadata for %s: %w", filepath.Base(path), err)
	}
	md := meta.Metadata{Path: path}
	if existing != nil {
		md = *existing
	}
	if strings.TrimSpace(data.Title) != "" {
		md.Title = data.Title
	}
	if len(data.Authors) > 0 {
		md.Author = strings.Join(data.Authors, ", ")
	}
	if data.Published != "" {
		md.Published = data.Published
	}
	if data.Year > 0 {
		md.Year = strconv.Itoa(data.Year)
	}
	if strings.TrimSpace(data.URL) != "" {
		md.URL = data.URL
	}
	if strings.TrimSpace(data.DOI) != "" {
		md.DOI = data.DOI
	}
	if strings.TrimSpace(data.Abstract) != "" {
		md.Abstract = data.Abstract
	}
	return store.Upsert(ctx, &md)
}

func extractIdentifiersFromText(text string) paperIdentifiers {
	ids := paperIdentifiers{}
	if doi := extractDOIFromText(text); doi != "" {
		ids.DOI = doi
	}
	if arxiv := extractArxivIDFromString(text); arxiv != "" {
		ids.Arxiv = arxiv
	}
	return ids
}

func extractDOIFromText(text string) string {
	if match := doiURLPattern.FindStringSubmatch(text); len(match) > 1 {
		if doi := sanitizeDetectedDOI(match[1]); doi != "" {
			return doi
		}
	}
	if match := doiPrefixedPattern.FindStringSubmatch(text); len(match) > 1 {
		if doi := sanitizeDetectedDOI(match[1]); doi != "" {
			return doi
		}
	}
	if match := doiBarePattern.FindString(text); match != "" {
		return sanitizeDetectedDOI(match)
	}
	return ""
}

func sanitizeDetectedDOI(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	trimmed = strings.Trim(trimmed, ".,;\"'<>[]()")
	trimmed = strings.TrimSpace(trimmed)
	return trimmed
}

func extractArxivIDFromString(input string) string {
	if input == "" {
		return ""
	}
	if match := arxivModernIDPattern.FindStringSubmatch(input); len(match) > 0 {
		return normalizeArxivMatch(match)
	}
	if match := arxivLegacyIDPattern.FindStringSubmatch(input); len(match) > 0 {
		return normalizeArxivMatch(match)
	}
	return ""
}

func filterPDFPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		if !isDocument(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func isPDF(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".pdf")
}

func isEPUB(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".epub")
}

func isDocument(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".pdf" || ext == ".epub"
}
