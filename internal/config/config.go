package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gorae/internal/theme"
)

const (
	colorReset = "\033[0m"
	colorCyan  = "\033[36m"
	colorGreen = "\033[32m"
	colorBold  = "\033[1m"
)

type Config struct {
	WatchDir            string `json:"watch_dir"`
	MetaDir             string `json:"meta_dir"`
	RecentlyAddedDir    string `json:"recent_dir,omitempty"` // keep legacy key for compatibility
	RecentlyAddedDays   int    `json:"recent_days,omitempty"`
	RecentlyOpenedDir   string `json:"recently_opened_dir,omitempty"`
	RecentlyOpenedLimit int    `json:"recently_opened_limit,omitempty"`
	Editor              string `json:"editor,omitempty"`
	PDFViewer           string `json:"pdf_viewer,omitempty"`
	NotesDir            string `json:"notes_dir,omitempty"`
	ThemePath           string `json:"theme_path,omitempty"`
	EnableMouse         bool   `json:"enable_mouse"`

	// Runtime-only fields (not persisted)
	ConfigPath    string `json:"-"`
	NeedsConfirm  bool   `json:"-"`
}

const (
	defaultRecentDays           = 30
	defaultRecentlyOpenedLimit  = 20
	legacyRecentDirName         = "_recent"
	legacyRecentlyAddedDirName  = "_recently_added"
	legacyRecentlyOpenedName    = "_recently_opened"
	defaultRecentlyAddedDirName = "Recently Added"
	defaultRecentlyOpenedName   = "Recently Read"
)

func defaultConfigPath() (string, error) {
	cfgHome := os.Getenv("XDG_CONFIG_HOME")
	if cfgHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cfgHome = filepath.Join(home, ".config")
	}
	return filepath.Join(cfgHome, "gorae", "config.json"), nil
}

// Path returns the full path to the config file, using the same rules as LoadOrInit.
func Path() (string, error) {
	return defaultConfigPath()
}

func defaultWatchDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("GORAE_WATCH_DIR")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv("GOPAPYRUS_WATCH_DIR")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv("PDF_TUI_WATCH_DIR")); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Documents", "Papers"), nil
}

func defaultMetaDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("GORAE_META_DIR")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv("GOPAPYRUS_META_DIR")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv("PDF_TUI_META_DIR")); v != "" {
		return v, nil
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "gorae"), nil
}

func defaultEditor() string {
	if v := strings.TrimSpace(os.Getenv("EDITOR")); v != "" {
		return v
	}
	return "vi"
}

func defaultPDFViewer() string {
	if v := strings.TrimSpace(os.Getenv("GORAE_PDF_VIEWER")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("PDF_VIEWER")); v != "" {
		return v
	}
	if v := detectSystemPDFViewer(); v != "" {
		return v
	}
	return "xdg-open"
}

// DefaultPDFViewer exposes the detected viewer so callers can use it as a fallback.
func DefaultPDFViewer() string {
	return defaultPDFViewer()
}

func defaultNotesDir(meta string) string {
	meta = strings.TrimSpace(meta)
	if meta == "" {
		return ""
	}
	return filepath.Join(meta, "notes")
}

func defaultThemePath() string {
	path, err := theme.Path()
	if err != nil {
		return ""
	}
	return path
}

func legacyThemePath() (string, error) {
	cfgHome := os.Getenv("XDG_CONFIG_HOME")
	if cfgHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cfgHome = filepath.Join(home, ".config")
	}
	return filepath.Join(cfgHome, "go-pdf", "theme.toml"), nil
}

