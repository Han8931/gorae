package config

import (
	"bufio"
	"bytes"
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

// WebSearchConfig holds settings for the web search feature.
type WebSearchConfig struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`        // "brave" | "tavily"
	APIKey   string `json:"api_key"`
	Results  int    `json:"results,omitempty"` // default 5
}

// AIConfig holds settings for the AI chat feature (:gorae).
type AIConfig struct {
	Provider       string `json:"provider,omitempty"`
	Model          string `json:"model,omitempty"`
	APIKey         string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	TopK           int    `json:"top_k,omitempty"`
	SystemPrompt   string `json:"system_prompt"`
	VectorSearch   bool   `json:"vector_search"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
	// EnableTools turns on function/tool calling so the model can invoke
	// in-app actions (e.g. save the chat as markdown). Requires a provider/
	// model that supports tool calls. Disabled by default.
	EnableTools bool `json:"enable_tools"`
}

type Config struct {
	WatchDir            string    `json:"watch_dir"`
	MetaDir             string    `json:"meta_dir"`
	RecentlyAddedDir    string    `json:"recent_dir,omitempty"` // keep legacy key for compatibility
	RecentlyAddedDays   int       `json:"recent_days,omitempty"`
	RecentlyOpenedDir   string    `json:"recently_opened_dir,omitempty"`
	RecentlyOpenedLimit int       `json:"recently_opened_limit,omitempty"`
	Editor              string    `json:"editor,omitempty"`
	PDFViewer           string    `json:"pdf_viewer,omitempty"`
	NotesDir            string    `json:"notes_dir,omitempty"`
	ThemePath           string    `json:"theme_path,omitempty"`
	EnableMouse         bool      `json:"enable_mouse"`
	// TextPreviewOnly forces the right pane to use the text/ASCII fallback
	// even on terminals that support kitty/iTerm2/sixel image protocols.
	// Useful over slow ssh, in tmux without passthrough, or as a personal
	// preference. Defaults to false (image preview when supported).
	TextPreviewOnly bool      `json:"text_preview_only"`
	AI              *AIConfig `json:"ai,omitempty"`

	WebSearch *WebSearchConfig `json:"web_search,omitempty"`

	// Runtime-only fields (not persisted)
	ConfigPath   string `json:"-"`
	NeedsConfirm bool   `json:"-"`
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

// DefaultAIConfig returns a ready-to-edit AIConfig with placeholder values so
// every field appears in the saved JSON instead of being omitted.
func DefaultAIConfig() *AIConfig {
	return &AIConfig{
		Provider:       "ollama",
		Model:          "llama3.2",
		APIKey:         "",
		BaseURL:        "",
		TopK:           3,
		SystemPrompt:   "",
		VectorSearch:   false,
		EmbeddingModel: "nomic-embed-text",
		EnableTools:    false,
	}
}

// stripJSONComments removes // line comments and /* block comments */ so the
// config file can be written as JSONC and still parsed by encoding/json.
func stripJSONComments(src []byte) []byte {
	var out bytes.Buffer
	inStr := false
	i := 0
	for i < len(src) {
		c := src[i]
		if inStr {
			out.WriteByte(c)
			if c == '\\' && i+1 < len(src) {
				i++
				out.WriteByte(src[i])
			} else if c == '"' {
				inStr = false
			}
			i++
			continue
		}
		if c == '"' {
			inStr = true
			out.WriteByte(c)
			i++
			continue
		}
		if c == '/' && i+1 < len(src) {
			if src[i+1] == '/' {
				for i < len(src) && src[i] != '\n' {
					i++
				}
				continue
			}
			if src[i+1] == '*' {
				i += 2
				for i+1 < len(src) {
					if src[i] == '*' && src[i+1] == '/' {
						i += 2
						break
					}
					i++
				}
				continue
			}
		}
		out.WriteByte(c)
		i++
	}
	return out.Bytes()
}

// writeDefaultConfig writes the initial config.json with inline comments so
// new users see every option explained without having to read the docs.
func writeDefaultConfig(path string, cfg *Config) error {
	tmpl := `{
  // Directory Gorae watches for documents (PDFs, EPUBs, Markdown).
  "watch_dir": %q,

  // Directory where metadata, the FTS index, notes, and the SQLite DB are stored.
  "meta_dir": %q,

  // Recently-added virtual folder inside watch_dir.
  "recent_dir": %q,

  // How many days back "recently added" looks.
  "recent_days": %d,

  // Recently-read virtual folder inside watch_dir.
  "recently_opened_dir": %q,

  // How many files to keep in the recently-read list.
  "recently_opened_limit": %d,

  // Text editor used for :config and metadata editing.
  "editor": %q,

  // PDF viewer command. Gorae auto-detects zathura/sioyek/open/xdg-open.
  "pdf_viewer": %q,

  // Directory for plain-text / Markdown notes (linked to documents).
  "notes_dir": %q,

  // Path to a custom theme.toml. Leave empty to use the built-in theme.
  "theme_path": %q,

  // Set to true to enable mouse support.
  "enable_mouse": %v,

  // ── AI chat (:gorae) ────────────────────────────────────────────────────────
  "ai": {
    // Provider: "openai" | "ollama" | "custom"
    //   openai  → uses https://api.openai.com/v1
    //   ollama  → uses http://localhost:11434/v1  (no api_key needed)
    //   custom  → requires base_url
    "provider": "ollama",

    // Model name served by the provider.
    //   Ollama examples : "llama3.2", "mistral", "gemma3"
    //   OpenAI examples : "gpt-4o-mini", "gpt-4o"
    "model": "llama3.2",

    // API key — required for OpenAI / custom providers; leave empty for Ollama.
    "api_key": "",

    // Override the provider's default endpoint. Leave empty to use the default.
    "base_url": "",

    // Number of document chunks injected into every query as context (default 3).
    "top_k": 3,

    // Optional system prompt prepended before the RAG context block.
    "system_prompt": "",

    // Set to true to use semantic (vector) search instead of keyword FTS.
    // Requires running :index after enabling. Slower to index but finds
    // conceptually related content even when exact keywords don't match.
    "vector_search": false,

    // Embedding model used when vector_search is true.
    // Ollama: "nomic-embed-text", "mxbai-embed-large"
    // OpenAI: "text-embedding-3-small", "text-embedding-3-large"
    "embedding_model": "nomic-embed-text"
  },

  // ── Web search ──────────────────────────────────────────────────────────────
  // Augments AI answers with live web results. Disabled by default.
  // Gorae uses a routing node to decide when web search is needed,
  // so it is only called for queries that genuinely require current information.
  "web_search": {
    "enabled": false,

    // Provider: "brave" | "tavily"
    //   brave  → https://api.search.brave.com/ (2 000 free req/month)
    //   tavily → https://tavily.com/           (1 000 free req/month, AI-optimised)
    "provider": "tavily",

    // API key for the chosen provider.
    "api_key": "",

    // Number of web results injected per query (default 5).
    "results": 5
  }
}
`
	content := fmt.Sprintf(tmpl,
		cfg.WatchDir,
		cfg.MetaDir,
		cfg.RecentlyAddedDir,
		cfg.RecentlyAddedDays,
		cfg.RecentlyOpenedDir,
		cfg.RecentlyOpenedLimit,
		cfg.Editor,
		cfg.PDFViewer,
		cfg.NotesDir,
		cfg.ThemePath,
		cfg.EnableMouse,
	)
	// Run the upgrade pass so any fields added since the template was last
	// edited also land in freshly-created configs. Same path as existing
	// configs go through, keeping behaviour identical for both.
	upgraded, _, _ := upgradeConfigBytes([]byte(content))
	return os.WriteFile(path, upgraded, 0o644)
}

// configUpgrade describes one top-level config key that may be missing from
// older user configs. When you add a new top-level field to Config, add an
// entry here (and update the struct + comments). On next launch, existing
// users' configs will be auto-extended with the new key + default value +
// explanatory comment — no manual editing required.
//
// `block` is a complete JSONC fragment (leading comment lines + `"key": value`)
// with NO trailing comma; the upgrader handles comma stitching itself.
type configUpgrade struct {
	key   string
	block string
}

var configUpgrades = []configUpgrade{
	{
		key: "text_preview_only",
		block: `  // Force the text/ASCII PDF preview even on kitty/iTerm2/sixel terminals.
  // Useful over slow ssh, inside tmux without passthrough, or if you simply
  // prefer the text view. Leave false to keep image previews when supported.
  "text_preview_only": false`,
	},
}

// upgradeConfigBytes injects any registered top-level keys that are missing
// from raw and returns the updated JSONC plus the list of injected keys.
// Existing values, ordering, and user comments are preserved untouched. If
// raw doesn't parse as JSON (after comment stripping), it's returned unchanged.
func upgradeConfigBytes(raw []byte) ([]byte, []string, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(stripJSONComments(raw), &top); err != nil {
		return raw, nil, err
	}
	var missing []configUpgrade
	for _, u := range configUpgrades {
		if _, present := top[u.key]; !present {
			missing = append(missing, u)
		}
	}
	if len(missing) == 0 {
		return raw, nil, nil
	}

	// Locate the root object's closing brace (the last `}` in the file).
	closeIdx := bytes.LastIndexByte(raw, '}')
	if closeIdx < 0 {
		return raw, nil, fmt.Errorf("malformed config: no closing brace")
	}
	head := bytes.TrimRight(raw[:closeIdx], " \t\r\n")
	tail := raw[closeIdx:]
	needsComma := len(head) > 0 && head[len(head)-1] != ',' && head[len(head)-1] != '{'

	var b bytes.Buffer
	b.Write(head)
	if needsComma {
		b.WriteByte(',')
	}
	for i, u := range missing {
		b.WriteString("\n\n")
		b.WriteString(u.block)
		if i < len(missing)-1 {
			b.WriteByte(',')
		}
	}
	b.WriteByte('\n')
	b.Write(tail)

	keys := make([]string, len(missing))
	for i, u := range missing {
		keys[i] = u.key
	}
	return b.Bytes(), keys, nil
}

func LoadOrInit() (*Config, error) {
	path, err := defaultConfigPath()
	if err != nil {
		return nil, err
	}

	// existing config — strip JSONC comments before parsing
	if data, err := os.ReadFile(path); err == nil {
		// Auto-inject any top-level fields that have been added to Gorae
		// since this config was created. Idempotent: noop when nothing's
		// missing. We swallow write errors here because parsing still
		// succeeds against the in-memory upgraded bytes either way.
		if upgraded, injected, err := upgradeConfigBytes(data); err == nil && len(injected) > 0 {
			_ = os.WriteFile(path, upgraded, 0o644)
			data = upgraded
		}
		var cfg Config
		if err := json.Unmarshal(stripJSONComments(data), &cfg); err != nil {
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

	if err := writeDefaultConfig(path, cfg); err != nil {
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
