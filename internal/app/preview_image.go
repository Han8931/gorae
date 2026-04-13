package app

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// chafaFormat is detected once at process start by inspecting terminal
// environment variables. It selects the best image rendering path:
//
//   - "kitty"   — kitty graphics protocol (kitty, ghostty, wezterm)
//   - "iterm"   — iTerm2 inline image protocol (macOS iTerm2)
//   - "sixel"   — DEC sixel (foot, mlterm, …)
//   - "symbols" — Unicode half-block characters (universal fallback)
var chafaFormat = detectChafaFormat()

// detectChafaFormat inspects well-known environment variables to choose the
// sharpest image output format that the running terminal supports.
func detectChafaFormat() string {
	// kitty and kitty-protocol-compatible terminals
	if os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("TERM") == "xterm-kitty" {
		return "kitty"
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "WezTerm", "ghostty":
		return "kitty"
	case "iTerm.app":
		return "iterm"
	}
	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" || os.Getenv("WEZTERM_EXECUTABLE") != "" {
		return "kitty"
	}
	// Terminals with reliable sixel support
	switch os.Getenv("TERM") {
	case "foot", "foot-extra", "mlterm":
		return "sixel"
	}
	// Unicode half-block fallback (works everywhere)
	return "symbols"
}

// pdfDPI returns the pdftoppm resolution appropriate for the detected chafa
// format. Pixel-based protocols (kitty, iterm, sixel) benefit from a higher
// source resolution; block-character art does not.
func pdfDPI() int {
	if chafaFormat == "symbols" {
		return 72
	}
	return 144
}

// extractFirstPageImagePreview converts the first page of a PDF to terminal
// image output using pdftoppm and chafa. The output format is chosen
// automatically for the running terminal (kitty, iTerm2, sixel, or Unicode
// half-block symbols). imgWidth and imgHeight are the desired visual
// dimensions in terminal columns/rows. Returns one string per row; each
// string may contain ANSI/protocol escape codes.
//
// The caller is responsible for ensuring that imgWidth and imgHeight are
// positive and appropriate for the panel that will render the output.
func extractFirstPageImagePreview(path string, imgWidth, imgHeight int) ([]string, error) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		if runtime.GOOS == "darwin" {
			return nil, fmt.Errorf("pdftoppm not found (brew install poppler)")
		}
		return nil, fmt.Errorf("pdftoppm not found (apt install poppler-utils / pacman -S poppler)")
	}
	if _, err := exec.LookPath("chafa"); err != nil {
		if runtime.GOOS == "darwin" {
			return nil, fmt.Errorf("chafa not found (brew install chafa)")
		}
		return nil, fmt.Errorf("chafa not found (apt install chafa / pacman -S chafa)")
	}
	if imgWidth < 4 {
		imgWidth = 4
	}
	if imgHeight < 2 {
		imgHeight = 2
	}

	dpi := pdfDPI()

	// Derive a deterministic temp-file base from the PDF path and DPI so that
	// successive calls for the same file reuse the already-converted image.
	// DPI is part of the key so that changing DPI never serves stale files.
	hash := sha1.Sum([]byte(fmt.Sprintf("%s@%d", path, dpi)))
	hashStr := hex.EncodeToString(hash[:])
	tmpDir := os.TempDir()
	tmpBase := filepath.Join(tmpDir, "gorae_prev_"+hashStr)

	// pdftoppm zero-pads page numbers according to total page count, so the
	// actual output file might be "base-1.ppm", "base-01.ppm", "base-001.ppm",
	// etc. Locate whichever variant was produced.
	findPPM := func() string {
		matches, _ := filepath.Glob(tmpBase + "-*.ppm")
		if len(matches) == 0 {
			return ""
		}
		sort.Strings(matches)
		return matches[0]
	}

	tmpPPM := findPPM()
	if tmpPPM == "" {
		// Not cached yet — rasterise the first page.
		cmd := exec.Command(
			"pdftoppm",
			"-f", "1",
			"-l", "1",
			"-r", fmt.Sprintf("%d", dpi),
			path,
			tmpBase,
		)
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("pdftoppm: %w — %s", err, errBuf.String())
		}
		tmpPPM = findPPM()
		if tmpPPM == "" {
			return nil, fmt.Errorf("pdftoppm produced no output for %s", filepath.Base(path))
		}
	}

	// Build chafa arguments for the detected terminal format.
	size := fmt.Sprintf("%dx%d", imgWidth, imgHeight)
	args := []string{"--size=" + size, "--format=" + chafaFormat}
	switch chafaFormat {
	case "symbols":
		// Half-block characters (▀▄) give 2× effective vertical resolution
		// compared to full-block art. Use the 256-colour palette which is safe
		// on virtually every terminal.
		args = append(args, "--symbols=half", "--colors=256")
	default:
		// Pixel-based protocols support full 24-bit colour; let chafa
		// auto-negotiate with the terminal rather than forcing a palette.
	}
	args = append(args, tmpPPM)

	cmd := exec.Command("chafa", args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("chafa: %w — %s", err, errBuf.String())
	}

	// Strip cursor-visibility sequences that chafa emits but that would
	// conflict with bubbletea's own cursor management.
	output := strings.ReplaceAll(out.String(), "\x1b[?25l", "")
	output = strings.ReplaceAll(output, "\x1b[?25h", "")

	raw := strings.TrimRight(output, "\n")
	lines := strings.Split(raw, "\n")
	return lines, nil
}
