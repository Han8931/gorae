package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (m *Model) loadEntries() {
	ents, err := os.ReadDir(m.cwd)
	m.err = err
	if err != nil {
		m.entries = nil
		m.cursor = 0
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

	// sort dirs first then alpha
	sort.SliceStable(filtered, func(i, j int) bool {
		di, dj := filtered[i].IsDir(), filtered[j].IsDir()
		if di != dj {
			return di && !dj
		}
		return strings.ToLower(filtered[i].Name()) <
			strings.ToLower(filtered[j].Name())
	})

	m.entries = filtered

	if m.cursor >= len(m.entries) {
		m.cursor = 0
	}
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
