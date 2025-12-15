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

func lstat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return info
}
