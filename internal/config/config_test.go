package config

import (
	"encoding/json"
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
	if len(injected) != 1 || injected[0] != "enable_mouse" {
		t.Fatalf("injected keys = %v, want [enable_mouse]", injected)
	}

	var cfg Config
	if err := json.Unmarshal(stripJSONComments(out), &cfg); err != nil {
		t.Fatalf("parse upgraded config: %v", err)
	}
	if !cfg.EnableMouse {
		t.Fatal("upgraded config should enable mouse input")
	}
}
