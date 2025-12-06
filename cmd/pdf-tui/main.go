package main

import (
	"flag"
	"log"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"pdf-tui/internal/app"
	"pdf-tui/internal/config"
	"pdf-tui/internal/meta"
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
		trimmedRecent := strings.TrimSpace(cfg.RecentDir)
		if trimmedRecent == "" || trimmedRecent == defaultOldRecent {
			cfg.RecentDir = filepath.Join(root, "_recent")
		}
	}

	dbPath := filepath.Join(cfg.MetaDir, "metadata.db")
	store, err := meta.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	m := app.NewModel(cfg, store)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
