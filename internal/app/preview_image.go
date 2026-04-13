package app

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

var errTerminalGraphicsUnsupported = errors.New("terminal image preview unsupported")
var execLookPath = exec.LookPath

const kittyPreviewImageID = 1
const kittyChunkRawSize = 3072

func terminalGraphicFormat() string {
	if override := normalizeGraphicFormat(os.Getenv("GORAE_PDF_PREVIEW_FORMAT")); override != "" {
		return override
	}

	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	termProgram := strings.ToLower(strings.TrimSpace(os.Getenv("TERM_PROGRAM")))

	switch {
	case os.Getenv("KITTY_WINDOW_ID") != "" || strings.Contains(term, "kitty"):
		return "kitty"
	case termProgram == "wezterm":
		return "kitty"
	case termProgram == "iterm.app":
		return "iterm"
	case strings.Contains(term, "sixel"):
		return "sixels"
	default:
		return ""
	}
}

func normalizeGraphicFormat(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "kitty":
		return "kitty"
	case "iterm", "imgcat":
		return "iterm"
	case "sixel", "sixels":
		return "sixels"
	default:
		return ""
	}
}

func ensurePreviewTools(requireChafa bool) error {
	if err := ensureCommand("pdftoppm"); err != nil {
		return err
	}
	if requireChafa {
		if err := ensureCommand("chafa"); err != nil {
			return err
		}
	}
	return nil
}

func ensureCommand(name string) error {
	if _, err := execLookPath(name); err != nil {
		switch name {
		case "chafa":
			if runtime.GOOS == "darwin" {
				return fmt.Errorf("chafa not found (brew install chafa)")
			}
			return fmt.Errorf("chafa not found (apt install chafa / pacman -S chafa)")
		case "pdftocairo", "pdftoppm":
			if runtime.GOOS == "darwin" {
				return fmt.Errorf("%s not found (brew install poppler)", name)
			}
			return fmt.Errorf("%s not found (apt install poppler-utils / pacman -S poppler)", name)
		default:
			return err
		}
	}
	return nil
}

func ensureGraphicPreviewTools(format string) error {
	switch format {
	case "kitty":
		return ensureCommand("pdftocairo")
	case "iterm", "sixels":
		return ensurePreviewTools(true)
	default:
		return errTerminalGraphicsUnsupported
	}
}

func rasterizeFirstPageToPPM(path string) (string, error) {
	if err := ensurePreviewTools(false); err != nil {
		return "", err
	}

	// 72 DPI is sufficient for previews inside a terminal pane.
	const dpi = 72

	// Derive a deterministic temp-file base from the PDF path and DPI so that
	// successive calls for the same file reuse the already-converted image.
	hash := sha1.Sum([]byte(fmt.Sprintf("%s@%d", path, dpi)))
	hashStr := hex.EncodeToString(hash[:])
	tmpDir := os.TempDir()
	tmpBase := filepath.Join(tmpDir, "gorae_prev_"+hashStr)

	findPPM := func() string {
		matches, _ := filepath.Glob(tmpBase + "-*.ppm")
		if len(matches) == 0 {
			return ""
		}
		sort.Strings(matches)
		return matches[0]
	}

	tmpPPM := findPPM()
	if tmpPPM != "" {
		return tmpPPM, nil
	}

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
		return "", fmt.Errorf("pdftoppm: %w — %s", err, errBuf.String())
	}

	tmpPPM = findPPM()
	if tmpPPM == "" {
		return "", fmt.Errorf("pdftoppm produced no output for %s", filepath.Base(path))
	}
	return tmpPPM, nil
}

func rasterizeFirstPageToPNG(path string) (string, error) {
	if err := ensureCommand("pdftocairo"); err != nil {
		return "", err
	}

	// Use a moderate, fixed-size cached PNG. Kitty handles the final scaling,
	// so rerendering per panel size just wastes time.
	const maxSize = 900

	hash := sha1.Sum([]byte(fmt.Sprintf("%s@png:%d", path, maxSize)))
	hashStr := hex.EncodeToString(hash[:])
	tmpDir := os.TempDir()
	tmpBase := filepath.Join(tmpDir, "gorae_prev_"+hashStr)
	tmpPNG := tmpBase + ".png"
	if _, err := os.Stat(tmpPNG); err == nil {
		return tmpPNG, nil
	}

	cmd := exec.Command(
		"pdftocairo",
		"-png",
		"-singlefile",
		"-f", "1",
		"-l", "1",
		"-scale-to", fmt.Sprintf("%d", maxSize),
		path,
		tmpBase,
	)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftocairo: %w — %s", err, errBuf.String())
	}
	if _, err := os.Stat(tmpPNG); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("pdftocairo produced no output for %s", filepath.Base(path))
		}
		return "", err
	}
	return tmpPNG, nil
}

