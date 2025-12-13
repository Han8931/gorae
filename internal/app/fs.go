package app

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func (m *Model) loadEntries() {
	if err := m.maybeSyncRecentlyAddedDir(false); err != nil {
		m.setStatus("Recently added sync failed: " + err.Error())
	}
	ents, err := os.ReadDir(m.cwd)
	m.err = err
	if err != nil {
		m.entries = nil
		m.cursor = 0
		m.resortEntries()
		return
	}

	// hide dotfiles and non-PDF files (but keep directories)
	filtered := make([]fs.DirEntry, 0, len(ents))
	notesDir := strings.TrimSpace(m.notesDir)
	noteAbs := ""
	if notesDir != "" {
		noteAbs = canonicalPath(notesDir)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if noteAbs != "" && e.IsDir() {
			full := filepath.Join(m.cwd, e.Name())
			if canonicalPath(full) == noteAbs {
				continue
			}
		}

		if !e.IsDir() {
			name := strings.ToLower(e.Name())
			if !strings.HasSuffix(name, ".pdf") {
				continue
			}
		}
		filtered = append(filtered, e)
	}

	m.entries = filtered
	if m.cursor >= len(m.entries) {
		m.cursor = 0
	}
	m.resortEntries()
	m.ensureCursorVisible()
}

func (m *Model) removeFromCut(path string) {
	out := m.cut[:0]
	for _, c := range m.cut {
		if c != path {
			out = append(out, c)
		}
	}
	m.cut = out
}

func (m Model) selectionOrCurrent() []string {
	if len(m.selected) > 0 {
		out := make([]string, 0, len(m.selected))
		for p := range m.selected {
			out = append(out, p)
		}
		return out
	}
	if len(m.entries) == 0 {
		return nil
	}
	full := filepath.Join(m.cwd, m.entries[m.cursor].Name())
	return []string{full}
}

func avoidNameClash(dst string) string {
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return dst
	}
	ext := filepath.Ext(dst)
	base := strings.TrimSuffix(filepath.Base(dst), ext)
	dir := filepath.Dir(dst)

	for i := 1; ; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
}

func (m *Model) refreshEntryTitles() {
	info := m.buildEntrySortInfo(m.entries)
	m.refreshEntryTitlesWithInfo(info)
}

