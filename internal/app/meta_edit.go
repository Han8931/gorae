package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The editing view ('e') shows one row per metadata field in place of the Files
// pane. Free-text rows are typed into the prompt line above the status bar;
// reading state and the two flags are changed on the row itself; the note and
// the whole record can still be handed to $EDITOR. Every change is saved as
// soon as it is made, so Esc is always a safe way out.

// metaEmptyValue marks a field with nothing in it.
const metaEmptyValue = "—"

// metaRowExpandLines caps how much of a long value is unfolded under the
// cursor, so a page-long abstract cannot push the rest of the rows off screen.
const metaRowExpandLines = 8

type metaRowKind int

const (
	metaRowText  metaRowKind = iota // free-text field, typed into the prompt line
	metaRowState                    // reading state, cycled in place
	metaRowFlag                     // Favorite / To-read, toggled in place
	metaRowNote                     // the Markdown note, edited in $EDITOR
)

type metaRow struct {
	kind  metaRowKind
	label string
	field int          // metaFieldLabels index, when kind is metaRowText
	flag  metadataFlag // which flag, when kind is metaRowFlag
}

// metaRows is the row list of the editing view, in display order.
func (m Model) metaRows() []metaRow {
	rows := make([]metaRow, 0, metaFieldCount()+4)
	for i := 0; i < metaFieldCount(); i++ {
		rows = append(rows, metaRow{kind: metaRowText, label: metaFieldLabel(i), field: i})
	}
	rows = append(rows,
		metaRow{kind: metaRowState, label: "Reading"},
		metaRow{kind: metaRowFlag, label: "Favorite", flag: metadataFlagFavorite},
		metaRow{kind: metaRowFlag, label: "To-read", flag: metadataFlagToRead},
	)
	// A Markdown document is its own note, so it has no separate note file.
	if !isMarkdown(m.metaEditingPath) {
		rows = append(rows, metaRow{kind: metaRowNote, label: "Note"})
	}
	return rows
}

func (m Model) metaRowCount() int {
	return len(m.metaRows())
}

// currentMetaRow returns the row under the cursor, clamped to the row list.
func (m Model) currentMetaRow() (metaRow, bool) {
	rows := m.metaRows()
	if len(rows) == 0 {
		return metaRow{}, false
	}
	index := clampInt(m.metaRowIndex, 0, len(rows)-1)
	return rows[index], true
}

// metaRowValue is the one-line value shown next to a row's label.
func (m Model) metaRowValue(row metaRow) string {
	switch row.kind {
	case metaRowState:
		state := normalizeReadingStateValue(m.metaDraft.ReadingState)
		return fmt.Sprintf("%s %s", m.readingStateIcon(state), readingStateLabel(state))
	case metaRowFlag:
		on, icon := m.metaDraft.Favorite, m.favoriteIcon()
		if row.flag == metadataFlagToRead {
			on, icon = m.metaDraft.ToRead, m.toReadIcon()
		}
		if on {
			return icon + " Yes"
		}
		return "· No"
	case metaRowNote:
		note := strings.TrimSpace(m.metaNote)
		if note == "" {
			return metaEmptyValue
		}
		if count := strings.Count(note, "\n") + 1; count > 1 {
			return fmt.Sprintf("%d lines", count)
		}
		return "1 line"
	default:
		value := collapseSpaces(metadataFieldValue(m.metaDraft, row.field))
		if value == "" {
			return metaEmptyValue
		}
		return value
	}
}

// metaRowDetail is the full text unfolded beneath the focused row when its
// value does not fit on one line.
func (m Model) metaRowDetail(row metaRow) string {
	switch row.kind {
	case metaRowNote:
		return strings.TrimSpace(m.metaNote)
	case metaRowText:
		return collapseSpaces(metadataFieldValue(m.metaDraft, row.field))
	default:
		return ""
	}
}

