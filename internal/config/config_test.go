package config

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestMouseInputEnabledByDefault(t *testing.T) {
	if !defaultEnableMouse {
		t.Fatal("mouse input should be enabled by default so :gorae output can be scrolled with the mouse wheel")
	}
}

func TestMissingMouseSettingUpgradesToEnabled(t *testing.T) {
	out, injected, err := upgradeConfigBytes([]byte(`{"watch_dir":"/tmp","text_preview_only":false}`))
	if err != nil {
		t.Fatalf("upgrade config: %v", err)
	}
	if !slices.Contains(injected, "enable_mouse") {
		t.Fatalf("injected keys = %v, want enable_mouse", injected)
	}

	var cfg Config
	if err := json.Unmarshal(stripJSONComments(out), &cfg); err != nil {
		t.Fatalf("parse upgraded config: %v", err)
	}
	if !cfg.EnableMouse {
		t.Fatal("upgraded config should enable mouse input")
	}
}

func TestMissingTreeSettingUpgradesToFolded(t *testing.T) {
	out, injected, err := upgradeConfigBytes([]byte(`{"watch_dir":"/tmp","enable_mouse":true,"text_preview_only":false}`))
	if err != nil {
		t.Fatalf("upgrade config: %v", err)
	}
	if !slices.Contains(injected, "show_tree") {
		t.Fatalf("injected keys = %v, want show_tree", injected)
	}
	var cfg Config
	if err := json.Unmarshal(stripJSONComments(out), &cfg); err != nil {
		t.Fatalf("parse upgraded config: %v", err)
	}
	if cfg.ShowTree {
		t.Fatal("upgraded config should start with the tree pane folded")
	}
}

// Indexing records this name alongside every stored vector and querying
// embeds against it, so the two must resolve a blank config identically — a
// mismatch would make freshly-built vectors look stale forever.
func TestResolvedEmbeddingModel(t *testing.T) {
	tests := []struct {
		name string
		cfg  *AIConfig
		want string
	}{
		{"explicit model", &AIConfig{EmbeddingModel: "mxbai-embed-large"}, "mxbai-embed-large"},
		{"blank falls back to default", &AIConfig{EmbeddingModel: ""}, DefaultEmbeddingModel},
		{"whitespace is not a model", &AIConfig{EmbeddingModel: "  "}, DefaultEmbeddingModel},
		{"surrounding whitespace trimmed", &AIConfig{EmbeddingModel: " nomic-embed-text\n"}, "nomic-embed-text"},
		{"nil config", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.ResolvedEmbeddingModel(); got != tt.want {
				t.Errorf("ResolvedEmbeddingModel() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The default config must be self-consistent: its embedding_model is the same
// string a blank config resolves to, so writing out defaults and relying on
// the fallback cannot disagree.
func TestDefaultAIConfigUsesDefaultEmbeddingModel(t *testing.T) {
	if got := DefaultAIConfig().EmbeddingModel; got != DefaultEmbeddingModel {
		t.Errorf("default embedding_model = %q, want %q", got, DefaultEmbeddingModel)
	}
}
