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
	if err := m.maybeSyncRecentDir(false); err != nil {
		m.setStatus("Recent sync failed: " + err.Error())
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
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
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

func (m *Model) refreshEntryTitlesWithInfo(info map[string]entrySortInfo) {
	if m.entryTitles == nil {
		m.entryTitles = make(map[string]string)
	}
	for k := range m.entryTitles {
		delete(m.entryTitles, k)
	}

	var ctx context.Context
	useMeta := info == nil && m.meta != nil
	if useMeta {
		ctx = context.Background()
	}

	for _, e := range m.entries {
		full := filepath.Join(m.cwd, e.Name())
		if e.IsDir() {
			m.entryTitles[full] = e.Name() + "/"
			continue
		}
		if info != nil {
			if data, ok := info[full]; ok {
				m.entryTitles[full] = data.display()
				continue
			}
		}
		if useMeta {
			m.entryTitles[full] = m.resolveEntryTitle(ctx, full, e)
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		m.entryTitles[full] = fmt.Sprintf("[-][%s]", name)
	}
}

func (m *Model) resolveEntryTitle(ctx context.Context, fullPath string, entry fs.DirEntry) string {
	if entry.IsDir() {
		return entry.Name() + "/"
	}

	name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
	if m.meta == nil {
		return fmt.Sprintf("[-][%s]", name)
	}

	md, err := m.meta.Get(ctx, fullPath)
	if err != nil || md == nil {
		return fmt.Sprintf("[-][%s]", name)
	}
	title := strings.TrimSpace(md.Title)
	if title == "" {
		title = name
	}
	year := strings.TrimSpace(md.Year)
	if year == "" {
		year = "-"
	}
	return fmt.Sprintf("[%s][%s]", year, title)
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
	info := make(map[string]entrySortInfo, len(entries))
	var ctx context.Context
	useMeta := m.meta != nil
	if useMeta {
		ctx = context.Background()
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(m.cwd, e.Name())
		base := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		data := entrySortInfo{title: base}
		if useMeta {
			md, err := m.meta.Get(ctx, full)
			if err == nil && md != nil {
				if t := strings.TrimSpace(md.Title); t != "" {
					data.title = t
				}
				data.year = strings.TrimSpace(md.Year)
			}
		}
		info[full] = data
	}
	if len(info) == 0 {
		return nil
	}
	return info
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
}

func (e entrySortInfo) display() string {
	title := strings.TrimSpace(e.title)
	if title == "" {
		title = "-"
	}
	year := strings.TrimSpace(e.year)
	if year == "" {
		year = "-"
	}
	return fmt.Sprintf("[%s][%s]", year, title)
}