func LoadOrInit() (*Config, error) {
	path, err := defaultConfigPath()
	if err != nil {
		return nil, err
	}

	// existing config
	if data, err := os.ReadFile(path); err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
		cfg.ConfigPath = path
		changed, err := cfg.ensureDefaults()
		if err != nil {
			return nil, err
		}
		if changed {
			if err := writeConfig(path, &cfg); err != nil {
				return nil, err
			}
		}
		if err := ensureNotesDirExists(&cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}

	// first run: create config from defaults so the app starts immediately.
	fmt.Printf("%s%sNo config found. Let's set it up.%s\n", colorCyan, colorBold, colorReset)
	watch, err := defaultWatchDir()
	if err != nil {
		return nil, err
	}
	meta, err := defaultMetaDir()
	if err != nil {
		return nil, err
	}
	recentAdded := filepath.Join(watch, defaultRecentlyAddedDirName)
	recentOpened := filepath.Join(watch, defaultRecentlyOpenedName)

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("  %swatch_dir%s (default: %s): ", colorGreen, colorReset, watch)
	if line, _ := reader.ReadString('\n'); strings.TrimSpace(line) != "" {
		watch = strings.TrimSpace(line)
	}
	fmt.Printf("  %smeta_dir %s (default: %s): ", colorGreen, colorReset, meta)
	if line, _ := reader.ReadString('\n'); strings.TrimSpace(line) != "" {
		meta = strings.TrimSpace(line)
	}
	if !filepath.IsAbs(watch) {
		if abs, err := filepath.Abs(watch); err == nil {
			watch = abs
		}
	}
	if !filepath.IsAbs(meta) {
		if abs, err := filepath.Abs(meta); err == nil {
			meta = abs
		}
	}
	recentAdded = filepath.Join(watch, defaultRecentlyAddedDirName)
	recentOpened = filepath.Join(watch, defaultRecentlyOpenedName)

	cfg := &Config{
		WatchDir:            watch,
		MetaDir:             meta,
		RecentlyAddedDir:    recentAdded,
		RecentlyAddedDays:   defaultRecentDays,
		RecentlyOpenedDir:   recentOpened,
		RecentlyOpenedLimit: defaultRecentlyOpenedLimit,
		Editor:              defaultEditor(),
		PDFViewer:           defaultPDFViewer(),
		NotesDir:            defaultNotesDir(meta),
		ThemePath:           defaultThemePath(),
		ConfigPath:          path,
		NeedsConfirm:        true,
		EnableMouse:         false,
	}
	fmt.Printf("  watch_dir: %s\n", cfg.WatchDir)
	fmt.Printf("  meta_dir : %s\n", cfg.MetaDir)
	fmt.Printf("  enable_mouse: %v (set to true to enable mouse input)\n", cfg.EnableMouse)
	fmt.Printf("Edit %s to change these paths.\n", path)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.MetaDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.RecentlyAddedDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.RecentlyOpenedDir, 0o755); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.NotesDir, 0o755); err != nil {
		return nil, err
	}

	if err := writeConfig(path, cfg); err != nil {
		return nil, err
	}

	fmt.Println("Config saved to", path)
	if _, err := cfg.ensureDefaults(); err != nil {
		return nil, err
	}
	if err := ensureNotesDirExists(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func writeConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Save persists the provided configuration to disk, ensuring directories exist first.
func Save(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	path, err := defaultConfigPath()
	if err != nil {
		return err
	}
	cfg.ConfigPath = path
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := cfg.ensureDefaults(); err != nil {
		return err
	}
	if err := ensureNotesDirExists(cfg); err != nil {
		return err
	}
	return writeConfig(path, cfg)
}

func (c *Config) ensureDefaults() (bool, error) {
	changed := false
	if c == nil {
		return changed, fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(c.WatchDir) == "" {
		w, err := defaultWatchDir()
		if err != nil {
			return changed, err
		}
		c.WatchDir = w
		changed = true
	}
	if strings.TrimSpace(c.MetaDir) == "" {
		m, err := defaultMetaDir()
		if err != nil {
			return changed, err
		}
		c.MetaDir = m
		changed = true
	}
	if strings.TrimSpace(c.RecentlyAddedDir) == "" {
		c.RecentlyAddedDir = filepath.Join(c.WatchDir, defaultRecentlyAddedDirName)
		changed = true
	} else if isLegacyRecentlyAddedPath(c.RecentlyAddedDir, c.WatchDir) {
		c.RecentlyAddedDir = upgradeLegacyRecentlyAddedPath(c.RecentlyAddedDir, c.WatchDir)
		changed = true
	}
	if c.RecentlyAddedDays <= 0 {
		c.RecentlyAddedDays = defaultRecentDays
		changed = true
	}
	if strings.TrimSpace(c.RecentlyOpenedDir) == "" {
		c.RecentlyOpenedDir = filepath.Join(c.WatchDir, defaultRecentlyOpenedName)
		changed = true
	} else if isLegacyRecentlyOpenedPath(c.RecentlyOpenedDir, c.WatchDir) {
		c.RecentlyOpenedDir = upgradeLegacyRecentlyOpenedPath(c.RecentlyOpenedDir, c.WatchDir)
		changed = true
	}
	if c.RecentlyOpenedLimit <= 0 {
		c.RecentlyOpenedLimit = defaultRecentlyOpenedLimit
		changed = true
	}
	if strings.TrimSpace(c.Editor) == "" {
		c.Editor = defaultEditor()
		changed = true
	}
	if strings.TrimSpace(c.PDFViewer) == "" {
		c.PDFViewer = defaultPDFViewer()
		changed = true
	}
	if strings.TrimSpace(c.NotesDir) == "" {
		c.NotesDir = defaultNotesDir(c.MetaDir)
		changed = true
	}
	if strings.TrimSpace(c.ThemePath) == "" {
		themePath := defaultThemePath()
		if themePath != "" {
			c.ThemePath = themePath
			changed = true
		}
	} else if newPath, upgraded := upgradeLegacyThemePath(c.ThemePath); upgraded {
		c.ThemePath = newPath
		changed = true
	}
	return changed, nil
}

func upgradeLegacyThemePath(current string) (string, bool) {
	clean := strings.TrimSpace(current)
	if clean == "" {
		return clean, false
	}
	target := defaultThemePath()
	if target == "" {
		return clean, false
	}
	legacy, err := legacyThemePath()
	if err != nil {
		return clean, false
	}
	cleanPath := filepath.Clean(clean)
	if cleanPath != filepath.Clean(legacy) {
		return clean, false
	}
	targetPath := filepath.Clean(target)
	if targetPath == cleanPath {
		return clean, false
	}
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err == nil {
			if data, readErr := os.ReadFile(cleanPath); readErr == nil {
				_ = os.WriteFile(targetPath, data, 0o644)
			}
		}
	}
	return targetPath, true
}

func isLegacyRecentlyAddedPath(path, watch string) bool {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return false
	}
	watchClean := filepath.Clean(strings.TrimSpace(watch))
	candidates := []string{
		filepath.Clean(filepath.Join(watchClean, legacyRecentDirName)),
		filepath.Clean(filepath.Join(watchClean, legacyRecentlyAddedDirName)),
	}
	for _, candidate := range candidates {
		if candidate != "" && clean == candidate {
			return true
		}
	}
	base := filepath.Base(clean)
	return base == legacyRecentDirName || base == legacyRecentlyAddedDirName
}

