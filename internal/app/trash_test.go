package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Han8931/gorae/internal/config"
	"github.com/Han8931/gorae/internal/meta"
)

// newTrashTestModel builds a Model with a real meta store and a meta dir under a
// temp directory, so Trash operations have somewhere to write.
func newTrashTestModel(t *testing.T) (*Model, string) {
	t.Helper()
	base := t.TempDir()
	metaDir := filepath.Join(base, "meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("mkdir meta: %v", err)
	}
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

func TestMoveToTrashRelocatesFile(t *testing.T) {
	m, base := newTrashTestModel(t)
	src := filepath.Join(base, "paper.pdf")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := m.moveToTrash(src); err != nil {
		t.Fatalf("moveToTrash: %v", err)
	}
	// Original is gone; the file now lives in trash/files.
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be gone, stat err = %v", err)
	}
	trashed := filepath.Join(m.trashDir(), "files", "paper.pdf")
	if data, err := os.ReadFile(trashed); err != nil || string(data) != "hello" {
		t.Fatalf("trashed file = %q, err = %v", data, err)
	}
	if got := m.trashCount(); got != 1 {
		t.Fatalf("trashCount = %d, want 1", got)
	}
}

func TestMoveToTrashSnapshotsMetadata(t *testing.T) {
	m, base := newTrashTestModel(t)
	src := filepath.Join(base, "tagged.pdf")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	canonical := canonicalPath(src)
	if err := m.meta.Upsert(context.Background(), &meta.Metadata{
		Path: canonical,
		Tag:  "ml/nlp",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := m.moveToTrash(src); err != nil {
		t.Fatalf("moveToTrash: %v", err)
	}

	infoPath := filepath.Join(m.trashDir(), "info", "tagged.pdf.json")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		t.Fatalf("read info sidecar: %v", err)
	}
	var rec trashInfo
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("unmarshal info: %v", err)
	}
	if rec.OriginalPath != canonical {
		t.Fatalf("OriginalPath = %q, want %q", rec.OriginalPath, canonical)
	}
	if rec.Metadata == nil || rec.Metadata.Tag != "ml/nlp" {
		t.Fatalf("metadata snapshot not preserved: %+v", rec.Metadata)
	}
}

func TestMoveToTrashUniqueNames(t *testing.T) {
	m, base := newTrashTestModel(t)
	for i := 0; i < 2; i++ {
		src := filepath.Join(base, "dup.pdf")
		if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
			t.Fatalf("write src: %v", err)
		}
		if err := m.moveToTrash(src); err != nil {
			t.Fatalf("moveToTrash %d: %v", i, err)
		}
	}
	if got := m.trashCount(); got != 2 {
		t.Fatalf("trashCount = %d, want 2 (collisions should be renamed)", got)
	}
}

func TestEmptyTrash(t *testing.T) {
	m, base := newTrashTestModel(t)
	src := filepath.Join(base, "gone.pdf")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := m.moveToTrash(src); err != nil {
		t.Fatalf("moveToTrash: %v", err)
	}

	n, err := m.emptyTrash()
	if err != nil {
		t.Fatalf("emptyTrash: %v", err)
	}
	if n != 1 {
		t.Fatalf("emptied count = %d, want 1", n)
	}
	if got := m.trashCount(); got != 0 {
		t.Fatalf("trashCount after empty = %d, want 0", got)
	}
}

func TestMoveToTrashDirectoryWithContents(t *testing.T) {
	m, base := newTrashTestModel(t)
	dir := filepath.Join(base, "folder")
	nested := filepath.Join(dir, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.pdf"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "b.pdf"), []byte("b"), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	if err := m.moveToTrash(dir); err != nil {
		t.Fatalf("moveToTrash dir: %v", err)
	}
	// Original directory is gone entirely.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("source dir should be gone, stat err = %v", err)
	}
	// The whole subtree lands in the Trash, contents intact.
	trashRoot := filepath.Join(m.trashDir(), "files", "folder")
	if data, err := os.ReadFile(filepath.Join(trashRoot, "a.pdf")); err != nil || string(data) != "a" {
		t.Fatalf("trashed a.pdf = %q, err = %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(trashRoot, "sub", "b.pdf")); err != nil || string(data) != "b" {
		t.Fatalf("trashed sub/b.pdf = %q, err = %v", data, err)
	}
}

