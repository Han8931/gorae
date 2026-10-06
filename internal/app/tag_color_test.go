package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Han8931/gorae/internal/theme"
)

// TestTagCircleColorStable verifies a tag always maps to the same palette color.
func TestTagCircleColorStable(t *testing.T) {
	if got, want := tagCircleColor("nlp"), tagCircleColor("nlp"); got != want {
		t.Fatalf("tagCircleColor not stable: %q vs %q", got, want)
	}
	if c := tagCircleColor("nlp"); c == "" {
		t.Fatal("tagCircleColor returned empty color")
	}
}

// TestTagCircleColorSharesRoot verifies hierarchical tags under the same
// top-level segment share a hue, reading as a family in the tree.
func TestTagCircleColorSharesRoot(t *testing.T) {
	if tagCircleColor("ml/nlp") != tagCircleColor("ml/cv") {
		t.Fatal("tags sharing a root segment should share a color")
	}
}

// TestStyledTagLineHasCircle verifies the composed tag row renders the circle
// glyph, the tag name, and fills the panel's inner width.
func TestStyledTagLineHasCircle(t *testing.T) {
	var m Model
	m.applyTheme(theme.Default())
	n := treeNode{name: "nlp", tag: "nlp", depth: 1, kind: treeNodeTag}
	innerWidth := 18

	line := m.styledTagLine(n, lipgloss.NewStyle(), innerWidth)
	if !strings.Contains(line, "●") {
		t.Fatalf("styled tag line missing circle glyph: %q", line)
	}
	if !strings.Contains(line, "nlp") {
		t.Fatalf("styled tag line missing tag name: %q", line)
	}
	if w := lipgloss.Width(line); w != innerWidth {
		t.Fatalf("styled tag line width = %d, want %d", w, innerWidth)
	}
}
