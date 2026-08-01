package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNavigationPrefixTogglesTreePane(t *testing.T) {
	m := Model{}
	if !m.handleNavigationPrefix(",") {
		t.Fatal("expected comma to start the navigation prefix")
	}
	if !m.handleNavigationPrefix("n") || !m.treePaneHidden {
		t.Fatal("expected ,n to hide the tree pane")
	}
	m.handleNavigationPrefix(",")
	m.handleNavigationPrefix("n")
	if m.treePaneHidden {
		t.Fatal("expected a second ,n to show the tree pane")
	}
}

func TestHiddenTreePaneReclaimsWidth(t *testing.T) {
	visible := Model{width: 120}
	_, visibleMiddle, visibleRight := visible.panelWidths()
	hidden := Model{width: 120, treePaneHidden: true}
	left, middle, right := hidden.panelWidths()
	gap := panelSeparatorWidth / 2
	if left != 0 {
		t.Fatalf("hidden tree width = %d, want 0", left)
	}
	if got := middle + right + gap; got != hidden.width {
		t.Fatalf("panels use %d columns, want %d", got, hidden.width)
	}
	if middle <= visibleMiddle {
		t.Fatalf("hidden-tree list width = %d, want more than %d", middle, visibleMiddle)
	}
	if right <= visibleRight {
		t.Fatalf("hidden-tree detail width = %d, want more than %d", right, visibleRight)
	}
}

func TestHiddenTreeMovesListHitAreaToLeftEdge(t *testing.T) {
	m := Model{width: 120, viewportHeight: 18, treePaneHidden: true}
	if _, ok := m.clickInListPanel(tea.MouseMsg{X: 0, Y: 2}); !ok {
		t.Fatal("expected list hit area at the left edge when tree is hidden")
	}
}
