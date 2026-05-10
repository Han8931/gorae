package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gorae/internal/meta"
)

func (m *Model) syncCollectionDirectories() error {
	if m.meta == nil {
		return nil
	}
	ctx := context.Background()
	if err := syncMetadataLinkDirectory(ctx, m.toReadDir, m.meta.ListToRead); err != nil {
		return err
	}
	return nil
}

func syncMetadataLinkDirectory(ctx context.Context, dir string, fetch func(context.Context) ([]meta.Metadata, error)) error {
	if dir == "" || fetch == nil {
		return nil
	}
	records, err := fetch(ctx)
	if err != nil {
		return err
	}
	desired := make(map[string]string, len(records))
	for _, md := range records {
		target := canonicalPath(strings.TrimSpace(md.Path))
		if target == "" {
			continue
		}
		info, err := os.Stat(target)
		if err != nil || info.IsDir() {
			continue
		}
		base := filepath.Base(target)
		title := strings.TrimSpace(md.Title)
		year := strings.TrimSpace(md.Year)
		linkName := mapBackedLinkName(base, title, year, desired)
		desired[linkName] = target
	}
	return reconcileLinkDirectory(dir, desired)
}

func reconcileLinkDirectory(dir string, desired map[string]string) error {
	if dir == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	existing := make(map[string]string)
	dirEntries, err := os.ReadDir(abs)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, entry := range dirEntries {
		linkPath := filepath.Join(abs, entry.Name())
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
			target = filepath.Join(abs, target)
		}
		existing[entry.Name()] = filepath.Clean(target)
	}

	for name := range existing {
		target := filepath.Clean(desired[name])
		if target == "" || target != existing[name] {
			_ = os.Remove(filepath.Join(abs, name))
		}
	}

	for name, target := range desired {
		clean := filepath.Clean(target)
		if clean == "" {
			continue
		}
		if existingTarget, ok := existing[name]; ok && existingTarget == clean {
			continue
		}
		linkPath := filepath.Join(abs, name)
		_ = os.Remove(linkPath)
		relTarget, err := filepath.Rel(filepath.Dir(linkPath), clean)
		if err != nil {
			relTarget = clean
		}
		if err := os.Symlink(relTarget, linkPath); err != nil {
			return fmt.Errorf("creating symlink for %s: %w", clean, err)
		}
	}
	return nil
}