func upgradeLegacyRecentlyAddedPath(path, watch string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		if watch == "" {
			return filepath.Join(".", defaultRecentlyAddedDirName)
		}
		return filepath.Join(watch, defaultRecentlyAddedDirName)
	}
	dir := filepath.Dir(clean)
	if dir == "." || dir == string(filepath.Separator) {
		dir = strings.TrimSpace(watch)
		if dir == "" {
			dir = "."
		}
	}
	return filepath.Join(dir, defaultRecentlyAddedDirName)
}

func isLegacyRecentlyOpenedPath(path, watch string) bool {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return false
	}
	watchClean := filepath.Clean(strings.TrimSpace(watch))
	legacy := filepath.Clean(filepath.Join(watchClean, legacyRecentlyOpenedName))
	if legacy != "" && clean == legacy {
		return true
	}
	return filepath.Base(clean) == legacyRecentlyOpenedName
}

func upgradeLegacyRecentlyOpenedPath(path, watch string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		if watch == "" {
			return filepath.Join(".", defaultRecentlyOpenedName)
		}
		return filepath.Join(watch, defaultRecentlyOpenedName)
	}
	dir := filepath.Dir(clean)
	if dir == "." || dir == string(filepath.Separator) {
		dir = strings.TrimSpace(watch)
		if dir == "" {
			dir = "."
		}
	}
	return filepath.Join(dir, defaultRecentlyOpenedName)
}

func detectSystemPDFViewer() string {
	candidates := []string{
		"zathura",
		"sioyek",
		"mupdf",
		"okular",
		"evince",
		"atril",
		"xdg-open",
		"open",
	}
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			viewer := path
			if viewer == "" {
				viewer = name
			}
			if strings.IndexFunc(viewer, func(r rune) bool {
				return r == ' ' || r == '\t' || r == '"' || r == '\''
			}) >= 0 {
				viewer = strconv.Quote(viewer)
			}
			return viewer
		}
	}
	switch runtime.GOOS {
	case "darwin":
		return "open"
	case "windows":
		return "rundll32 url.dll,FileProtocolHandler"
	default:
		return "xdg-open"
	}
}

func ensureNotesDirExists(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	dir := strings.TrimSpace(cfg.NotesDir)
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
