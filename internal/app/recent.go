package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultRecentSyncInterval = time.Minute

func (m *Model) maybeSyncRecentDir(force bool) error {
	if m.recentDir == "" || m.recentMaxAge <= 0 {
		return nil
	}
	if !force && !m.lastRecentSync.IsZero() {
		if time.Since(m.lastRecentSync) < m.recentSyncInt {
			return nil
		}
	}
	if err := syncRecentDirectory(m.root, m.recentDir, m.recentMaxAge); err != nil {
		return err
	}
	m.lastRecentSync = time.Now()
	return nil
}

func syncRecentDirectory(root, recentDir string, maxAge time.Duration) error {
	if root == "" || recentDir == "" || maxAge <= 0 {
		return nil
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	recentAbs, err := filepath.Abs(recentDir)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	desired := make(map[string]string)

	err = filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}

		if path == recentAbs {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if path != rootAbs && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		if strings.HasPrefix(name, ".") {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(name), ".pdf") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			return nil
		}

		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			return nil
		}

		linkName := recentLinkName(rel)
		desired[linkName] = path
		return nil
	})
	if err != nil {
		return err
	}

	if err := os.MkdirAll(recentAbs, 0o755); err != nil {
		return err
	}

	existing := make(map[string]string)
	dirEntries, err := os.ReadDir(recentAbs)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, entry := range dirEntries {
		linkPath := filepath.Join(recentAbs, entry.Name())
		info, err := os.Lstat(linkPath)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(linkPath)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(recentAbs, target)
		}
		existing[entry.Name()] = filepath.Clean(target)
	}

	for name := range existing {
		target, ok := desired[name]
		if !ok || filepath.Clean(target) != existing[name] {
			_ = os.Remove(filepath.Join(recentAbs, name))
		}
	}

	for name, target := range desired {
		linkPath := filepath.Join(recentAbs, name)
		existingTarget, ok := existing[name]
		if ok && filepath.Clean(existingTarget) == filepath.Clean(target) {
			continue
		}
		_ = os.Remove(linkPath)
		relTarget, err := filepath.Rel(filepath.Dir(linkPath), target)
		if err != nil {
			relTarget = target
		}
		if err := os.Symlink(relTarget, linkPath); err != nil {
			return fmt.Errorf("creating symlink for %s: %w", target, err)
		}
	}

	return nil
}

func recentLinkName(rel string) string {
	trimmed := strings.TrimSpace(rel)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "\\", string(filepath.Separator))
	parts := strings.Split(trimmed, string(filepath.Separator))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.ReplaceAll(part, "\\", "_")
		part = strings.ReplaceAll(part, "/", "_")
		part = strings.ReplaceAll(part, " ", "_")
		if part == "" {
			part = "_"
		}
		parts[i] = part
	}
	return strings.Join(parts, "__")
}