func (m *Model) refreshEntryTitlesWithInfo(entryInfo map[string]entrySortInfo) {
	if m.entryTitles == nil {
		m.entryTitles = make(map[string]string)
	}
	for k := range m.entryTitles {
		delete(m.entryTitles, k)
	}

	var ctx context.Context
	useMeta := entryInfo == nil && m.meta != nil
	if useMeta {
		ctx = context.Background()
	}

	for _, e := range m.entries {
		full := filepath.Join(m.cwd, e.Name())
		fileInfo, err := e.Info()
		isDir := e.IsDir()
		if err == nil {
			isDir = fileInfo.IsDir()
		}
		if isDir {
			name := e.Name()
			if err == nil {
				name = fileInfo.Name()
			}
			m.entryTitles[full] = name + "/"
			continue
		}
		if entryInfo != nil {
			if data, ok := entryInfo[full]; ok {
				m.entryTitles[full] = data.display()
				continue
			}
		}
		if useMeta {
			m.entryTitles[full] = m.resolveEntryTitle(ctx, full, e)
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		m.entryTitles[full] = fmt.Sprintf("[%s][-][%s]", readingStateIcon(""), name)
	}
}

func (m *Model) resolveEntryTitle(ctx context.Context, fullPath string, entry fs.DirEntry) string {
	info, err := entry.Info()
	if err == nil && info.IsDir() {
		return info.Name() + "/"
	}
	if entry.IsDir() {
		return entry.Name() + "/"
	}

	baseName := entry.Name()
	if err == nil {
		baseName = info.Name()
	}
	name := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	if m.meta == nil {
		return fmt.Sprintf("[%s][-][%s]", readingStateIcon(""), name)
	}

	path := canonicalPath(fullPath)
	md, err := m.meta.Get(ctx, path)
	if err != nil || md == nil {
		return fmt.Sprintf("[%s][-][%s]", readingStateIcon(""), name)
	}
	stateIcon := readingStateIcon(md.ReadingState)
	title := strings.TrimSpace(md.Title)
	if title == "" {
		title = name
	}
	year := strings.TrimSpace(md.Year)
	if year == "" {
		year = "-"
	}
	return fmt.Sprintf("[%s][%s][%s]", stateIcon, year, title)
}

func (m *Model) resortEntries() {
	if len(m.entries) == 0 {
		if m.entryTitles != nil {
			for k := range m.entryTitles {
				delete(m.entryTitles, k)
			}
		}
		return
	}
	info := m.buildEntrySortInfo(m.entries)
	m.sortEntries(m.entries, info)
	m.refreshEntryTitlesWithInfo(info)
}

func (m *Model) buildEntrySortInfo(entries []fs.DirEntry) map[string]entrySortInfo {
	if len(entries) == 0 {
		return nil
	}
	sortInfo := make(map[string]entrySortInfo, len(entries))
	var ctx context.Context
	useMeta := m.meta != nil
	if useMeta {
		ctx = context.Background()
	}
	for _, e := range entries {
		info, err := e.Info()
		isDir := e.IsDir()
		if err == nil {
			isDir = info.IsDir()
		}
		if isDir {
			continue
		}
		full := filepath.Join(m.cwd, e.Name())
		baseName := e.Name()
		if err == nil {
			baseName = info.Name()
		}
		base := strings.TrimSuffix(baseName, filepath.Ext(baseName))
		data := entrySortInfo{title: base}
		if useMeta {
			path := canonicalPath(full)
			md, err := m.meta.Get(ctx, path)
			if err == nil && md != nil {
				if t := strings.TrimSpace(md.Title); t != "" {
					data.title = t
				}
				data.year = strings.TrimSpace(md.Year)
				data.state = normalizeReadingStateValue(md.ReadingState)
			}
		}
		sortInfo[full] = data
	}
	if len(sortInfo) == 0 {
		return nil
	}
	return sortInfo
}

func (m *Model) sortEntries(entries []fs.DirEntry, info map[string]entrySortInfo) {
	if len(entries) == 0 {
		return
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		di, dj := a.IsDir(), b.IsDir()
		if di != dj {
			return di && !dj
		}
		switch m.sortMode {
		case sortByTitle:
			return m.compareByTitle(a, b, info)
		case sortByYear:
			return m.compareByYear(a, b, info)
		default:
			return strings.ToLower(a.Name()) < strings.ToLower(b.Name())
		}
	})
}

func (m *Model) compareByTitle(a, b fs.DirEntry, info map[string]entrySortInfo) bool {
	ia := m.lookupSortInfo(a, info)
	ib := m.lookupSortInfo(b, info)
	ta := strings.ToLower(ia.title)
	tb := strings.ToLower(ib.title)
	if ta != tb {
		return ta < tb
	}
	return strings.ToLower(a.Name()) < strings.ToLower(b.Name())
}

func (m *Model) compareByYear(a, b fs.DirEntry, info map[string]entrySortInfo) bool {
	ia := m.lookupSortInfo(a, info)
	ib := m.lookupSortInfo(b, info)
	ya := parseYearValue(ia.year)
	yb := parseYearValue(ib.year)
	if ya != yb {
		return ya > yb
	}
	ta := strings.ToLower(ia.title)
	tb := strings.ToLower(ib.title)
	if ta != tb {
		return ta < tb
	}
	return strings.ToLower(a.Name()) < strings.ToLower(b.Name())
}

func (m *Model) lookupSortInfo(entry fs.DirEntry, info map[string]entrySortInfo) entrySortInfo {
	if info != nil {
		full := filepath.Join(m.cwd, entry.Name())
		if data, ok := info[full]; ok {
			return data
		}
	}
	base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
	return entrySortInfo{title: base}
}

func parseYearValue(year string) int {
	year = strings.TrimSpace(year)
	if year == "" {
		return -1
	}
	if y, err := strconv.Atoi(year); err == nil {
		return y
	}
	return -1
}

type entrySortInfo struct {
	title string
	year  string
	state string
}

func (e entrySortInfo) display() string {
	title := strings.TrimSpace(e.title)
	if title == "" {
		title = "-"
	}
	status := readingStateIcon(e.state)
	year := strings.TrimSpace(e.year)
	if year == "" {
		year = "-"
	}
	return fmt.Sprintf("[%s][%s][%s]", status, year, title)
}
