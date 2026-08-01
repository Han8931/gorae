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
	m := Model{width: 120, treePaneHidden: true}
	left, middle, right := m.panelWidths()
	gap := panelSeparatorWidth / 2
	if left != 0 {
		t.Fatalf("hidden tree width = %d, want 0", left)
	}
	if got := middle + right + gap; got != m.width {
		t.Fatalf("panels use %d columns, want %d", got, m.width)
	}
}

func TestHiddenTreeMovesListHitAreaToLeftEdge(t *testing.T) {
	m := Model{width: 120, viewportHeight: 18, treePaneHidden: true}
	if _, ok := m.clickInListPanel(tea.MouseMsg{X: 0, Y: 2}); !ok {
		t.Fatal("expected list hit area at the left edge when tree is hidden")
	}
}
