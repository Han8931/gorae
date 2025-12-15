package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorae/internal/meta"
)

func (m *Model) recordRecentlyOpened(path string) {
	if path == "" {
		return
	}
	canonical := canonicalPath(path)
	if canonical == "" {
		return
	}
	now := time.Now()
	if m.meta != nil {
		ctx := context.Background()
		if err := m.meta.RecordOpened(ctx, canonical, now); err != nil {
			m.setStatus("Recently read update failed: " + err.Error())
		}
	}
	if m.meta == nil || m.recentlyOpenedDir == "" || m.recentlyOpenedLimit <= 0 {
		return
	}
	if err := rebuildRecentlyOpenedDirectory(m.recentlyOpenedDir, m.recentlyOpenedLimit, m.meta); err != nil {
		m.setStatus("Recently read directory sync failed: " + err.Error())
	}
}

func rebuildRecentlyOpenedDirectory(dest string, limit int, store *meta.Store) error {
	if dest == "" || limit <= 0 || store == nil {
		return nil
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		return err
	}

	ctx := context.Background()
	list, err := store.ListRecentlyOpened(ctx, limit)
	if err != nil {
		return err
	}

	existing := make(map[string]string)
	dirEntries, err := os.ReadDir(destAbs)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, entry := range dirEntries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		linkPath := filepath.Join(destAbs, entry.Name())
		target, err := os.Readlink(linkPath)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(destAbs, target)
		}
		existing[entry.Name()] = filepath.Clean(target)
	}

	desired := make(map[string]string)
	for _, md := range list {
		target := strings.TrimSpace(md.Path)
		if target == "" {
			continue
		}
		target = canonicalPath(target)
		if target == "" {
			continue
		}
		if _, err := os.Stat(target); err != nil {
			continue
		}
		openedAt := md.LastOpenedAt
		if openedAt.IsZero() {
			openedAt = time.Now()
		}
		linkName := recentLinkName(filepath.Base(target), md.Title, md.Year, openedAt)
		desired[linkName] = target
	}

	for name, target := range desired {
		if existingTarget, ok := existing[name]; ok && existingTarget == target {
			delete(existing, name)
			continue
		}
		linkPath := filepath.Join(destAbs, name)
		relTarget, err := filepath.Rel(filepath.Dir(linkPath), target)
		if err != nil {
			relTarget = target
		}
		_ = os.Remove(linkPath)
		if err := os.Symlink(relTarget, linkPath); err != nil {
			return fmt.Errorf("creating recently opened link for %s: %w", target, err)
		}
	}

	for name := range existing {
		if _, keep := desired[name]; keep {
			continue
		}
		_ = os.Remove(filepath.Join(destAbs, name))
	}

	return nil
}

const recentLinkTimestampLayout = "20060102T150405.000000000Z"

func recentLinkName(baseName, title, year string, openedAt time.Time) string {
	base := buildLinkBase(baseName, title, year)
	ts := openedAt.UTC().Format(recentLinkTimestampLayout)
	return fmt.Sprintf("%s-%s", ts, base)
}

func parseRecentLinkTimestamp(name string) (time.Time, bool) {
	idx := strings.IndexByte(name, '-')
	if idx <= 0 {
		return time.Time{}, false
	}
	tsStr := name[:idx]
	ts, err := time.Parse(recentLinkTimestampLayout, tsStr)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func stripRecentLinkPrefix(name string) string {
	idx := strings.IndexByte(name, '-')
	if idx <= 0 {
		return name
	}
	if _, ok := parseRecentLinkTimestamp(name); !ok {
		return name
	}
	trimmed := name[idx+1:]
	if trimmed == "" {
		return name
	}
	return trimmed
}