// collapseSpaces folds runs of whitespace (including the newlines an abstract
// picks up from a PDF) into single spaces so a value fits on one row.
func collapseSpaces(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// ── entering and leaving ──────────────────────────────────────────────────────

// openMetaEditor points the editing view at path and loads its stored record.
func (m *Model) openMetaEditor(path string) {
	canonical := canonicalPath(path)
	m.state = stateMetaPreview
	m.metaEditingPath = canonical
	m.metaRowIndex = 0
	m.metaPopupOffset = 0
	m.input.SetValue("")
	m.input.Blur()

	if err := m.reloadMetaDraft(); err != nil {
		m.setStatus("Failed to load metadata: " + err.Error())
		return
	}
	m.metaNote = m.readNoteFor(canonical)
	m.setPersistentStatus(metaEditorHint())
}

// closeMetaEditor returns to the file list.
func (m *Model) closeMetaEditor(status string) {
	m.state = stateNormal
	m.metaEditingPath = ""
	m.metaNote = ""
	m.metaPopupOffset = 0
	m.metaRowIndex = 0
	m.input.SetValue("")
	m.input.Blur()
	m.setStatus(status)
}

func metaEditorHint() string {
	return "Metadata: Enter edit · r/f/t state & flags · E editor · Esc close"
}

// ── key handling ──────────────────────────────────────────────────────────────

// handleMetaEditorKey drives the editing view's row navigation and actions.
func (m Model) handleMetaEditorKey(key string) (tea.Model, tea.Cmd) {
	rows := m.metaRows()
	if len(rows) == 0 {
		m.closeMetaEditor("Nothing to edit")
		return m, nil
	}
	m.metaRowIndex = clampInt(m.metaRowIndex, 0, len(rows)-1)

	switch key {
	case "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		m.moveMetaRow(1)
	case "k", "up":
		m.moveMetaRow(-1)
	case "tab":
		m.moveMetaRowWrapping(1)
	case "shift+tab":
		m.moveMetaRowWrapping(-1)
	case "g", "home":
		m.setMetaRow(0)
	case "G", "end":
		m.setMetaRow(len(rows) - 1)
	case "ctrl+d", "pgdown":
		m.moveMetaRow(m.metaPageStep())
	case "ctrl+u", "pgup":
		m.moveMetaRow(-m.metaPageStep())

	case "enter", "e", " ":
		row := rows[m.metaRowIndex]
		return m, m.activateMetaRow(row)

	case "E":
		if cmd := m.launchMetadataEditor(); cmd != nil {
			return m, cmd
		}
	case "n":
		if cmd := m.editMetaNote(); cmd != nil {
			return m, cmd
		}
	case "r":
		m.focusMetaRowKind(metaRowState, 0)
		m.setMetaReadingState(nextReadingState(m.metaDraft.ReadingState))
	case "f":
		m.focusMetaRowKind(metaRowFlag, metadataFlagFavorite)
		m.toggleMetaDraftFlag(metadataFlagFavorite)
	case "t":
		m.focusMetaRowKind(metaRowFlag, metadataFlagToRead)
		m.toggleMetaDraftFlag(metadataFlagToRead)

	case "esc", "q":
		m.closeMetaEditor("Metadata closed")
	}
	return m, nil
}

// handleMetaFieldKey drives the prompt line while one text field is being typed.
// Enter commits and returns to the row list; Tab commits and keeps editing on
// the neighbouring text row, so a record can be filled in one pass.
func (m Model) handleMetaFieldKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	row, ok := m.currentMetaRow()
	if !ok || row.kind != metaRowText {
		m.exitMetaFieldEdit()
		m.state = stateMetaPreview
		return m, nil
	}

	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.exitMetaFieldEdit()
		m.state = stateMetaPreview
		m.setPersistentStatus(row.label + " unchanged")
		return m, nil
	case "enter":
		if !m.commitMetaField(row, m.input.Value()) {
			return m, nil // rejected: stay in the prompt so the text is not lost
		}
		m.exitMetaFieldEdit()
		m.state = stateMetaPreview
		return m, nil
	case "tab", "shift+tab":
		if !m.commitMetaField(row, m.input.Value()) {
			return m, nil
		}
		step := 1
		if key == "shift+tab" {
			step = -1
		}
		if !m.focusNextTextRow(step) {
			m.exitMetaFieldEdit()
			m.state = stateMetaPreview
			return m, nil
		}
		next, _ := m.currentMetaRow()
		m.loadMetaFieldIntoInput()
		m.setPersistentStatus(metaFieldEditHint(next.label))
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// activateMetaRow performs the row's edit action: text rows open the prompt
// line, state and flag rows change in place, the note row opens $EDITOR.
func (m *Model) activateMetaRow(row metaRow) tea.Cmd {
	switch row.kind {
	case metaRowText:
		m.enterMetaFieldEdit(row)
		return nil
	case metaRowState:
		m.setMetaReadingState(nextReadingState(m.metaDraft.ReadingState))
		return nil
	case metaRowFlag:
		m.toggleMetaDraftFlag(row.flag)
		return nil
	case metaRowNote:
		return m.editMetaNote()
	}
	return nil
}

// editMetaNote opens the note in $EDITOR, moving the cursor onto the note row
// first so the view reflects what is being edited.
func (m *Model) editMetaNote() tea.Cmd {
	if isMarkdown(m.metaEditingPath) {
		m.setPersistentStatus("A Markdown document is its own note")
		return nil
	}
	m.focusMetaRowKind(metaRowNote, 0)
	return m.launchNoteEditor()
}

// ── row navigation ────────────────────────────────────────────────────────────

func (m *Model) setMetaRow(index int) {
	rows := m.metaRows()
	if len(rows) == 0 {
		return
	}
	m.metaRowIndex = clampInt(index, 0, len(rows)-1)
	m.ensureMetaRowVisible()
}

func (m *Model) moveMetaRow(delta int) {
	m.setMetaRow(m.metaRowIndex + delta)
}

func (m *Model) moveMetaRowWrapping(delta int) {
	rows := m.metaRows()
	if len(rows) == 0 {
		return
	}
	m.metaRowIndex = ((m.metaRowIndex+delta)%len(rows) + len(rows)) % len(rows)
	m.ensureMetaRowVisible()
}

// focusMetaRowKind moves the cursor onto the first row of the given kind, so a
// direct key such as 'f' highlights the row it changed. flag is only consulted
// for metaRowFlag.
func (m *Model) focusMetaRowKind(kind metaRowKind, flag metadataFlag) {
	for i, row := range m.metaRows() {
		if row.kind != kind {
			continue
		}
		if kind == metaRowFlag && row.flag != flag {
			continue
		}
		m.setMetaRow(i)
		return
	}
}

// focusNextTextRow moves to the next (or previous) free-text row, reporting
// false when there is none left in that direction.
func (m *Model) focusNextTextRow(step int) bool {
	rows := m.metaRows()
	for i := m.metaRowIndex + step; i >= 0 && i < len(rows); i += step {
		if rows[i].kind == metaRowText {
			m.setMetaRow(i)
			return true
		}
	}
	return false
}

func (m Model) metaPageStep() int {
	step := m.metaEditorBodyRows() / 2
	if step < 1 {
		step = 1
	}
	return step
}

// ── editing one text field ────────────────────────────────────────────────────

// defaultInputCharLimit bounds the shared prompt input. Metadata fields lift it
// while they are being edited because an abstract is routinely longer.
const defaultInputCharLimit = 200

func (m *Model) enterMetaFieldEdit(row metaRow) {
	m.state = stateMetaField
	// Raise the limit before SetValue: the input truncates to CharLimit.
	m.input.CharLimit = 0
	m.input.Width = m.metaPromptWidth(row.label)
	m.loadMetaFieldIntoInput()
	m.input.Focus()
	m.setPersistentStatus(metaFieldEditHint(row.label))
}

func (m *Model) exitMetaFieldEdit() {
	m.input.SetValue("")
	m.input.Blur()
	m.input.CharLimit = defaultInputCharLimit
	m.input.Width = 0
}

// metaPromptWidth is how much room the prompt line leaves for the value, so a
// long abstract scrolls inside the input instead of overflowing the row.
func (m Model) metaPromptWidth(label string) int {
	width := m.width
	if width <= 0 {
		width = 80
	}
	// " LABEL " chip, the leading space before the value, and a cursor cell.
	width -= len(label) + 4
	if width < 10 {
		width = 10
	}
	return width
}

// loadMetaFieldIntoInput puts the focused text field's stored value into the
// prompt input, ready to be edited.
func (m *Model) loadMetaFieldIntoInput() {
	row, ok := m.currentMetaRow()
	if !ok || row.kind != metaRowText {
		m.input.SetValue("")
		return
	}
	m.input.SetValue(metadataFieldValue(m.metaDraft, row.field))
	m.input.CursorEnd()
}

func metaFieldEditHint(label string) string {
	return fmt.Sprintf("%s: Enter saves · Tab saves and moves on · Esc cancels", label)
}

// commitMetaField writes the typed value into the draft and saves it, reporting
// false when the value was rejected and the prompt should stay open.
func (m *Model) commitMetaField(row metaRow, raw string) bool {
	value := strings.TrimSpace(raw)
	if err := validateMetaField(row, value); err != nil {
		m.setPersistentStatus(err.Error())
		return false
	}
	if value == strings.TrimSpace(metadataFieldValue(m.metaDraft, row.field)) {
		m.setPersistentStatus(row.label + " unchanged")
		return true
	}
	setMetadataFieldValue(&m.metaDraft, row.field, value)
	if err := m.saveMetaDraft(); err != nil {
		// The typed value stays in the draft so it is visible, not lost.
		m.setPersistentStatus("Failed to save metadata: " + err.Error())
		return true
	}
	m.setPersistentStatus(row.label + " saved")
	return true
}

// validateMetaField rejects values that would quietly break something else —
// today that is only a Year the year sort and BibTeX export cannot use.
func validateMetaField(row metaRow, value string) error {
	if row.kind != metaRowText || !strings.EqualFold(row.label, "Year") || value == "" {
		return nil
	}
	if !containsFourDigitYear(value) {
		return errors.New("Year needs a 4-digit year (e.g. 2017) — Esc to cancel")
	}
	return nil
}

func containsFourDigitYear(value string) bool {
	run := 0
	for _, r := range value {
		if r >= '0' && r <= '9' {
			run++
			if run == 4 {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

// ── in-place changes ──────────────────────────────────────────────────────────

func (m *Model) setMetaReadingState(state string) {
	m.metaDraft.ReadingState = normalizeReadingStateValue(state)
	if err := m.saveMetaDraft(); err != nil {
		m.setPersistentStatus("Failed to save metadata: " + err.Error())
		return
	}
	if m.cwdIsStateView {
		// The document now belongs to a different reading-state tab; rebuild the
		// helper views so it has left this one by the time the editor closes.
		if err := m.syncReadingStateViews(); err != nil {
			m.setPersistentStatus("Reading state view sync failed: " + err.Error())
			return
		}
	}
	m.setPersistentStatus("Reading state: " + readingStateLabel(m.metaDraft.ReadingState))
}

func (m *Model) toggleMetaDraftFlag(flag metadataFlag) {
	label := "Favorite"
	switch flag {
	case metadataFlagFavorite:
		m.metaDraft.Favorite = !m.metaDraft.Favorite
	case metadataFlagToRead:
		label = "To-read"
		m.metaDraft.ToRead = !m.metaDraft.ToRead
	}
	if err := m.saveMetaDraft(); err != nil {
		m.setPersistentStatus("Failed to save metadata: " + err.Error())
		return
	}
	if err := m.syncCollectionDirectories(); err != nil {
		m.setPersistentStatus("Failed to sync Favorites/To-read directories: " + err.Error())
		return
	}
	state := "off"
	if (flag == metadataFlagFavorite && m.metaDraft.Favorite) || (flag == metadataFlagToRead && m.metaDraft.ToRead) {
		state = "on"
	}
	m.setPersistentStatus(fmt.Sprintf("%s %s", label, state))
}

// saveMetaDraft persists the draft and keeps the list and detail panes in sync.
// The draft carries the flags and timestamps it was loaded with, so the
// write-every-column upsert cannot drop them.
func (m *Model) saveMetaDraft() error {
	if m.meta == nil {
		return errors.New("metadata store not available")
	}
	target := strings.TrimSpace(m.metaEditingPath)
	if target == "" {
		return errors.New("no metadata target selected")
	}
	md := m.metaDraft
	md.Path = target
	md.ReadingState = normalizeReadingStateValue(md.ReadingState)
	if err := m.meta.Upsert(context.Background(), &md); err != nil {
		return err
	}
	m.metaDraft = md
	m.refreshMetadataCache(target, md)
	m.refreshEntryTitles()
	m.resortAndPreserveSelection()
	return nil
}

// readNoteFor returns the note body stored for path, or "" when there is none.
func (m Model) readNoteFor(path string) string {
	if path == "" {
		return ""
	}
	filePath, err := m.noteFilePath(path)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return string(data)
}

// ── rendering ─────────────────────────────────────────────────────────────────

// metaEditorLine is one rendered row of the editing view. row is the index into
// metaRows, or -1 for spacers and the unfolded continuation lines, which must
// not become cursor targets.
type metaEditorLine struct {
	row  int
	line panelLine
}

// metaEditorBodyRows is how many content rows the panel can show: the pane
// height minus the two border rows and the header.
func (m Model) metaEditorBodyRows() int {
	rows := m.paneHeight() - 3
	if rows < 1 {
		rows = 1
	}
	return rows
}

// metaEditorLines builds every line of the editing view for a pane of the given
// width, grouping the text fields, the status rows and the note.
func (m Model) metaEditorLines(width int) []metaEditorLine {
	rows := m.metaRows()
	if len(rows) == 0 {
		return nil
	}
	usable := panelContentUsableWidth(width)
	if usable < 8 {
		usable = width
	}
	labelWidth := 0
	for _, row := range rows {
		if l := len(row.label); l > labelWidth {
			labelWidth = l
		}
	}

	cursor := clampInt(m.metaRowIndex, 0, len(rows)-1)
	out := make([]metaEditorLine, 0, len(rows)+metaRowExpandLines+4)
	spacer := func() {
		out = append(out, metaEditorLine{row: -1, line: panelLine{}})
	}

	prevKind := rows[0].kind
	for i, row := range rows {
		// A blank line between the text fields, the status rows and the note.
		if i > 0 && metaRowGroup(row.kind) != metaRowGroup(prevKind) {
			spacer()
		}
		prevKind = row.kind

		focused := i == cursor
		kind := panelLineStyled
		if focused {
			kind = panelLineCursor
		}
		var detail []string
		if focused {
			detail = m.metaRowDetailLines(row, labelWidth, usable)
		}
		// A text field unfolded in full below does not also repeat itself, cut
		// short, on the row. A note's "N lines" summary is not a repeat, so it
		// stays.
		hideValue := len(detail) > 0 && row.kind == metaRowText
		text := m.metaEditorRowText(row, labelWidth, usable, !focused, hideValue)
		out = append(out, metaEditorLine{row: i, line: panelLine{text: text, kind: kind}})

		for _, line := range detail {
			out = append(out, metaEditorLine{row: -1, line: panelLine{text: line, kind: panelLineInfo}})
		}
	}
	return out
}

// metaRowGroup buckets rows so a blank line can separate the text fields from
// the status toggles and the note.
func metaRowGroup(kind metaRowKind) int {
	switch kind {
	case metaRowState, metaRowFlag:
		return 1
	case metaRowNote:
		return 2
	default:
		return 0
	}
}

// metaEditorRowText renders "▸ Label  value". The label is coloured only when
// the row is not focused: the panel paints the focused row as a whole, and an
// inline colour reset would cut that highlight short. hideValue drops the value
// for a row whose full text is unfolded beneath it.
func (m Model) metaEditorRowText(row metaRow, labelWidth, usable int, colored, hideValue bool) string {
	marker := "  "
	if !colored {
		marker = "▸ "
	}
	// hideValue only ever applies to the focused row, which is never coloured
	// inline, so the bare label can be trimmed safely.
	if hideValue {
		return trimLine(marker+row.label, usable)
	}
	label := fmt.Sprintf("%-*s", labelWidth, row.label)
	if colored {
		label = m.styles.Preview.Info.Render(label)
	}
	value := m.metaRowValue(row)
	valueWidth := metaRowValueWidth(labelWidth, usable)
	if lipgloss.Width(value) > valueWidth {
		value = trimLine(value, valueWidth-1) + "…"
	}
	return trimLine(marker+label+"  "+value, usable)
}

// metaRowValueWidth is how much room a row leaves for its value: the usable
// pane width less the cursor marker, the label column and their separators.
func metaRowValueWidth(labelWidth, usable int) int {
	width := usable - labelWidth - 4
	if width < 4 {
		width = 4
	}
	return width
}

// metaRowDetailLines unfolds the focused row's full value beneath it when the
// one-line form had to be cut short.
func (m Model) metaRowDetailLines(row metaRow, labelWidth, usable int) []string {
	detail := m.metaRowDetail(row)
	if strings.TrimSpace(detail) == "" {
		return nil
	}
	indent := 4
	wrapWidth := usable - indent
	if wrapWidth < 8 {
		return nil
	}
	// Nothing to unfold when the value already fits on the row itself.
	if row.kind != metaRowNote && lipgloss.Width(detail) <= metaRowValueWidth(labelWidth, usable) {
		return nil
	}
	// A note keeps its own line breaks; a field value is one wrapped paragraph.
	var wrapped []string
	if row.kind == metaRowNote {
		for _, line := range strings.Split(detail, "\n") {
			if strings.TrimSpace(line) == "" {
				wrapped = append(wrapped, "")
				continue
			}
			wrapped = append(wrapped, wrapTextToWidth(line, wrapWidth)...)
		}
	} else {
		wrapped = wrapTextToWidth(detail, wrapWidth)
	}
	truncated := false
	if len(wrapped) > metaRowExpandLines {
		wrapped = wrapped[:metaRowExpandLines]
		truncated = true
	}
	pad := strings.Repeat(" ", indent)
	out := make([]string, 0, len(wrapped)+1)
	for _, line := range wrapped {
		out = append(out, pad+line)
	}
	if truncated {
		out = append(out, pad+"…")
	}
	return out
}

// renderMetaEditorPanel draws the editing view in place of the Files pane.
func (m Model) renderMetaEditorPanel(width, height int) []string {
	lines := m.metaEditorLines(width)
	bodyRows := height - 3
	if bodyRows < 1 {
		bodyRows = 1
	}
	offset := clampInt(m.metaPopupOffset, 0, maxInt(0, len(lines)-bodyRows))

	visible := make([]panelLine, 0, bodyRows)
	for i := offset; i < len(lines) && len(visible) < bodyRows; i++ {
		visible = append(visible, lines[i].line)
	}
	title := m.metaEditorTitle(width, offset, len(lines), bodyRows)
	return m.renderPanelBlock(title, visible, width, height, m.styles.List)
}

// metaEditorTitle is the header row: the file on the left, the cursor position
// and a scroll marker on the right.
func (m Model) metaEditorTitle(width, offset, total, bodyRows int) string {
	name := filepath.Base(m.metaEditingPath)
	if name == "" || name == "." {
		name = m.metaEditingPath
	}
	left := "Metadata · " + name

	right := fmt.Sprintf("%d/%d", clampInt(m.metaRowIndex, 0, maxInt(0, m.metaRowCount()-1))+1, m.metaRowCount())
	if total > bodyRows {
		switch {
		case offset <= 0:
			right += " ↓"
		case offset+bodyRows >= total:
			right += " ↑"
		default:
			right += " ↕"
		}
	}

	usable := panelContentUsableWidth(width)
	if usable <= 0 {
		return left
	}
	// The file name is also in the status bar, so trim it rather than lose the
	// position marker. Only a pane too narrow for both drops the marker.
	rightWidth := lipgloss.Width(right)
	if rightWidth+12 > usable {
		return trimLine(left, usable)
	}
	if maxLeft := usable - rightWidth - 2; lipgloss.Width(left) > maxLeft {
		left = trimLine(left, maxLeft-1) + "…"
	}
	gap := usable - lipgloss.Width(left) - rightWidth
	return left + strings.Repeat(" ", gap) + right
}

// ── scrolling ─────────────────────────────────────────────────────────────────

// ensureMetaRowVisible scrolls the panel just far enough to show the cursor row
// and, where it fits, the lines unfolded beneath it.
func (m *Model) ensureMetaRowVisible() {
	_, middleWidth, _ := m.panelWidths()
	if middleWidth <= 0 {
		m.metaPopupOffset = 0
		return
	}
	lines := m.metaEditorLines(middleWidth)
	bodyRows := m.metaEditorBodyRows()
	if len(lines) <= bodyRows {
		m.metaPopupOffset = 0
		return
	}

	first, last := -1, -1
	for i, line := range lines {
		if line.row == m.metaRowIndex {
			first = i
			last = i
			continue
		}
		// Continuation lines follow their row, so extend the span through them.
		if first >= 0 && last == i-1 && line.row < 0 {
			last = i
		}
	}
	if first < 0 {
		m.clampMetaEditorOffset(len(lines), bodyRows)
		return
	}

	if m.metaPopupOffset > first {
		m.metaPopupOffset = first
	}
	// Keep the row itself on screen even when its unfolded lines do not fit.
	if wanted := last - bodyRows + 1; wanted > m.metaPopupOffset {
		m.metaPopupOffset = minInt(wanted, first)
	}
	m.clampMetaEditorOffset(len(lines), bodyRows)
}

func (m *Model) clampMetaEditorOffset(total, bodyRows int) {
	m.metaPopupOffset = clampInt(m.metaPopupOffset, 0, maxInt(0, total-bodyRows))
}

// scrollMetaPopup scrolls the editing view by delta lines, used by the mouse
// wheel; the keyboard moves the row cursor instead.
func (m *Model) scrollMetaPopup(delta int) {
	if delta == 0 {
		return
	}
	if m.state != stateMetaPreview && m.state != stateMetaField {
		m.metaPopupOffset = 0
		return
	}
	_, middleWidth, _ := m.panelWidths()
	if middleWidth <= 0 {
		m.metaPopupOffset = 0
		return
	}
	m.metaPopupOffset += delta
	m.clampMetaEditorOffset(len(m.metaEditorLines(middleWidth)), m.metaEditorBodyRows())
}
