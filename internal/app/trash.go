package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Han8931/gorae/internal/meta"
)

// trashInfo is the sidecar record written for every item moved to the Trash. It
// records where the item came from and, for files, a snapshot of the document's
// metadata so a future restore can reinstate tags, notes, and reading state.
type trashInfo struct {
	Name         string          `json:"name"`                    // basename under trash/files
	OriginalPath string          `json:"original_path"`           // absolute path the item was deleted from
	IsDir        bool            `json:"is_dir"`                  //
	DeletedAt    time.Time       `json:"deleted_at"`              //
	Metadata     *meta.Metadata  `json:"metadata,omitempty"`      // file metadata snapshot, if any
	TreeMetadata []meta.Metadata `json:"tree_metadata,omitempty"` // for directories: snapshots of every document inside
}

// trashDir returns the base Trash directory (<meta_dir>/trash), or "" when no
// meta directory is configured. The Trash lives under the meta dir — outside the
// watched library — so trashed items are never re-indexed or shown in the tree.
func (m *Model) trashDir() string {
	if m == nil || m.cfg == nil {
		return ""
	}
	base := strings.TrimSpace(m.cfg.MetaDir)
	if base == "" {
		return ""
	}
	return filepath.Join(base, "trash")
}

func trashFilesDir(base string) string { return filepath.Join(base, "files") }
func trashInfoDir(base string) string  { return filepath.Join(base, "info") }

// moveToTrash relocates path into the Trash, snapshotting its metadata first so
// the deletion is recoverable. It returns an error (without touching path) when
// no Trash is available, so a failed move never degrades into a real delete.
func (m *Model) moveToTrash(path string) error {
	base := m.trashDir()
	if base == "" {
		return errors.New("trash unavailable (no meta dir configured)")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	filesDir := trashFilesDir(base)
	infoDir := trashInfoDir(base)
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return err
	}

	// Resolve the path while it still exists: once the item has moved, symlinks
	// can no longer be followed and canonicalPath would silently fall back to a
	// bare filepath.Abs. Both the metadata lookup and the restore record need
	// the resolved path, so take it once, here.
	canonical := canonicalPath(path)

	// Snapshot metadata before the move so tags/notes survive a later restore.
	// Files record their own metadata; directories record a snapshot for every
	// document inside the tree.
	var snapshot *meta.Metadata
	var treeSnapshot []meta.Metadata
	if m.meta != nil && canonical != "" {
		if info.IsDir() {
			if list, err := m.meta.ListTree(context.Background(), canonical); err == nil {
				treeSnapshot = list
			}
		} else if md, err := m.meta.Get(context.Background(), canonical); err == nil {
			snapshot = md
		}
	}

	dest := avoidNameClash(filepath.Join(filesDir, filepath.Base(path)))
	if err := moveOrCopy(path, dest); err != nil {
		return err
	}

	rec := trashInfo{
		Name:         filepath.Base(dest),
		OriginalPath: canonical,
		IsDir:        info.IsDir(),
		DeletedAt:    time.Now(),
		Metadata:     snapshot,
		TreeMetadata: treeSnapshot,
	}
	if rec.OriginalPath == "" {
		rec.OriginalPath = path
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		// The file is already safely in the Trash; a missing sidecar only costs
		// the (not-yet-built) restore path, so don't fail the deletion for it.
		return nil
	}
	_ = os.WriteFile(filepath.Join(infoDir, rec.Name+".json"), data, 0o644)
	return nil
}

// trashCount reports how many top-level items are currently in the Trash.
func (m *Model) trashCount() int {
	base := m.trashDir()
	if base == "" {
		return 0
	}
	entries, err := os.ReadDir(trashFilesDir(base))
	if err != nil {
		return 0
	}
	return len(entries)
}