func stripChafaCursorSequences(output string) string {
	output = strings.ReplaceAll(output, "\x1b[?25l", "")
	output = strings.ReplaceAll(output, "\x1b[?25h", "")
	return output
}

func kittyDeletePreviewSequence() string {
	return fmt.Sprintf("\x1b_Ga=d,d=i,i=%d,q=1\x1b\\", kittyPreviewImageID)
}

func kittyPNGPreviewSequence(pngPath string, imgWidth, imgHeight int) (string, error) {
	data, err := os.ReadFile(pngPath)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("empty PNG preview for %s", filepath.Base(pngPath))
	}

	var b strings.Builder
	for offset := 0; offset < len(data); offset += kittyChunkRawSize {
		end := offset + kittyChunkRawSize
		if end > len(data) {
			end = len(data)
		}
		more := 0
		if end < len(data) {
			more = 1
		}
		payload := base64.StdEncoding.EncodeToString(data[offset:end])
		if offset == 0 {
			fmt.Fprintf(
				&b,
				"\x1b_Ga=T,i=%d,f=100,c=%d,r=%d,C=1,q=1,m=%d;%s\x1b\\",
				kittyPreviewImageID,
				imgWidth,
				imgHeight,
				more,
				payload,
			)
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, payload)
		}
	}
	return b.String(), nil
}

// extractFirstPageGraphicPreview renders the first PDF page as a terminal image
// sequence supported by kitty, iTerm2, or sixel-capable terminals.
func extractFirstPageGraphicPreview(path string, imgWidth, imgHeight int) (string, string, error) {
	format := terminalGraphicFormat()
	if format == "" {
		return "", "", errTerminalGraphicsUnsupported
	}
	if err := ensureGraphicPreviewTools(format); err != nil {
		return "", "", err
	}
	if imgWidth < 4 {
		imgWidth = 4
	}
	if imgHeight < 2 {
		imgHeight = 2
	}

	if format == "kitty" {
		tmpPNG, err := rasterizeFirstPageToPNG(path)
		if err != nil {
			return "", "", err
		}
		rendered, err := kittyPNGPreviewSequence(tmpPNG, imgWidth, imgHeight)
		if err != nil {
			return "", "", err
		}
		return rendered, format, nil
	}

	tmpPPM, err := rasterizeFirstPageToPPM(path)
	if err != nil {
		return "", "", err
	}

	size := fmt.Sprintf("%dx%d", imgWidth, imgHeight)
	args := []string{
		"--probe=off",
		"--format=" + format,
		"--relative=on",
		"--passthrough=auto",
		"--align=top,left",
		"--animate=off",
		"--size=" + size,
		tmpPPM,
	}
	cmd := exec.Command("chafa", args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("chafa %s: %w — %s", format, err, errBuf.String())
	}

	rendered := strings.TrimRight(stripChafaCursorSequences(out.String()), "\n")
	if rendered == "" {
		return "", "", fmt.Errorf("chafa produced no %s output for %s", format, filepath.Base(path))
	}
	return rendered, format, nil
}

// extractFirstPageImagePreview converts the first page of a PDF to terminal
// block-character art using pdftoppm and chafa. imgWidth and imgHeight are the
// desired visual dimensions in terminal columns/rows. Returns one string per
// row; each string may contain ANSI escape codes.
func extractFirstPageImagePreview(path string, imgWidth, imgHeight int) ([]string, error) {
	if err := ensurePreviewTools(true); err != nil {
		return nil, err
	}
	if imgWidth < 4 {
		imgWidth = 4
	}
	if imgHeight < 2 {
		imgHeight = 2
	}

	tmpPPM, err := rasterizeFirstPageToPPM(path)
	if err != nil {
		return nil, err
	}

	if err := ensureCommand("chafa"); err != nil {
		return nil, err
	}

	// Render the PPM as terminal art sized to the panel.
	// --symbols=half uses half-block characters (▀▄) which pack two pixels per
	// character row, giving 2× effective vertical resolution over full-block art.
	// --colors=256 is safe on virtually every terminal.
	size := fmt.Sprintf("%dx%d", imgWidth, imgHeight)
	cmd := exec.Command(
		"chafa",
		"--size="+size,
		"--format=symbols",
		"--symbols=half",
		"--colors=256",
		tmpPPM,
	)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("chafa: %w — %s", err, errBuf.String())
	}

	output := stripChafaCursorSequences(out.String())
	raw := strings.TrimRight(output, "\n")
	lines := strings.Split(raw, "\n")
	return lines, nil
}
