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

// extractFirstPageImagePreview converts the first page of a PDF to terminal
// block-character art using pdftoppm and chafa. imgWidth and imgHeight are the
// desired visual dimensions in terminal columns/rows. Returns one string per
// row; each string may contain ANSI escape codes.
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

	// Derive a deterministic temp-file base from the PDF path so that
	// successive calls for the same file reuse the already-converted image.
	hash := sha1.Sum([]byte(path))
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
		// Not cached yet — convert first page.
		cmd := exec.Command(
			"pdftoppm",
			"-f", "1",
			"-l", "1",
			"-r", "72",
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

	// Render the PPM as terminal art sized to the panel.
	size := fmt.Sprintf("%dx%d", imgWidth, imgHeight)
	cmd := exec.Command(
		"chafa",
		"--size="+size,
		"--format=symbols",
		"--colors=256",
		tmpPPM,
	)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("chafa: %w — %s", err, errBuf.String())
	}

	// Strip cursor-visibility control sequences that chafa emits but that
	// would conflict with bubbletea's own cursor management.
	output := strings.ReplaceAll(out.String(), "\x1b[?25l", "")
	output = strings.ReplaceAll(output, "\x1b[?25h", "")

	raw := strings.TrimRight(output, "\n")
	lines := strings.Split(raw, "\n")
	return lines, nil
}
