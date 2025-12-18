package main

import (
	"flag"
	"log"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"gorae/internal/app"
	"gorae/internal/config"
	"gorae/internal/meta"
)

func main() {
	rootFlag := flag.String("root", "", "Root directory to start in (overrides config watch_dir)")
	flag.Parse()

	cfg, err := config.LoadOrInit()
	if err != nil {
		log.Fatal(err)
	}

	origWatch := cfg.WatchDir
	root := cfg.WatchDir
	if *rootFlag != "" {
		root = *rootFlag
	}
	cfg.WatchDir = root
	if *rootFlag != "" {
		defaultOldRecent := filepath.Join(origWatch, "_recent")
		legacyAdded := filepath.Join(origWatch, "_recently_added")
		defaultAdded := filepath.Join(origWatch, "Recently Added")
		trimmedRecent := strings.TrimSpace(cfg.RecentlyAddedDir)
		if trimmedRecent == "" || trimmedRecent == defaultOldRecent || trimmedRecent == legacyAdded || trimmedRecent == defaultAdded {
			cfg.RecentlyAddedDir = filepath.Join(root, "Recently Added")
		}

		legacyOpened := filepath.Join(origWatch, "_recently_opened")
		defaultOpened := filepath.Join(origWatch, "Recently Read")
		trimmedOpened := strings.TrimSpace(cfg.RecentlyOpenedDir)
		if trimmedOpened == "" || trimmedOpened == legacyOpened || trimmedOpened == defaultOpened {
			cfg.RecentlyOpenedDir = filepath.Join(root, "Recently Read")
		}
	}

	dbPath := filepath.Join(cfg.MetaDir, "metadata.db")
	store, err := meta.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	m := app.NewModel(cfg, store)

	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if cfg != nil && cfg.EnableMouse {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(m, opts...)
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
