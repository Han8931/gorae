package meta_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Han8931/gorae/internal/meta"
)

func TestRecordOpenedAndList(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "meta.db")

	store, err := meta.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()

	first := "/tmp/a.pdf"
	second := "/tmp/b.pdf"

	if err := store.RecordOpened(ctx, first, time.Unix(1000, 0)); err != nil {
		t.Fatalf("record first: %v", err)
	}
	if err := store.RecordOpened(ctx, second, time.Unix(2000, 0)); err != nil {
		t.Fatalf("record second: %v", err)
	}

	results, err := store.ListRecentlyOpened(ctx, 10)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Path != second {
		t.Fatalf("expected most recent to be %s, got %s", second, results[0].Path)
	}

	// Update first to be most recent.
	if err := store.RecordOpened(ctx, first, time.Unix(3000, 0)); err != nil {
		t.Fatalf("record update: %v", err)
	}
	results, err = store.ListRecentlyOpened(ctx, 10)
	if err != nil {
		t.Fatalf("list recent 2: %v", err)
	}
	if results[0].Path != first {
		t.Fatalf("expected updated path first, got %s", results[0].Path)
	}
	if results[0].LastOpenedAt.IsZero() {
		t.Fatalf("expected last opened timestamp to be set")
	}
}

func TestDeletePathAndTree(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "meta.db")

	store, err := meta.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()

	root := filepath.Join(dir, "papers")
	fileA := filepath.Join(root, "a.pdf")
	fileB := filepath.Join(root, "sub", "b.pdf")

	for _, path := range []string{fileA, fileB} {
		md := meta.Metadata{Path: path, Title: filepath.Base(path)}
		if err := store.Upsert(ctx, &md); err != nil {
			t.Fatalf("upsert %s: %v", path, err)
		}
	}

	if err := store.DeletePath(ctx, fileA); err != nil {
		t.Fatalf("delete path: %v", err)
	}
	if md, err := store.Get(ctx, fileA); err != nil || md != nil {
		t.Fatalf("expected a.pdf removed, got %v err=%v", md, err)
	}

	if err := store.DeleteTree(ctx, filepath.Join(root, "sub")); err != nil {
		t.Fatalf("delete tree: %v", err)
	}
	if md, err := store.Get(ctx, fileB); err != nil || md != nil {
		t.Fatalf("expected subtree removed, got %v err=%v", md, err)
	}
}