// emptyTrash permanently deletes everything in the Trash and returns the number
// of top-level items removed.
func (m *Model) emptyTrash() (int, error) {
	base := m.trashDir()
	if base == "" {
		return 0, errors.New("trash unavailable (no meta dir configured)")
	}
	n := m.trashCount()
	filesDir := trashFilesDir(base)
	infoDir := trashInfoDir(base)
	if err := os.RemoveAll(filesDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	if err := os.RemoveAll(infoDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	return n, nil
}

// restoreFromTrash moves the named Trash item back to where it was deleted from
// and reinstates its metadata snapshot (tags, notes, reading state). It returns
// the path the item was restored to, which differs from the original when that
// path is occupied again.
func (m *Model) restoreFromTrash(name string) (string, error) {
	base := m.trashDir()
	if base == "" {
		return "", errors.New("trash unavailable (no meta dir configured)")
	}
	src := filepath.Join(trashFilesDir(base), name)
	if _, err := os.Lstat(src); err != nil {
		return "", err
	}

	infoPath := filepath.Join(trashInfoDir(base), name+".json")
	var rec trashInfo
	data, err := os.ReadFile(infoPath)
	if err != nil {
		return "", fmt.Errorf("no restore record for %s: %w", name, err)
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return "", fmt.Errorf("restore record for %s is unreadable: %w", name, err)
	}
	if strings.TrimSpace(rec.OriginalPath) == "" {
		return "", fmt.Errorf("restore record for %s has no original path", name)
	}

	dest := avoidNameClash(rec.OriginalPath)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := moveOrCopy(src, dest); err != nil {
		return "", err
	}
	m.reinstateTrashMetadata(rec, dest)
	_ = os.Remove(infoPath)
	return dest, nil
}

// reinstateTrashMetadata upserts the metadata snapshots recorded in rec,
// remapping paths when the item had to be restored somewhere other than its
// original location. Upsert re-syncs the tag table, so restored tags reappear
// in the tree's Tags section.
func (m *Model) reinstateTrashMetadata(rec trashInfo, dest string) {
	if m.meta == nil {
		return
	}
	ctx := context.Background()
	destCanonical := canonicalPath(dest)
	if destCanonical == "" {
		destCanonical = dest
	}
	if rec.Metadata != nil {
		md := *rec.Metadata
		md.Path = destCanonical
		_ = m.meta.Upsert(ctx, &md)
	}
	oldPrefix := filepath.Clean(rec.OriginalPath) + string(os.PathSeparator)
	for _, snap := range rec.TreeMetadata {
		rel, ok := strings.CutPrefix(snap.Path, oldPrefix)
		if !ok {
			continue
		}
		md := snap
		md.Path = filepath.Join(destCanonical, rel)
		_ = m.meta.Upsert(ctx, &md)
	}
}

// restoreAllFromTrash restores every item currently in the Trash, continuing
// past failures so one bad record doesn't strand the rest. It returns the
// number restored alongside the first error encountered.
func (m *Model) restoreAllFromTrash() (int, error) {
	base := m.trashDir()
	if base == "" {
		return 0, errors.New("trash unavailable (no meta dir configured)")
	}
	entries, err := os.ReadDir(trashFilesDir(base))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	restored := 0
	var firstErr error
	for _, e := range entries {
		if _, err := m.restoreFromTrash(e.Name()); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		restored++
	}
	return restored, firstErr
}

// moveOrCopy renames src to dst, falling back to a recursive copy-then-remove
// when the two live on different filesystems (os.Rename returns EXDEV) — as they
// may when the Trash and the library are on separate mounts.
func moveOrCopy(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return err
	}
	if err := copyTree(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// isCrossDevice reports whether err is an EXDEV ("invalid cross-device link")
// rename failure.
func isCrossDevice(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		err = linkErr.Err
	}
	return errors.Is(err, syscall.EXDEV)
}

// copyTree recursively copies src (a file or directory) to dst, preserving file
// modes.
func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFile(src, dst, info.Mode().Perm())
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// handleTrashCommand implements ":trash" and its subcommands. With no argument
// it reports how many items the Trash holds; "empty" permanently clears it;
// "restore [name]" puts one item (or, with no name, everything) back where it
// was deleted from, reinstating its metadata.
func (m *Model) handleTrashCommand(args []string) tea.Cmd {
	base := m.trashDir()
	if base == "" {
		m.setStatus("Trash unavailable (no meta dir configured)")
		return nil
	}
	if len(args) == 0 {
		n := m.trashCount()
		if n == 0 {
			m.setStatus("Trash is empty")
		} else {
			m.setStatus(fmt.Sprintf("Trash holds %d item(s) — ':trash restore' to put back, ':trash empty' to clear", n))
		}
		return nil
	}
	switch strings.ToLower(args[0]) {
	case "empty":
		n, err := m.emptyTrash()
		if err != nil {
			m.setStatus("Empty Trash failed: " + err.Error())
			return nil
		}
		if n == 0 {
			m.setStatus("Trash is already empty")
		} else {
			m.setStatus(fmt.Sprintf("Emptied Trash (%d item(s) permanently deleted)", n))
		}
	case "restore":
		restored := 0
		var err error
		if len(args) > 1 {
			// Trash names may contain spaces, so rejoin the remaining args.
			if _, err = m.restoreFromTrash(strings.Join(args[1:], " ")); err == nil {
				restored = 1
			}
		} else {
			restored, err = m.restoreAllFromTrash()
		}
		if restored > 0 {
			m.afterTrashRestore()
		}
		switch {
		case err != nil && restored == 0:
			m.setStatus("Restore failed: " + err.Error())
		case err != nil:
			m.setStatus(fmt.Sprintf("Restored %d item(s); some failed: %s", restored, err.Error()))
		case restored == 0:
			m.setStatus("Trash is empty — nothing to restore")
		default:
			m.setStatus(fmt.Sprintf("Restored %d item(s) from Trash", restored))
		}
	default:
		m.setStatus(fmt.Sprintf("Unknown trash command: %s (try ':trash restore' or ':trash empty')", args[0]))
	}
	return nil
}

// afterTrashRestore refreshes the Files pane and every derived view (favorites,
// recents, tag list) so restored documents reappear everywhere at once.
func (m *Model) afterTrashRestore() {
	m.loadEntries()
	m.updateTextPreview()
	if m.meta != nil {
		_ = m.syncCollectionDirectories()
		_ = m.syncRecentlyOpenedDirectory()
	}
	_ = m.maybeSyncRecentlyAddedDir(true)
	m.reloadTreeTags()
}
