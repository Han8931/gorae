package app

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"gorae/internal/meta"
)

type metadataFlag int

const (
	metadataFlagFavorite metadataFlag = iota
	metadataFlagToRead
)

type unmarkChoice int

const (
	unmarkChoiceFavorite unmarkChoice = iota
	unmarkChoiceToRead
	unmarkChoiceBoth
)

func (m *Model) toggleMetadataFlag(flag metadataFlag) {
	if m.meta == nil {
		m.setStatus("Metadata store not available")
		return
	}
	targets := m.selectionOrCurrent()
	paths := m.canonicalFilePaths(targets)
	if len(paths) == 0 {
		m.setStatus("No files selected")
		return
	}
	ctx := context.Background()
	turnedOn := 0
	turnedOff := 0
	refreshPreview := false
	for _, path := range paths {
		md, err := m.loadMetadataRecord(ctx, path)
		if err != nil {
			m.setStatus("Failed to load metadata: " + err.Error())
			return
		}
		switch flag {
		case metadataFlagFavorite:
			md.Favorite = !md.Favorite
			if md.Favorite {
				turnedOn++
			} else {
				turnedOff++
			}
		case metadataFlagToRead:
			md.ToRead = !md.ToRead
			if md.ToRead {
				turnedOn++
			} else {
				turnedOff++
			}
		}
		if err := m.meta.Upsert(ctx, &md); err != nil {
			m.setStatus("Failed to save metadata: " + err.Error())
			return
		}
		m.refreshMetadataCache(path, md)
		if path == m.currentEntryPath() {
			refreshPreview = true
		}
	}
	if refreshPreview {
		m.updateTextPreview()
	}
	label := "Favorite"
	if flag == metadataFlagToRead {
		label = "To-read"
	}
	m.setStatus(fmt.Sprintf("%s toggled (%d on, %d off)", label, turnedOn, turnedOff))
}

func (m *Model) loadMetadataRecord(ctx context.Context, path string) (meta.Metadata, error) {
	existing, err := m.meta.Get(ctx, path)
	if err != nil {
		return meta.Metadata{}, err
	}
	if existing != nil {
		existing.ReadingState = normalizeReadingStateValue(existing.ReadingState)
		return *existing, nil
	}
	return meta.Metadata{Path: path, ReadingState: readingStateUnread}, nil
}

func (m *Model) refreshMetadataCache(path string, md meta.Metadata) {
	if m.currentMetaPath == path {
		updated := md
		m.currentMeta = &updated
	}
}

func (m *Model) cycleReadingState() {
	if m.meta == nil {
		m.setStatus("Metadata store not available")
		return
	}
	targets := m.selectionOrCurrent()
	paths := m.canonicalFilePaths(targets)
	if len(paths) == 0 {
		m.setStatus("No files selected")
		return
	}
	ctx := context.Background()
	countByState := make(map[string]int, 3)
	refreshPreview := false
	for _, path := range paths {
		md, err := m.loadMetadataRecord(ctx, path)
		if err != nil {
			m.setStatus("Failed to load metadata: " + err.Error())
			return
		}
		md.ReadingState = nextReadingState(md.ReadingState)
		countByState[md.ReadingState]++
		if err := m.meta.Upsert(ctx, &md); err != nil {
			m.setStatus("Failed to save metadata: " + err.Error())
			return
		}
		m.refreshMetadataCache(path, md)
		if path == m.currentEntryPath() {
			refreshPreview = true
		}
	}
	m.refreshEntryTitles()
	if refreshPreview {
		m.updateTextPreview()
	}
	summary := make([]string, 0, len(countByState))
	for _, state := range []string{readingStateUnread, readingStateReading, readingStateRead} {
		if count := countByState[state]; count > 0 {
			summary = append(summary, fmt.Sprintf("%s: %d", readingStateLabel(state), count))
		}
	}
	if len(summary) == 0 {
		m.setStatus("Reading state unchanged")
		return
	}
	m.setStatus("Reading state -> " + strings.Join(summary, ", "))
}

func (m *Model) canonicalFilePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		canonical := canonicalPath(raw)
		if canonical == "" {
			continue
		}
		info, err := os.Stat(canonical)
		if err != nil || info.IsDir() {
			continue
		}
		if _, exists := unique[canonical]; exists {
			continue
		}
		unique[canonical] = struct{}{}
		out = append(out, canonical)
	}
	sort.Strings(out)
	return out
}

func (m *Model) startUnmarkPrompt() {
	if m.meta == nil {
		m.setStatus("Metadata store not available")
		return
	}
	targets := m.selectionOrCurrent()
	paths := m.canonicalFilePaths(targets)
	if len(paths) == 0 {
		m.setStatus("No files selected")
		return
	}
	m.unmarkTargets = paths
	m.state = stateUnmarkPrompt
	m.setPersistentStatus("Unmark: f favorite, t to-read, b both, Esc cancel")
}

