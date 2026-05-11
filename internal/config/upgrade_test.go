package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// withTempUpgrade swaps configUpgrades for the duration of a test so the
// behaviour is independent of which upgrades the production binary currently
// ships. Restores on cleanup.
func withTempUpgrade(t *testing.T, upgrades []configUpgrade) {
	t.Helper()
	saved := configUpgrades
	configUpgrades = upgrades
	t.Cleanup(func() { configUpgrades = saved })
}

func TestUpgradeConfigBytesInjectsMissingField(t *testing.T) {
	withTempUpgrade(t, []configUpgrade{
		{key: "text_preview_only", block: `  // doc
  "text_preview_only": false`},
	})

	in := []byte(`{
  // existing
  "watch_dir": "/tmp"
}
`)
	out, injected, err := upgradeConfigBytes(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(injected) != 1 || injected[0] != "text_preview_only" {
		t.Fatalf("expected [text_preview_only], got %v", injected)
	}
	// The upgraded bytes must parse and contain both fields.
	var parsed map[string]any
	if err := json.Unmarshal(stripJSONComments(out), &parsed); err != nil {
		t.Fatalf("output does not parse: %v\n---\n%s", err, out)
	}
	if parsed["watch_dir"] != "/tmp" {
		t.Errorf("watch_dir lost: %v", parsed["watch_dir"])
	}
	if v, ok := parsed["text_preview_only"].(bool); !ok || v != false {
		t.Errorf("text_preview_only = %v, want false bool", parsed["text_preview_only"])
	}
}

func TestUpgradeConfigBytesNoopWhenPresent(t *testing.T) {
	withTempUpgrade(t, []configUpgrade{
		{key: "text_preview_only", block: `  "text_preview_only": false`},
	})
	in := []byte(`{"text_preview_only": true}`)
	out, injected, err := upgradeConfigBytes(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(injected) != 0 {
		t.Errorf("expected no injection, got %v", injected)
	}
	if string(out) != string(in) {
		t.Errorf("output changed unexpectedly:\n got: %s\nwant: %s", out, in)
	}
}

func TestUpgradeConfigBytesEmptyObject(t *testing.T) {
	withTempUpgrade(t, []configUpgrade{
		{key: "text_preview_only", block: `  "text_preview_only": false`},
	})
	in := []byte(`{}`)
	out, injected, err := upgradeConfigBytes(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(injected) != 1 {
		t.Fatalf("expected 1 injection, got %v", injected)
	}
	// Must remain valid JSON — no spurious leading comma.
	var parsed map[string]any
	if err := json.Unmarshal(stripJSONComments(out), &parsed); err != nil {
		t.Fatalf("output does not parse: %v\n---\n%s", err, out)
	}
	if strings.Contains(string(out), "{,") {
		t.Errorf("spurious leading comma:\n%s", out)
	}
}

func TestUpgradeConfigBytesMultipleMissing(t *testing.T) {
	withTempUpgrade(t, []configUpgrade{
		{key: "alpha", block: `  "alpha": 1`},
		{key: "beta", block: `  "beta": 2`},
	})
	in := []byte(`{"watch_dir": "/tmp"}`)
	out, injected, err := upgradeConfigBytes(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(injected) != 2 {
		t.Fatalf("expected 2 injections, got %v", injected)
	}
	var parsed map[string]any
	if err := json.Unmarshal(stripJSONComments(out), &parsed); err != nil {
		t.Fatalf("output does not parse: %v\n---\n%s", err, out)
	}
	if parsed["alpha"] == nil || parsed["beta"] == nil || parsed["watch_dir"] != "/tmp" {
		t.Errorf("missing field in upgraded config: %v", parsed)
	}
}

func TestUpgradeConfigBytesMalformedInputUnchanged(t *testing.T) {
	withTempUpgrade(t, []configUpgrade{
		{key: "alpha", block: `  "alpha": 1`},
	})
	in := []byte(`{not valid json`)
	out, injected, err := upgradeConfigBytes(in)
	if err == nil {
		t.Fatalf("expected error on malformed input")
	}
	if len(injected) != 0 {
		t.Errorf("no injection should happen on error, got %v", injected)
	}
	if string(out) != string(in) {
		t.Errorf("malformed input must round-trip unchanged")
	}
}