func TestRestoreFromTrashReinstatesFileAndTags(t *testing.T) {
	m, base := newTrashTestModel(t)
	src := filepath.Join(base, "paper.pdf")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	canonical := canonicalPath(src)
	ctx := context.Background()
	if err := m.meta.Upsert(ctx, &meta.Metadata{Path: canonical, Title: "Paper", Tag: "ml/nlp"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := m.moveToTrash(src); err != nil {
		t.Fatalf("moveToTrash: %v", err)
	}
	if err := m.meta.DeletePath(ctx, canonical); err != nil {
		t.Fatalf("delete path: %v", err)
	}
	if tags, _ := m.meta.AllTags(ctx); len(tags) != 0 {
		t.Fatalf("tags should be gone after delete, got %v", tags)
	}

	dest, err := m.restoreFromTrash("paper.pdf")
	if err != nil {
		t.Fatalf("restoreFromTrash: %v", err)
	}
	if dest != canonical {
		t.Fatalf("restored to %q, want %q", dest, canonical)
	}
	if data, err := os.ReadFile(src); err != nil || string(data) != "hello" {
		t.Fatalf("restored file = %q, err = %v", data, err)
	}
	md, err := m.meta.Get(ctx, canonical)
	if err != nil || md == nil || md.Tag != "ml/nlp" || md.Title != "Paper" {
		t.Fatalf("metadata not reinstated: %+v, err = %v", md, err)
	}
	if tags, _ := m.meta.AllTags(ctx); len(tags) != 1 || tags[0] != "ml/nlp" {
		t.Fatalf("AllTags = %v, want [ml/nlp]", tags)
	}
	if got := m.trashCount(); got != 0 {
		t.Fatalf("trashCount after restore = %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(m.trashDir(), "info", "paper.pdf.json")); !os.IsNotExist(err) {
		t.Fatalf("info sidecar should be removed, stat err = %v", err)
	}
}

func TestRestoreFromTrashDirectoryReinstatesTreeMetadata(t *testing.T) {
	m, base := newTrashTestModel(t)
	dir := filepath.Join(base, "folder")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	inner := filepath.Join(dir, "a.pdf")
	if err := os.WriteFile(inner, []byte("a"), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	ctx := context.Background()
	innerCanonical := canonicalPath(inner)
	if err := m.meta.Upsert(ctx, &meta.Metadata{Path: innerCanonical, Tag: "physics"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := m.moveToTrash(dir); err != nil {
		t.Fatalf("moveToTrash dir: %v", err)
	}
	if err := m.meta.DeleteTree(ctx, canonicalPath(dir)); err != nil {
		t.Fatalf("delete tree: %v", err)
	}

	if _, err := m.restoreFromTrash("folder"); err != nil {
		t.Fatalf("restoreFromTrash: %v", err)
	}
	if data, err := os.ReadFile(inner); err != nil || string(data) != "a" {
		t.Fatalf("restored a.pdf = %q, err = %v", data, err)
	}
	md, err := m.meta.Get(ctx, innerCanonical)
	if err != nil || md == nil || md.Tag != "physics" {
		t.Fatalf("tree metadata not reinstated: %+v, err = %v", md, err)
	}
}

func TestRestoreFromTrashAvoidsClobbering(t *testing.T) {
	m, base := newTrashTestModel(t)
	src := filepath.Join(base, "paper.pdf")
	if err := os.WriteFile(src, []byte("old"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := m.moveToTrash(src); err != nil {
		t.Fatalf("moveToTrash: %v", err)
	}
	// A new file now occupies the original path.
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatalf("write replacement: %v", err)
	}

	dest, err := m.restoreFromTrash("paper.pdf")
	if err != nil {
		t.Fatalf("restoreFromTrash: %v", err)
	}
	if dest == canonicalPath(src) {
		t.Fatalf("restore should not clobber the occupied path %q", dest)
	}
	if data, err := os.ReadFile(src); err != nil || string(data) != "new" {
		t.Fatalf("occupant = %q, err = %v (should be untouched)", data, err)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "old" {
		t.Fatalf("restored copy = %q, err = %v", data, err)
	}
}

func TestRestoreAllFromTrash(t *testing.T) {
	m, base := newTrashTestModel(t)
	for _, name := range []string{"one.pdf", "two.pdf"} {
		src := filepath.Join(base, name)
		if err := os.WriteFile(src, []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if err := m.moveToTrash(src); err != nil {
			t.Fatalf("moveToTrash %s: %v", name, err)
		}
	}

	n, err := m.restoreAllFromTrash()
	if err != nil {
		t.Fatalf("restoreAllFromTrash: %v", err)
	}
	if n != 2 {
		t.Fatalf("restored = %d, want 2", n)
	}
	for _, name := range []string{"one.pdf", "two.pdf"} {
		if data, err := os.ReadFile(filepath.Join(base, name)); err != nil || string(data) != name {
			t.Fatalf("restored %s = %q, err = %v", name, data, err)
		}
	}
	if got := m.trashCount(); got != 0 {
		t.Fatalf("trashCount after restore all = %d, want 0", got)
	}
}

func TestMoveToTrashNoMetaDir(t *testing.T) {
	m := &Model{cfg: &config.Config{}} // no MetaDir
	if err := m.moveToTrash("/tmp/whatever"); err == nil {
		t.Fatal("expected error when no meta dir is configured")
	}
}

// newTagViewLinkModel builds a model sitting in a tag view whose single row is
// a symlink to a real document, the arrangement that made `D` operate on the
// link instead of the paper.
func newTagViewLinkModel(t *testing.T) (m *Model, realPath, linkPath string) {
	t.Helper()
	m, base := newTrashTestModel(t)

	library := filepath.Join(base, "library")
	tagView := filepath.Join(base, "meta", "tagview")
	for _, dir := range []string{library, tagView} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	realPath = filepath.Join(library, "paper.pdf")
	if err := os.WriteFile(realPath, []byte("paper"), 0o644); err != nil {
		t.Fatalf("write real doc: %v", err)
	}
	linkPath = filepath.Join(tagView, "paper.pdf")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	m.root = base
	m.cwd = tagView
	m.tagViewDir = tagView
	m.cwdIsTagView = true
	m.activeTagFilter = "ml"
	return m, realPath, linkPath
}

func TestResolveLinkTargetsFollowsLinksInLinkViews(t *testing.T) {
	m, realPath, linkPath := newTagViewLinkModel(t)

	got := m.resolveLinkTargets([]string{linkPath})
	if len(got) != 1 || got[0] != canonicalPath(realPath) {
		t.Fatalf("in a tag view: resolved = %v, want [%s]", got, canonicalPath(realPath))
	}

	// Outside a link view the same symlink is a real entry in its own right and
	// must be left alone, so deleting it removes the link and not the target.
	m.cwdIsTagView = false
	got = m.resolveLinkTargets([]string{linkPath})
	if len(got) != 1 || got[0] != linkPath {
		t.Fatalf("outside a link view: resolved = %v, want [%s]", got, linkPath)
	}
}

func TestResolveLinkTargetsDeduplicates(t *testing.T) {
	m, realPath, linkPath := newTagViewLinkModel(t)

	// A second row pointing at the same document must not yield two deletions.
	otherLink := filepath.Join(m.cwd, "paper (dup).pdf")
	if err := os.Symlink(realPath, otherLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if got := m.resolveLinkTargets([]string{linkPath, otherLink}); len(got) != 1 {
		t.Fatalf("resolved = %v, want a single deduplicated path", got)
	}
}

// Deleting a row in a tag view must trash the paper itself: the file leaves the
// library, and what lands in the Trash is the document, not a dangling link.
func TestMoveToTrashFromTagViewTrashesRealDocument(t *testing.T) {
	m, realPath, linkPath := newTagViewLinkModel(t)

	md := &meta.Metadata{Path: canonicalPath(realPath), Title: "A Paper", Tag: "ml"}
	if err := m.meta.Upsert(context.Background(), md); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	targets := m.resolveLinkTargets([]string{linkPath})
	if err := m.moveToTrash(targets[0]); err != nil {
		t.Fatalf("moveToTrash: %v", err)
	}

	if _, err := os.Lstat(realPath); !os.IsNotExist(err) {
		t.Fatalf("real document still in the library (err = %v)", err)
	}

	trashed := filepath.Join(trashFilesDir(m.trashDir()), "paper.pdf")
	info, err := os.Lstat(trashed)
	if err != nil {
		t.Fatalf("lstat trashed item: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("trashed item is a symlink; the link was trashed instead of the document")
	}
	if data, err := os.ReadFile(trashed); err != nil || string(data) != "paper" {
		t.Fatalf("trashed contents = %q, err = %v", data, err)
	}

	// The restore record must point back into the library, not at the tag view.
	var rec trashInfo
	data, err := os.ReadFile(filepath.Join(trashInfoDir(m.trashDir()), "paper.pdf.json"))
	if err != nil {
		t.Fatalf("read trash info: %v", err)
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("unmarshal trash info: %v", err)
	}
	if rec.OriginalPath != canonicalPath(realPath) {
		t.Fatalf("original_path = %q, want %q", rec.OriginalPath, canonicalPath(realPath))
	}
	if rec.Metadata == nil || rec.Metadata.Tag != "ml" {
		t.Fatalf("metadata snapshot = %+v, want the tag preserved", rec.Metadata)
	}
}

// End-to-end: pressing D then y on a row in a tag view must take the paper out
// of the library, drop the now-stale row from the view, and retire the tag once
// its last document is gone.
func TestDeleteKeyInTagViewRemovesDocumentAndTag(t *testing.T) {
	m, realPath, _ := newTagViewLinkModel(t)
	ctx := context.Background()
	if err := m.meta.Upsert(ctx, &meta.Metadata{Path: canonicalPath(realPath), Title: "A Paper", Tag: "ml"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	m.reloadTreeTags()
	if len(m.treeTags) != 1 {
		t.Fatalf("treeTags = %v, want [ml] before the delete", m.treeTags)
	}
	m.loadEntries()
	if len(m.entries) != 1 {
		t.Fatalf("entries = %d, want the single tag-view row", len(m.entries))
	}

	// D arms the confirmation against the real document, not the link.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")})
	nm := updated.(Model)
	if nm.state != stateConfirmDelete {
		t.Fatalf("state = %v after D, want stateConfirmDelete", nm.state)
	}
	if len(nm.confirmItems) != 1 || nm.confirmItems[0] != canonicalPath(realPath) {
		t.Fatalf("confirmItems = %v, want [%s]", nm.confirmItems, canonicalPath(realPath))
	}

	updated, _ = nm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm = updated.(Model)

	if _, err := os.Lstat(realPath); !os.IsNotExist(err) {
		t.Fatalf("paper still in the library (err = %v)", err)
	}
	if nm.trashCount() != 1 {
		t.Fatalf("trashCount = %d, want 1", nm.trashCount())
	}
	if len(nm.entries) != 0 {
		t.Fatalf("entries = %d, want the stale tag-view row dropped", len(nm.entries))
	}
	if len(nm.treeTags) != 0 {
		t.Fatalf("treeTags = %v, want the tag retired with its last document", nm.treeTags)
	}
}

// A library reached through a symlinked parent must still record its resolved
// path: capturing it after the move would leave the link unresolved, because
// the moved file can no longer be followed.
func TestOriginalPathResolvedThroughSymlinkedParent(t *testing.T) {
	m, base := newTrashTestModel(t)

	realLib := filepath.Join(base, "real_lib")
	if err := os.MkdirAll(realLib, 0o755); err != nil {
		t.Fatalf("mkdir real lib: %v", err)
	}
	linkedLib := filepath.Join(base, "lib")
	if err := os.Symlink(realLib, linkedLib); err != nil {
		t.Fatalf("symlink lib: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realLib, "paper.pdf"), []byte("paper"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	if err := m.moveToTrash(filepath.Join(linkedLib, "paper.pdf")); err != nil {
		t.Fatalf("moveToTrash: %v", err)
	}

	var rec trashInfo
	data, err := os.ReadFile(filepath.Join(trashInfoDir(m.trashDir()), "paper.pdf.json"))
	if err != nil {
		t.Fatalf("read trash info: %v", err)
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("unmarshal trash info: %v", err)
	}
	want := filepath.Join(canonicalPath(realLib), "paper.pdf")
	if rec.OriginalPath != want {
		t.Fatalf("original_path = %q, want %q", rec.OriginalPath, want)
	}
}
