package app

import (
	"os/exec"
	"strings"
	"testing"
)

func TestEnsureGraphicPreviewToolsKittyDoesNotRequireChafaOrPdftoppm(t *testing.T) {
	originalLookPath := execLookPath
	t.Cleanup(func() {
		execLookPath = originalLookPath
	})

	execLookPath = func(file string) (string, error) {
		if file == "pdftocairo" {
			return "/usr/bin/pdftocairo", nil
		}
		return "", exec.ErrNotFound
	}

	if err := ensureGraphicPreviewTools("kitty"); err != nil {
		t.Fatalf("expected kitty requirements to pass with pdftocairo only, got %v", err)
	}
}

func TestEnsureGraphicPreviewToolsITermStillRequiresChafa(t *testing.T) {
	originalLookPath := execLookPath
	t.Cleanup(func() {
		execLookPath = originalLookPath
	})

	execLookPath = func(file string) (string, error) {
		if file == "pdftoppm" {
			return "/usr/bin/pdftoppm", nil
		}
		return "", exec.ErrNotFound
	}

	err := ensureGraphicPreviewTools("iterm")
	if err == nil {
		t.Fatal("expected missing chafa error")
	}
	if !strings.Contains(err.Error(), "chafa") {
		t.Fatalf("expected chafa-related error, got %v", err)
	}
}
