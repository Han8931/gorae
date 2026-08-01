package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDailyLaunchQuoteDeterministic(t *testing.T) {
	day := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	same := time.Date(2026, 8, 1, 23, 30, 0, 0, time.UTC)
	next := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)

	q := dailyLaunchQuote(day)
	if q.Text == "" {
		t.Fatal("expected a non-empty quote")
	}
	if got := dailyLaunchQuote(same); got != q {
		t.Fatalf("quote changed within the same day: %q vs %q", got.Text, q.Text)
	}
	// Not a hard guarantee, but the curated set is large enough that adjacent
	// days should differ; if this ever flakes, it is fine to relax.
	if got := dailyLaunchQuote(next); got == q && len(launchQuotes) > 1 {
		t.Logf("adjacent-day quote matched (hash collision): %q", q.Text)
	}
}

func TestLaunchMenuNavigationWraps(t *testing.T) {
	m := Model{state: stateLaunch}
	n := len(launchItems())

	// Up from the top wraps to the last row.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.launchCursor != n-1 {
		t.Fatalf("up from top: got cursor %d, want %d", m.launchCursor, n-1)
	}

	// Down from the last row wraps back to the top.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.launchCursor != 0 {
		t.Fatalf("down from bottom: got cursor %d, want 0", m.launchCursor)
	}
}

func TestLaunchEscOpensLibrary(t *testing.T) {
	m := Model{state: stateLaunch}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != stateNormal {
		t.Fatalf("esc should open the library (stateNormal), got %v", m.state)
	}
}

func TestLaunchOpenLibraryAction(t *testing.T) {
	m := Model{state: stateLaunch, launchCursor: 1} // "Open library"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != stateNormal {
		t.Fatalf("selecting Open library should enter stateNormal, got %v", m.state)
	}
}