func (m *Model) applyUnmark(choice unmarkChoice) {
	if m.meta == nil {
		m.setStatus("Metadata store not available")
		m.state = stateNormal
		m.unmarkTargets = nil
		return
	}
	paths := append([]string{}, m.unmarkTargets...)
	m.unmarkTargets = nil
	m.state = stateNormal
	if len(paths) == 0 {
		m.setStatus("No files selected")
		return
	}
	ctx := context.Background()
	clearedFavorite := 0
	clearedToRead := 0
	refreshPreview := false
	for _, path := range paths {
		md, err := m.loadMetadataRecord(ctx, path)
		if err != nil {
			m.setStatus("Failed to load metadata: " + err.Error())
			return
		}
		changed := false
		if (choice == unmarkChoiceFavorite || choice == unmarkChoiceBoth) && md.Favorite {
			md.Favorite = false
			clearedFavorite++
			changed = true
		}
		if (choice == unmarkChoiceToRead || choice == unmarkChoiceBoth) && md.ToRead {
			md.ToRead = false
			clearedToRead++
			changed = true
		}
		if !changed {
			continue
		}
		if err := m.meta.Upsert(ctx, &md); err != nil {
			m.setStatus("Failed to save metadata: " + err.Error())
			return
		}
		m.refreshMetadataCache(path, md)
		if path == m.currentEntryPath() {
			refreshPreview = true
		}
	}
	if refreshPreview {
		m.updateTextPreview()
	}
	switch choice {
	case unmarkChoiceFavorite:
		m.setStatus(fmt.Sprintf("Favorite cleared on %d file(s)", clearedFavorite))
	case unmarkChoiceToRead:
		m.setStatus(fmt.Sprintf("To-read cleared on %d file(s)", clearedToRead))
	case unmarkChoiceBoth:
		m.setStatus(fmt.Sprintf("Favorite cleared on %d file(s), To-read on %d", clearedFavorite, clearedToRead))
	}
}

func (m *Model) showQuickFilter(mode quickFilterMode) tea.Cmd {
	if m.meta == nil {
		m.setStatus("Metadata store not available")
		return nil
	}
	ctx := context.Background()
	var (
		list []meta.Metadata
		err  error
	)
	switch mode {
	case quickFilterFavorites:
		list, err = m.meta.ListFavorites(ctx)
	case quickFilterToRead:
		list, err = m.meta.ListToRead(ctx)
	case quickFilterUnread:
		list, err = m.meta.ListByReadingState(ctx, readingStateUnread)
	case quickFilterReading:
		list, err = m.meta.ListByReadingState(ctx, readingStateReading)
	case quickFilterRead:
		list, err = m.meta.ListByReadingState(ctx, readingStateRead)
	default:
		return nil
	}
	if err != nil {
		m.setStatus("Failed to load metadata: " + err.Error())
		return nil
	}
	matches := make([]searchMatch, 0, len(list))
	for _, md := range list {
		match := searchMatch{
			Path:       md.Path,
			Mode:       searchModeTitle,
			MatchCount: 1,
			Snippets:   metadataSnippets(md),
		}
		matches = append(matches, match)
	}
	summary := quickFilterSummary(mode, len(matches))
	msg := searchResultMsg{
		req:      searchRequest{},
		matches:  matches,
		summary:  summary,
		warnings: nil,
	}
	m.enterSearchResults(msg)
	m.quickFilter = mode
	if len(matches) == 0 {
		m.setStatus(summary)
	} else {
		m.setPersistentStatus(fmt.Sprintf("%s (Esc/q to exit)", summary))
	}
	return nil
}

func metadataSnippets(md meta.Metadata) []string {
	lines := make([]string, 0, 5)
	if strings.TrimSpace(md.Title) != "" {
		lines = append(lines, "Title: "+md.Title)
	}
	if strings.TrimSpace(md.Author) != "" {
		lines = append(lines, "Author: "+md.Author)
	}
	if strings.TrimSpace(md.Year) != "" {
		lines = append(lines, "Year: "+md.Year)
	}
	if strings.TrimSpace(md.Published) != "" {
		lines = append(lines, "Published: "+md.Published)
	}
	if strings.TrimSpace(md.Tag) != "" {
		lines = append(lines, "Tag: "+md.Tag)
	}
	status := []string{}
	if md.Favorite {
		status = append(status, "Favorite")
	}
	if md.ToRead {
		status = append(status, "To-read")
	}
	if len(status) > 0 {
		lines = append(lines, "Status: "+strings.Join(status, ", "))
	}
	lines = append(lines, "Reading: "+readingStateLabel(md.ReadingState))
	if len(lines) == 0 {
		lines = append(lines, "(no metadata)")
	}
	return lines
}

func quickFilterSummary(mode quickFilterMode, count int) string {
	label := mode.label()
	if label == "" {
		label = "Filter"
	}
	if count == 0 {
		return fmt.Sprintf("%s: no files", label)
	}
	return fmt.Sprintf("%s: %d file(s)", label, count)
}

const (
	readingStateUnread  = "unread"
	readingStateReading = "reading"
	readingStateRead    = "read"
)

func normalizeReadingStateValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case readingStateReading:
		return readingStateReading
	case readingStateRead:
		return readingStateRead
	default:
		return readingStateUnread
	}
}

func readingStateIcon(value string) string {
	switch normalizeReadingStateValue(value) {
	case readingStateReading:
		return "▶"
	case readingStateRead:
		return "✓"
	default:
		return "○"
	}
}

func readingStateLabel(value string) string {
	switch normalizeReadingStateValue(value) {
	case readingStateReading:
		return "Reading"
	case readingStateRead:
		return "Read"
	default:
		return "Unread"
	}
}

func nextReadingState(value string) string {
	switch normalizeReadingStateValue(value) {
	case readingStateUnread:
		return readingStateReading
	case readingStateReading:
		return readingStateRead
	default:
		return readingStateUnread
	}
}
