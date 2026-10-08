package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Han8931/gorae/internal/config"
	"github.com/Han8931/gorae/internal/meta"
)

// newMetaEditTestModel builds a Model with a real meta store so the editing
// view's save paths can be exercised end to end.
func newMetaEditTestModel(t *testing.T) (*Model, string) {
	t.Helper()
	base := t.TempDir()
	metaDir := filepath.Join(base, "meta")
	store, err := meta.Open(filepath.Join(metaDir, "meta.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	m := &Model{
		cfg:  &config.Config{MetaDir: metaDir},
		meta: store,
	}
	return m, base
}

// The editor file has no favorite/to-read fields, but Upsert writes every
// column — so the save path must carry the stored flags and timestamps over
// instead of resetting them.
func TestApplyMetadataEditPreservesFlagsAndTimestamps(t *testing.T) {
	m, base := newMetaEditTestModel(t)
	target := filepath.Join(base, "paper.pdf")
	ctx := context.Background()

	added := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	before := meta.Metadata{
		Path:         target,
		Title:        "Old title",
		Author:       "Vaswani",
		Favorite:     true,
		ToRead:       true,
		ReadingState: readingStateReading,
		AddedAt:      added,
	}
	if err := m.meta.Upsert(ctx, &before); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}

	tmp := filepath.Join(base, "edit.json")
	edited := `{"title":"New title","author":"Vaswani","year":"2017","reading_state":"read"}`
	if err := os.WriteFile(tmp, []byte(edited), 0o644); err != nil {
		t.Fatalf("write edit file: %v", err)
	}

	if status := m.applyMetadataEdit(metadataEditFinishedMsg{tmpPath: tmp, targetPath: target}); status != "Metadata saved" {
		t.Fatalf("applyMetadataEdit status = %q, want %q", status, "Metadata saved")
	}

	after, err := m.meta.Get(ctx, target)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after == nil {
		t.Fatal("metadata row missing after edit")
	}
	if after.Title != "New title" || after.Year != "2017" {
		t.Fatalf("edited fields not saved: title = %q, year = %q", after.Title, after.Year)
	}
	if !after.Favorite {
		t.Error("Favorite was cleared by the metadata edit")
	}
	if !after.ToRead {
		t.Error("To-read was cleared by the metadata edit")
	}
	if got := normalizeReadingStateValue(after.ReadingState); got != readingStateRead {
		t.Errorf("ReadingState = %q, want %q", got, readingStateRead)
	}
	if !after.AddedAt.Equal(added) {
		t.Errorf("AddedAt = %v, want %v", after.AddedAt, added)
	}
}

// A failed or empty editor run must not wipe the stored row.
func TestApplyMetadataEditRejectsBadInput(t *testing.T) {
	m, base := newMetaEditTestModel(t)
	target := filepath.Join(base, "paper.pdf")
	ctx := context.Background()

	before := meta.Metadata{Path: target, Title: "Keep me", Favorite: true}
	if err := m.meta.Upsert(ctx, &before); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}

	bad := filepath.Join(base, "bad.json")
	if err := os.WriteFile(bad, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}

	cases := []struct {
		name string
		msg  metadataEditFinishedMsg
	}{
		{"no data returned", metadataEditFinishedMsg{targetPath: target}},
		{"unknown target", metadataEditFinishedMsg{tmpPath: bad}},
		{"unparseable json", metadataEditFinishedMsg{tmpPath: bad, targetPath: target}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if status := m.applyMetadataEdit(tc.msg); status == "Metadata saved" {
				t.Fatal("applyMetadataEdit reported success on invalid input")
			}
			after, err := m.meta.Get(ctx, target)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if after == nil || after.Title != "Keep me" || !after.Favorite {
				t.Fatalf("stored row changed: %+v", after)
			}
		})
	}
}
