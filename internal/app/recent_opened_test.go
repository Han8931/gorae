package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorae/internal/meta"
)

func TestRebuildRecentlyOpenedDirectory(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "recent")
	db := filepath.Join(dir, "meta.db")

	store, err := meta.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()

	fileA := filepath.Join(dir, "a.pdf")
	writeDummyPDF(t, fileA)
	fileB := filepath.Join(dir, "b.pdf")
	writeDummyPDF(t, fileB)

	if err := store.RecordOpened(ctx, canonicalPath(fileA), time.Unix(1000, 0)); err != nil {
		t.Fatalf("record a: %v", err)
	}

	if err := rebuildRecentlyOpenedDirectory(dest, 5, store); err != nil {
		t.Fatalf("rebuild initial: %v", err)
	}
	names := listSymlinkNames(t, dest)
	if len(names) != 1 {
		t.Fatalf("expected 1 link, got %v", names)
	}
	firstLink := filepath.Join(dest, names[0])
	firstInfo := lstat(t, firstLink)

	// Rebuild without changes should keep existing link untouched.
	if err := rebuildRecentlyOpenedDirectory(dest, 5, store); err != nil {
		t.Fatalf("rebuild no change: %v", err)
	}
	if newInfo := lstat(t, firstLink); !newInfo.ModTime().Equal(firstInfo.ModTime()) {
		t.Fatalf("expected symlink to remain untouched")
	}

	// Add another recent file.
	if err := store.RecordOpened(ctx, canonicalPath(fileB), time.Unix(2000, 0)); err != nil {
		t.Fatalf("record b: %v", err)
	}
	if err := rebuildRecentlyOpenedDirectory(dest, 5, store); err != nil {
		t.Fatalf("rebuild with two files: %v", err)
	}
	if names := listSymlinkNames(t, dest); len(names) != 2 {
		t.Fatalf("expected 2 links after adding b, got %v", names)
	}

	// Shrink limit to 1 and ensure oldest entry removed.
	if err := rebuildRecentlyOpenedDirectory(dest, 1, store); err != nil {
		t.Fatalf("rebuild limit 1: %v", err)
	}
	names = listSymlinkNames(t, dest)
	if len(names) != 1 {
		t.Fatalf("expected 1 link after limit shrink, got %v", names)
	}

	// Record a missing file; rebuild should ignore it and keep existing links.
	missing := filepath.Join(dir, "missing.pdf")
	if err := store.RecordOpened(ctx, canonicalPath(missing), time.Unix(3000, 0)); err != nil {
		t.Fatalf("record missing: %v", err)
	}
	if err := rebuildRecentlyOpenedDirectory(dest, 2, store); err != nil {
		t.Fatalf("rebuild with missing file: %v", err)
	}
	if len(listSymlinkNames(t, dest)) != 1 {
		t.Fatalf("expected missing file to be ignored")
	}
}

func TestRebuildRecentlyOpenedDirectoryUpdatesMovedPaths(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "recent")
	db := filepath.Join(dir, "meta.db")

	store, err := meta.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()

	oldDir := filepath.Join(dir, "old")
	newDir := filepath.Join(dir, "new")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("mkdir old dir: %v", err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("mkdir new dir: %v", err)
	}

	oldPath := filepath.Join(oldDir, "paper.pdf")
	writeDummyPDF(t, oldPath)

	if err := store.RecordOpened(ctx, canonicalPath(oldPath), time.Unix(1000, 0)); err != nil {
		t.Fatalf("record initial open: %v", err)
	}
	if err := rebuildRecentlyOpenedDirectory(dest, 5, store); err != nil {
		t.Fatalf("initial rebuild: %v", err)
	}
	if targets := listSymlinkTargets(t, dest); len(targets) != 1 || targets[0] != canonicalPath(oldPath) {
		t.Fatalf("unexpected initial target list: %v", targets)
	}

	newPath := filepath.Join(newDir, "paper.pdf")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("rename file: %v", err)
	}
	if err := store.MovePath(ctx, canonicalPath(oldPath), canonicalPath(newPath)); err != nil {
		t.Fatalf("update metadata path: %v", err)
	}

	if err := rebuildRecentlyOpenedDirectory(dest, 5, store); err != nil {
		t.Fatalf("rebuild after move: %v", err)
	}

	targets := listSymlinkTargets(t, dest)
	if len(targets) != 1 {
		t.Fatalf("expected 1 target after move, got %v", targets)
	}
	if targets[0] != canonicalPath(newPath) {
		t.Fatalf("expected target %s after move, got %s", canonicalPath(newPath), targets[0])
	}
}

func listSymlinkNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		names = append(names, entry.Name())
	}
	return names
}

func listSymlinkTargets(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		linkPath := filepath.Join(dir, entry.Name())
		target, err := os.Readlink(linkPath)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(dir, target)
		}
		out = append(out, filepath.Clean(target))
	}
	return out
}

func lstat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return info
}
