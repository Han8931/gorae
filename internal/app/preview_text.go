package app

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// extractFirstPageText runs `pdftotext` on the first page and returns lines.
// maxLines <= 0 means "no limit".
func extractFirstPageText(path string, maxLines int) ([]string, error) {
	// pdftotext path -f 1 -l 1 -layout -  (write to stdout)

	// Ensure pdftotext exists
	if _, err := exec.LookPath("pdftotext"); err != nil {
		if runtime.GOOS == "darwin" {
			return nil, fmt.Errorf("pdftotext not found (brew install poppler)")
		}
		return nil, fmt.Errorf("pdftotext not found (apt install poppler-utils / pacman -S poppler)")
	}

	cmd := exec.Command(
		"pdftotext",
		"-f", "1",
		"-l", "1",
		"-layout",
		path,
		"-", // stdout
	)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftotext: %w\n%s", err, errBuf.String())
	}

	text := out.String()
	rawLines := strings.Split(text, "\n")

	lines := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		// remove leading spaces and tabs to avoid big left margin
		trimmedLeft := strings.TrimLeft(l, " \t")

		// you can also collapse multiple spaces if you want, but this is enough
		lines = append(lines, trimmedLeft)
	}

	// trim trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	if len(lines) == 0 {
		lines = []string{"(no text extracted)"}
	}
	return lines, nil
}
