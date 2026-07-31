package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/cursor"
	key "github.com/charmbracelet/bubbles/key"
	textarea "github.com/charmbracelet/bubbles/textarea"
	textinput "github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Han8931/gorae/internal/ai"
	"github.com/Han8931/gorae/internal/config"
	"github.com/Han8931/gorae/internal/meta"
	"github.com/Han8931/gorae/internal/search"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type goraeSpinnerTickMsg struct{}

func goraeSpinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return goraeSpinnerTickMsg{}
	})
}

// ── messages ──────────────────────────────────────────────────────────────────

type aiTokenMsg struct {
	text      string
	toolCalls []ai.ToolCall
	done      bool
	err       error
	ch        <-chan ai.StreamToken
}

type aiSourcesMsg struct {
	paths []string
}

type aiCompactDoneMsg struct {
	summary string
	kept    int
	err     error
}

const autoCompactThreshold = 40 // auto-compact after this many messages

// Placeholder shown in the chat input in its default (ask-anything) state and
// while the /load fuzzy-find box is active, respectively.
const (
	goraeAskPlaceholder  = "ask anything…  (Ctrl+J newline · Esc navigate · /help)"
	goraeFindPlaceholder = "type to search titles & contents…"
)

// Chat input box geometry: the multi-line textarea shows goraeInputTextareaRows
// rows, and the whole boxed region (badge/hint header + top & bottom borders)
// occupies goraeInputBoxRows rows in the layout.
const (
	goraeInputTextareaRows = 3
	goraeInputBoxRows      = goraeInputTextareaRows + 3
)

type aiLiveFindMsg struct {
	query   string
	results []meta.NameMatch
}

// aiFocusResolvedMsg carries papers the no-tools resolver pulled into context so
// the main update loop can add them to the focus set and persist them.
type aiFocusResolvedMsg struct {
	paths []string
}

func (m *Model) doLiveFind(query string) tea.Cmd {
	store := m.meta
	return func() tea.Msg {
		return aiLiveFindMsg{query: query, results: searchPapers(context.Background(), store, query, 8)}
	}
}

// searchPapers finds papers by title/filename (SearchByName) and by full-text
// content (SearchFTS), merged with title matches first and content-only matches
// carrying a snippet. An empty query lists everything (up to limit). This is the
// shared engine for both the /load box and the find_papers tool.
func searchPapers(ctx context.Context, store *meta.Store, query string, limit int) []meta.NameMatch {
	if store == nil {
		return nil
	}
	if limit <= 0 {
		limit = 8
	}
	q := strings.TrimSpace(query)

	results, err := store.SearchByName(ctx, q, limit)
	if err != nil {
		results = nil
	}
	seen := make(map[string]bool, len(results))
	for _, r := range results {
		seen[r.Path] = true
	}

	if ftsQuery := buildFTSPrefixQuery(q); ftsQuery != "" {
		if hits, err := store.SearchFTS(ctx, ftsQuery, limit); err == nil {
			for _, h := range hits {
				if seen[h.Path] {
					continue
				}
				seen[h.Path] = true
				results = append(results, meta.NameMatch{
					Path:    h.Path,
					Title:   titleForPath(store, ctx, h.Path),
					Snippet: cleanFTSSnippet(h.Snippet),
				})
			}
		}
	}

	return results
}

// buildFTSPrefixQuery turns a free-text query into a safe FTS5 MATCH expression.
// Each word becomes a prefix term (word*) so results update as the user types;
// non-alphanumeric characters are dropped to avoid FTS5 syntax errors. Returns
// "" when there is nothing searchable (e.g. an empty query).
func buildFTSPrefixQuery(query string) string {
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(fields) == 0 {
		return ""
	}
	for i, f := range fields {
		fields[i] = f + "*"
	}
	return strings.Join(fields, " ")
}

// cleanFTSSnippet flattens an FTS snippet to a single line: the '**' markers that
// bracket the matched terms are removed and runs of whitespace are collapsed.
func cleanFTSSnippet(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	return strings.Join(strings.Fields(s), " ")
}

// titleForPath returns the stored metadata title for a file, falling back to the
// filename stem when no title is recorded.
func titleForPath(store *meta.Store, ctx context.Context, path string) string {
	if md, err := store.Get(ctx, path); err == nil && md != nil && strings.TrimSpace(md.Title) != "" {
		return md.Title
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// ── entry / exit ──────────────────────────────────────────────────────────────

// newGoraeTextarea builds the multi-line chat message box with its static
// configuration. Enter sends (handled in the key loop), so newline insertion is
// rebound to Ctrl+J. It must be created via textarea.New() — a zero-value
// textarea.Model panics on SetWidth — so this is used both at model construction
// and when (re)entering the chat.
func newGoraeTextarea() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = goraeAskPlaceholder
	ta.CharLimit = 4096
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.EndOfBufferCharacter = ' '
	ta.SetHeight(goraeInputTextareaRows)
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "newline"))
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle() // no full-width current-line highlight
	ta.Cursor.SetMode(cursor.CursorStatic)
	return ta
}

func (m *Model) enterGoraeChat() tea.Cmd {
	// Always enter the chat view; errors are shown as chat messages so the
	// user can see them clearly instead of a brief status-bar flash.
	var welcomeMsg string

	if m.cfg == nil || m.cfg.AI == nil {
		welcomeMsg = "No AI config found.\n\n" +
			"Add an \"ai\" block to your config.json, for example:\n\n" +
			"  \"ai\": {\n" +
			"    \"provider\": \"openai\",\n" +
			"    \"model\": \"gpt-4o-mini\",\n" +
			"    \"api_key\": \"sk-...\"\n" +
			"  }\n\n" +
			"For Ollama: set provider to \"ollama\" (no api_key needed).\n" +
			"For any OpenAI-compatible API: set provider to \"custom\" and provide base_url.\n\n" +
			"Press Esc to return."
	} else {
		client, err := ai.NewClient(m.cfg.AI)
		if err != nil {
			welcomeMsg = "AI config error: " + err.Error() + "\n\nPress Esc to return."
		} else {
			m.aiClient = client
			if m.meta != nil {
				if count, err2 := m.meta.IndexedCount(context.Background()); err2 == nil && count == 0 {
					welcomeMsg = "Welcome to Gorae AI  (model: " + client.Model() + ")\n\n" +
						"Tip: your library is not indexed yet — run :index first for\n" +
						"document-aware answers. Ask anything to get started."
				}
			}
		}
	}

	// Single-line input used only by the /load find box.
	inp := textinput.New()
	inp.Placeholder = goraeFindPlaceholder
	inp.CharLimit = 4096
	inp.Cursor.SetMode(cursor.CursorStatic)
	inp.Blur()
	m.aiInput = inp

	// Multi-line input box for composing chat messages.
	ta := newGoraeTextarea()
	if taW := m.width - 4; taW > 0 {
		ta.SetWidth(taW)
	}
	ta.Focus()
	m.aiTextarea = ta

	m.aiStreaming = false
	m.aiStreamBuf = ""
	m.aiMessages = nil
	m.aiSources = nil
	// Ensure any active kitty/iTerm2 image is cleared when entering chat.
	m.previewGraphic = ""
	m.previewGraphicClear = true
	m.aiChatScroll = 0
	m.aiFollowBottom = true
	m.aiFindMode = false
	m.aiSearchSelecting = false
	m.aiSearchResults = nil
	m.aiLiveQuery = ""
	m.aiNormalMode = false
	m.aiMsgCursor = 0
	m.aiMsgMarks = nil
	m.aiLastNavKey = ""
	m.aiNormalHintShown = false
	m.aiUnknownHintShown = false
	m.aiHistoryCursor = -1
	m.aiHistoryDraft = ""

	// Load user-defined skills.
	if skills, err := loadSkills(m.skillsDir); err == nil {
		m.aiUserSkills = skills
	}

	// Load persisted messages and focused file when resuming a saved session.
	if m.aiSessionID > 0 && m.meta != nil {
		// Restore focused papers (stored newline-joined for multi-paper sessions).
		if sessions, err := m.meta.ListSessions(context.Background()); err == nil {
			for _, s := range sessions {
				if s.ID == m.aiSessionID && s.FocusedFile != "" {
					m.aiFocusedFiles = parseFocusedPaths(s.FocusedFile)
					break
				}
			}
		}
		if msgs, err := m.meta.LoadMessages(context.Background(), m.aiSessionID); err == nil && len(msgs) > 0 {
			for _, cm := range msgs {
				m.aiMessages = append(m.aiMessages, ai.Message{
					Role:     ai.Role(cm.Role),
					Content:  cm.Content,
					Thinking: cm.Thinking,
				})
			}
			welcomeMsg = fmt.Sprintf("Resumed session — %d message(s) loaded. Type /new for a fresh session.", len(msgs))
			m.aiFollowBottom = true
		}
	}

	if welcomeMsg != "" {
		m.appendAISystem(welcomeMsg)
	}

	modelName := "not configured"
	if m.aiClient != nil {
		modelName = m.aiClient.Model()
	}
	m.state = stateGorae
	m.updateGoraeStatus(modelName)
	return nil
}

// enterFindMode opens the vim-style fuzzy-find box that /load uses to pick a
// file to focus. The chat input becomes the search query and the results update
// live as the user types; an empty query lists everything indexed.
func (m *Model) enterFindMode(query string) tea.Cmd {
	if m.meta == nil {
		m.appendAISystem("Metadata store not available.")
		return nil
	}
	m.aiFindMode = true
	m.aiSearchSelecting = true
	m.aiSearchResults = nil
	m.aiSearchCursor = 0
	m.aiLiveQuery = query
	m.aiInput.SetValue(query)
	m.aiInput.CursorEnd()
	m.aiTextarea.Blur()
	m.aiInput.Focus()
	m.aiFollowBottom = true // stick to bottom so the find box stays visible
	return m.doLiveFind(query)
}

// exitFindMode closes the find box and returns focus to the chat message box.
func (m *Model) exitFindMode() {
	m.aiFindMode = false
	m.aiSearchSelecting = false
	m.aiSearchResults = nil
	m.aiSearchCursor = 0
	m.aiLiveQuery = ""
	m.aiInput.SetValue("")
	m.aiInput.Width = 0 // undo the width the modal set on the find input
	m.aiInput.Blur()
	m.aiTextarea.Focus()
}

// ── focused paper set ─────────────────────────────────────────────────────────

const (
	maxFocusedPapers     = 5    // cap on how many papers are held in context at once
	defaultMaxPaperChars = 8000 // per-paper injection cap when cfg.AI.MaxPaperChars is 0
)

// addFocusedPaper adds a paper to the context set (most recent last), de-duping
// and dropping the oldest once the cap is exceeded. Returns false if it was
// already present.
func (m *Model) addFocusedPaper(path string) bool {
	for _, p := range m.aiFocusedFiles {
		if p == path {
			return false
		}
	}
	m.aiFocusedFiles = append(m.aiFocusedFiles, path)
	if len(m.aiFocusedFiles) > maxFocusedPapers {
		m.aiFocusedFiles = m.aiFocusedFiles[len(m.aiFocusedFiles)-maxFocusedPapers:]
	}
	return true
}

// primaryFocusedPaper returns the most recently added paper, or "".
func (m *Model) primaryFocusedPaper() string {
	if len(m.aiFocusedFiles) == 0 {
		return ""
	}
	return m.aiFocusedFiles[len(m.aiFocusedFiles)-1]
}

// maxPaperChars is the per-paper context cap (config override or default).
func (m *Model) maxPaperChars() int {
	if m.cfg != nil && m.cfg.AI != nil && m.cfg.AI.MaxPaperChars > 0 {
		return m.cfg.AI.MaxPaperChars
	}
	return defaultMaxPaperChars
}

// persistFocusedPapers records the focus set on the active session. Multiple
// paths are stored newline-joined in the single focused_file column.
func (m *Model) persistFocusedPapers() {
	if m.aiSessionID > 0 && m.meta != nil {
		_ = m.meta.UpdateSessionFocusedFile(context.Background(), m.aiSessionID, strings.Join(m.aiFocusedFiles, "\n"))
	}
}

// paperRefCues reports whether a message plausibly refers to a specific paper
// the user wants pulled into context. It gates the (LLM-backed) resolver so we
// don't add latency to ordinary conversational turns.
func paperRefCues(text string) bool {
	lower := strings.ToLower(text)
	cues := []string{
		"paper", "papers", "article", "study", "studies", "publication",
		"compare", "comparison", "contrast", "relation", "relationship",
		"between", "versus", " vs ", " vs.", "author", "this work",
	}
	for _, c := range cues {
		if strings.Contains(lower, c) {
			return true
		}
	}
	return strings.Contains(text, "\"") // quoted title
}

// isGenericPaperRef reports whether a descriptor is only filler/pronoun words
// (e.g. "this paper", "the study") and so names no specific paper to search for.
func isGenericPaperRef(desc string) bool {
	generic := map[string]bool{
		"this": true, "that": true, "the": true, "a": true, "an": true, "paper": true,
		"papers": true, "work": true, "it": true, "article": true, "study": true,
		"above": true, "below": true, "current": true, "same": true, "one": true,
		"these": true, "those": true, "and": true, "of": true, "about": true, "on": true,
	}
	fields := strings.FieldsFunc(strings.ToLower(desc), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(fields) == 0 {
		return true
	}
	for _, f := range fields {
		if !generic[f] {
			return false
		}
	}
	return true
}

// extractPaperRefs asks the model for short search phrases naming specific papers
// the user wants to discuss, returning at most 3. It is only used in the no-tools
// fallback and fails soft (returns nil) so a slow or unhelpful model never blocks
// the answer for long.
func extractPaperRefs(ctx context.Context, userText string, client *ai.Client) []string {
	if client == nil {
		return nil
	}
	exCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	prompt := "From the user's message, extract up to 3 short search phrases that each identify a SPECIFIC paper " +
		"they want to pull from their library to discuss or compare. Use the user's own words (a topic, title " +
		"fragment, or author). If the message does not reference a specific paper to load, return an empty list.\n" +
		"Reply with JSON only: {\"papers\": [\"...\"]}\n" +
		"Message: \"" + userText + "\""

	ch := client.StreamChat(exCtx, []ai.Message{{Role: ai.RoleUser, Content: prompt}}, nil)
	var buf strings.Builder
	for tok := range ch {
		if tok.Err != nil {
			return nil
		}
		buf.WriteString(tok.Text)
		if tok.Done {
			break
		}
	}
	raw := buf.String()
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil
	}
	var parsed struct {
		Papers []string `json:"papers"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return nil
	}
	var out []string
	for _, p := range parsed.Papers {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
		if len(out) == 3 {
			break
		}
	}
	return out
}

// sliceContains reports whether v is present in s.
func sliceContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// prependUnique returns front followed by the items of rest not already in front.
func prependUnique(front, rest []string) []string {
	out := append([]string(nil), front...)
	seen := make(map[string]bool, len(front))
	for _, f := range front {
		seen[f] = true
	}
	for _, r := range rest {
		if !seen[r] {
			out = append(out, r)
			seen[r] = true
		}
	}
	return out
}

// parseFocusedPaths splits a stored focused_file value (possibly newline-joined)
// into individual paper paths.
func parseFocusedPaths(stored string) []string {
	var out []string
	for _, line := range strings.Split(stored, "\n") {
		if p := strings.TrimSpace(line); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (m *Model) goraeSelectCurrent() {
	if len(m.aiSearchResults) == 0 {
		return
	}
	chosen := m.aiSearchResults[m.aiSearchCursor]
	added := m.addFocusedPaper(chosen.Path)
	m.aiFindMode = false
	m.aiSearchSelecting = false
	m.aiSearchResults = nil
	m.aiLiveQuery = ""
	m.aiInput.SetValue("")
	m.aiInput.Width = 0 // undo the width the modal set on the find input
	m.aiInput.Blur()
	m.aiTextarea.Focus()
	if m.aiClient != nil {
		m.updateGoraeStatus(m.aiClient.Model())
	}
	if added {
		m.appendAISystem(fmt.Sprintf("Added to context: %s  (%d in context — /unfocus to clear)", chosen.Title, len(m.aiFocusedFiles)))
	} else {
		m.appendAISystem("Already in context: " + chosen.Title)
	}
	m.persistFocusedPapers()
}

func (m *Model) updateGoraeStatus(modelName string) {
	status := "Gorae AI  model:" + modelName + "  Esc:normal mode  Ctrl+C:exit"
	switch n := len(m.aiFocusedFiles); n {
	case 0:
	case 1:
		status += "  focus:" + filepath.Base(m.aiFocusedFiles[0])
	default:
		status += fmt.Sprintf("  focus:%d papers", n)
	}
	if m.aiShowThinking {
		status += "  [think:on]"
	}
	m.setPersistentStatus(status)
}

func (m *Model) exitGoraeChat() {
	m.state = stateNormal
	m.aiStreaming = false
	m.aiNormalMode = false
	m.aiMsgMarks = nil
	m.aiLastNavKey = ""
	m.aiInput.Blur()
	m.aiTextarea.Blur()
	m.setPersistentStatus("")
}

// ── update ────────────────────────────────────────────────────────────────────

func (m *Model) updateGoraeChat(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case aiCompactDoneMsg:
		m.aiCompacting = false
		if msg.err != nil {
			m.setStatus("Compact failed: " + msg.err.Error())
			return m, nil
		}
		n := len(m.aiMessages)
		keep := msg.kept
		if keep > n {
			keep = n
		}
		compacted := n - keep
		summaryMsg := ai.Message{
			Role:      ai.RoleAssistant,
			Content:   msg.summary,
			IsSummary: true,
		}
		m.aiMessages = append([]ai.Message{summaryMsg}, m.aiMessages[n-keep:]...)
		m.aiChatScroll = 0
		m.aiFollowBottom = true
		m.setStatus(fmt.Sprintf("Compacted %d messages → summary + last %d kept verbatim", compacted, keep))
		return m, nil

	case tea.KeyMsg:
		if m.aiCompacting {
			return m, nil
		}
		if m.aiStreaming {
			switch msg.String() {
			case "esc", "ctrl+c":
				if m.aiCancelFunc != nil {
					m.aiCancelFunc()
					m.aiCancelFunc = nil
				}
				m.aiStreaming = false
				m.flushStreamBuf()
			}
			return m, nil
		}

		key := msg.String()

		// Universal keys (work in both modes, regardless of overlays).
		switch key {
		case "ctrl+c":
			m.exitGoraeChat()
			return m, nil
		case "ctrl+t":
			m.aiShowThinking = !m.aiShowThinking
			if m.aiClient != nil {
				m.updateGoraeStatus(m.aiClient.Model())
			}
			return m, nil
		case "ctrl+p":
			m.scrollAIChatBy(-1)
			return m, nil
		case "ctrl+n":
			m.scrollAIChatBy(1)
			return m, nil
		case "pgup":
			m.scrollAIChatBy(-m.viewportHeight / 2)
			return m, nil
		case "pgdown":
			m.scrollAIChatBy(m.viewportHeight / 2)
			return m, nil
		}

		// Find-overlay intercepts navigation/select while active.
		if m.aiSearchSelecting {
			switch key {
			case "esc":
				if m.aiFindMode {
					m.exitFindMode()
				} else {
					m.aiSearchSelecting = false
				}
				return m, nil
			case "up":
				if m.aiSearchCursor > 0 {
					m.aiSearchCursor--
				}
				return m, nil
			case "down":
				if m.aiSearchCursor < len(m.aiSearchResults)-1 {
					m.aiSearchCursor++
				}
				return m, nil
			case "enter":
				m.goraeSelectCurrent()
				return m, nil
			}
			// Typing keys fall through to the input so the live filter updates.
		}

		if m.aiNormalMode {
			return m.handleGoraeNormalKey(key)
		}

		// Insert mode.
		switch key {
		case "esc":
			if len(m.aiMessages) == 0 {
				// No messages to navigate — preserve the old "Esc exits" shortcut.
				m.exitGoraeChat()
				return m, nil
			}
			m.enterAINormalMode()
			return m, nil
		case "up":
			// Recall history only when the cursor is on the very first visual row
			// (top of the first logical line); otherwise let the cursor move up.
			if m.aiTextarea.Line() == 0 && m.aiTextarea.LineInfo().RowOffset == 0 {
				m.goraeHistoryBack()
				return m, nil
			}
		case "down":
			// Recall history only when the cursor is on the very last visual row
			// (bottom of the last logical line); otherwise let the cursor move down.
			li := m.aiTextarea.LineInfo()
			if m.aiTextarea.Line() >= m.aiTextarea.LineCount()-1 && li.RowOffset >= li.Height-1 {
				m.goraeHistoryForward()
				return m, nil
			}
		case "tab":
			if cmd := m.goraeAutocomplete(); cmd != nil {
				return m, cmd
			}
			return m, nil
		case "enter":
			raw := strings.TrimSpace(m.aiTextarea.Value())
			if raw == "" {
				return m, nil
			}
			m.aiTextarea.Reset()
			if strings.HasPrefix(raw, "/") {
				return m, m.handleGoraeSlashCommand(raw)
			}
			return m, m.submitGoraeMessage(raw)
		}

	case aiTokenMsg:
		return m, m.handleAIToken(msg)

	case aiSourcesMsg:
		m.aiSources = msg.paths
		return m, nil

	case aiFocusResolvedMsg:
		var added []string
		for _, p := range msg.paths {
			if m.addFocusedPaper(p) {
				added = append(added, titleForPath(m.meta, context.Background(), p))
			}
		}
		if len(added) > 0 {
			m.persistFocusedPapers()
			if m.aiClient != nil {
				m.updateGoraeStatus(m.aiClient.Model())
			}
			m.appendAISystem("Pulled into context: " + strings.Join(added, ", ") + "  (use /load if I picked the wrong one)")
		}
		return m, nil

	case aiLiveFindMsg:
		// Discard stale results from a previous query
		if msg.query == m.aiLiveQuery {
			m.aiSearchResults = msg.results
			// Keep the box open while in find mode even if nothing matches, so
			// the user can keep editing the query instead of it snapping shut.
			m.aiSearchSelecting = m.aiFindMode || len(msg.results) > 0
			if m.aiSearchCursor >= len(msg.results) {
				m.aiSearchCursor = 0
			}
		}
		return m, nil

	case goraeSpinnerTickMsg:
		if m.aiStreaming || m.aiCompacting {
			m.aiSpinnerFrame = (m.aiSpinnerFrame + 1) % len(spinnerFrames)
			return m, goraeSpinnerTick()
		}
		return m, nil
	}

	// Route input to the /load find box (single-line) or the chat message box
	// (multi-line) depending on which is active.
	var inputCmd tea.Cmd
	if m.aiFindMode {
		m.aiInput, inputCmd = m.aiInput.Update(msg)
	} else if !m.aiNormalMode {
		m.aiTextarea, inputCmd = m.aiTextarea.Update(msg)
	}
	cmds = append(cmds, inputCmd)

	// While the /load find box owns the input, re-run the search whenever the
	// query changes so results filter live as the user types.
	if !m.aiStreaming && m.aiFindMode {
		query := strings.TrimSpace(m.aiInput.Value())
		if query != m.aiLiveQuery {
			m.aiLiveQuery = query
			cmds = append(cmds, m.doLiveFind(query))
		}
	}

	return m, tea.Batch(cmds...)
}

// ── slash commands ────────────────────────────────────────────────────────────

// goraeSlashCommands drives autocomplete and the on-screen hint overlay. The
// legacy "/find" command still works as a silent alias (see handleGoraeSlashCommand)
// but is intentionally absent here so new users see only "/load".
var goraeSlashCommands = []string{"/load", "/unfocus", "/summarize", "/clear", "/compact", "/export", "/sources", "/sessions", "/new", "/skills", "/help"}

func (m *Model) goraeAutocomplete() tea.Cmd {
	val := m.aiTextarea.Value()
	if !strings.HasPrefix(val, "/") {
		return nil
	}
	// Only complete when no space yet (completing the command name itself).
	if strings.ContainsRune(val, ' ') {
		return nil
	}
	lower := strings.ToLower(val)
	var matches []string
	for _, cmd := range goraeSlashCommands {
		if strings.HasPrefix(cmd, lower) {
			matches = append(matches, cmd)
		}
	}
	for _, sk := range m.aiUserSkills {
		skillCmd := "/" + sk.Name
		if strings.HasPrefix(skillCmd, lower) {
			matches = append(matches, skillCmd)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	// Find longest common prefix among matches.
	lcp := matches[0]
	for _, m := range matches[1:] {
		for !strings.HasPrefix(m, lcp) {
			lcp = lcp[:len(lcp)-1]
		}
	}
	if len(matches) == 1 {
		m.aiTextarea.SetValue(lcp + " ")
	} else {
		m.aiTextarea.SetValue(lcp)
	}
	m.aiTextarea.CursorEnd()

	// When Tab uniquely resolves to /sessions, open the list immediately.
	if len(matches) == 1 && lcp == "/sessions" {
		m.aiTextarea.SetValue("")
		return m.enterSessionList()
	}
	return nil
}

// resolveSlashCommand expands an unambiguous prefix to its full command name so
// that "/sum" or "/summ" resolve to "/summarize". An exact match always wins; an
// ambiguous prefix (e.g. "/s", which could be /select, /summarize, …) or an
// unknown one is returned unchanged so the caller can report it.
func (m *Model) resolveSlashCommand(cmd string) string {
	known := make([]string, 0, len(goraeSlashCommands)+len(m.aiUserSkills))
	known = append(known, goraeSlashCommands...)
	for _, sk := range m.aiUserSkills {
		known = append(known, "/"+sk.Name)
	}
	for _, k := range known {
		if k == cmd {
			return cmd
		}
	}
	match := ""
	for _, k := range known {
		if strings.HasPrefix(k, cmd) {
			if match != "" {
				return cmd // ambiguous — leave it to the caller to reject
			}
			match = k
		}
	}
	if match != "" {
		return match
	}
	return cmd
}

func (m *Model) handleGoraeSlashCommand(raw string) tea.Cmd {
	parts := strings.Fields(raw)
	cmd := m.resolveSlashCommand(strings.ToLower(parts[0]))
	switch cmd {
	case "/load", "/find": // /find is the legacy name; keep it working silently
		// Open the vim-style fuzzy-find box. Any text after the command is used
		// as the initial query so "/load foo" jumps straight to filtered results.
		query := strings.TrimSpace(strings.TrimPrefix(raw, parts[0]))
		return m.enterFindMode(query)

	case "/summarize":
		return m.startSummarize()

	case "/select", "/unfocus": // /select is the legacy name
		if len(m.aiFocusedFiles) == 0 {
			m.appendAISystem("No papers in context. Use /load or just mention a paper to add one.")
		} else {
			n := len(m.aiFocusedFiles)
			m.aiFocusedFiles = nil
			if m.aiClient != nil {
				m.updateGoraeStatus(m.aiClient.Model())
			}
			m.persistFocusedPapers()
			m.appendAISystem(fmt.Sprintf("Cleared %d paper(s) from context.", n))
		}
		return nil
	case "/clear":
		m.aiMessages = nil
		m.aiSources = nil
		m.aiChatScroll = 0
		m.aiFollowBottom = true
		m.aiMsgCursor = 0
		m.aiMsgMarks = nil
		if m.aiSessionID > 0 && m.meta != nil {
			_ = m.meta.ClearSessionMessages(context.Background(), m.aiSessionID)
		}
		m.setStatus("Chat history cleared")
	case "/compact":
		return m.compactChat()
	case "/sessions":
		return m.enterSessionList()
	case "/new":
		m.aiSessionID = 0
		m.aiMessages = nil
		m.aiSources = nil
		m.aiFocusedFiles = nil
		m.aiChatScroll = 0
		m.aiFollowBottom = true
		m.aiMsgCursor = 0
		m.aiMsgMarks = nil
		m.setStatus("New session started")
		if m.aiClient != nil {
			m.updateGoraeStatus(m.aiClient.Model())
		}
		return nil
	case "/export":
		return m.exportGoraeChat()
	case "/sources":
		if len(m.aiSources) == 0 {
			m.appendAISystem("No sources were cited in the last answer.")
		} else {
			var sb strings.Builder
			sb.WriteString("Sources used in last answer:\n")
			for _, p := range m.aiSources {
				sb.WriteString("  • " + filepath.Base(p) + "\n")
			}
			m.appendAISystem(strings.TrimRight(sb.String(), "\n"))
		}
	case "/skills":
		return m.handleSkillsCommand(raw, parts)
	case "/help":
		m.appendAISystem(goraeHelpText())
	default:
		// Check user-defined skills before giving up.
		skillName := strings.TrimPrefix(cmd, "/")
		for _, skill := range m.aiUserSkills {
			if skill.Name == skillName {
				extraArgs := ""
				if len(parts) > 1 {
					extraArgs = strings.Join(parts[1:], " ")
				}
				return m.invokeUserSkill(skill, extraArgs)
			}
		}
		m.appendAISystem(fmt.Sprintf("Unknown command: %s\nType /help for a list.", cmd))
	}
	return nil
}

func goraeHelpText() string {
	return strings.TrimSpace(`
Gorae AI slash commands:
  /load       — add a paper to the conversation (fuzzy-find box; ↑/↓/Enter). Or just mention a paper and it's pulled in.
  /unfocus    — clear all papers from the conversation
  /summarize  — summarize the focused paper and save to its note
  /clear      — clear chat history (also removes from saved session)
  /compact    — summarise old messages to free up context window
  /export     — save conversation to a markdown file
  /sources    — show documents cited in the last answer
  /sessions   — open session picker to load or manage past conversations
  /new        — start a new session (current session stays saved)
  /skills     — manage custom prompt templates (edit / list)
  /help       — this help text

Keyboard shortcuts (insert mode — typing into the prompt):
  Ctrl+T      — toggle thinking / reasoning display
  Ctrl+P/N    — scroll chat up / down
  PgUp/PgDn   — scroll half a page
  Mouse wheel — scroll chat up / down
  ↑/↓         — browse input history
  Tab         — autocomplete / command
  Esc         — switch to NORMAL mode (or exit if chat is empty)
  Ctrl+C      — exit chat

Normal mode (vim-style navigation across messages):
  i, a        — return to insert mode
  /           — insert mode prefilled with "/" for a slash command
  j / ↓       — next message
  k / ↑       — previous message
  l / →       — jump to next user message
  h / ←       — jump to previous user message
  gg          — first message
  G           — last message
  Space       — toggle mark on current message (multi-select)
  y           — yank current message — or all marks — to clipboard
  c           — clear all marks
  ?           — show this help
  q           — exit chat`)
}

// ── modal navigation (vim-style) ──────────────────────────────────────────────

const aiNavSeqTTL = 1200 * time.Millisecond

func (m *Model) enterAINormalMode() {
	if len(m.aiMessages) == 0 {
		return
	}
	m.aiNormalMode = true
	m.aiTextarea.Blur()
	// Snap cursor to the last message — most users land here from "I just sent
	// something, let me look at the answer".
	m.aiMsgCursor = len(m.aiMessages) - 1
	m.aiLastNavKey = ""
	m.ensureMessageCursorVisible()
	if m.aiClient != nil {
		m.updateGoraeStatus(m.aiClient.Model())
	}
	// First-time teaching: explain what just happened. Once per session so it
	// stays out of the way of users who already get it.
	if !m.aiNormalHintShown {
		m.aiNormalHintShown = true
		m.setStatus("NORMAL mode — j/k navigate · gg/G top/bottom · y copy · space mark · i to type · q to quit · ? help")
	}
}

func (m *Model) enterAIInsertMode() {
	m.aiNormalMode = false
	m.aiLastNavKey = ""
	m.aiTextarea.Focus()
	m.aiFollowBottom = true
	if m.aiClient != nil {
		m.updateGoraeStatus(m.aiClient.Model())
	}
}

// isUserMessage reports whether the given index points at a user message that
// h/l should jump to.
func (m *Model) isUserMessage(idx int) bool {
	if idx < 0 || idx >= len(m.aiMessages) {
		return false
	}
	return m.aiMessages[idx].Role == ai.RoleUser
}

func (m *Model) moveAIMsgCursor(delta int) {
	if len(m.aiMessages) == 0 {
		return
	}
	next := m.aiMsgCursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.aiMessages) {
		next = len(m.aiMessages) - 1
	}
	m.aiMsgCursor = next
	m.ensureMessageCursorVisible()
}

func (m *Model) jumpToUserMessage(direction int) {
	if len(m.aiMessages) == 0 {
		return
	}
	idx := m.aiMsgCursor + direction
	for idx >= 0 && idx < len(m.aiMessages) {
		if m.isUserMessage(idx) {
			m.aiMsgCursor = idx
			m.ensureMessageCursorVisible()
			return
		}
		idx += direction
	}
	// No further user message — clamp to start/end.
	if direction < 0 {
		m.aiMsgCursor = 0
	} else {
		m.aiMsgCursor = len(m.aiMessages) - 1
	}
	m.ensureMessageCursorVisible()
}

func (m *Model) toggleAIMsgMark() {
	if len(m.aiMessages) == 0 {
		return
	}
	if m.aiMsgMarks == nil {
		m.aiMsgMarks = map[int]bool{}
	}
	if m.aiMsgMarks[m.aiMsgCursor] {
		delete(m.aiMsgMarks, m.aiMsgCursor)
	} else {
		m.aiMsgMarks[m.aiMsgCursor] = true
	}
}

// yankAIMessages copies one-or-more chat messages to the system clipboard.
// When marks are present it yanks every marked message in chronological order;
// otherwise it yanks the message under the cursor. Returns a status string for
// the status bar.
func (m *Model) yankAIMessages() string {
	if len(m.aiMessages) == 0 {
		return "Nothing to yank"
	}
	var indices []int
	if len(m.aiMsgMarks) > 0 {
		for i := range m.aiMessages {
			if m.aiMsgMarks[i] {
				indices = append(indices, i)
			}
		}
	} else {
		indices = []int{m.aiMsgCursor}
	}

	var body string
	if len(indices) == 1 {
		// Single message: paste-ready, no role label.
		body = m.aiMessages[indices[0]].Content
	} else {
		var sb strings.Builder
		for i, idx := range indices {
			msg := m.aiMessages[idx]
			label := "Message"
			switch msg.Role {
			case ai.RoleUser:
				label = "You"
			case ai.RoleAssistant:
				label = "Gorae"
			case ai.RoleTool:
				label = "Tool (" + msg.Name + ")"
			}
			sb.WriteString("**" + label + ":** " + msg.Content)
			if i < len(indices)-1 {
				sb.WriteString("\n\n---\n\n")
			}
		}
		body = sb.String()
	}

	if err := clipboard.WriteAll(body); err != nil {
		return "Yank failed: " + err.Error()
	}
	if len(indices) == 1 {
		return "Yanked 1 message to clipboard"
	}
	return fmt.Sprintf("Yanked %d messages to clipboard", len(indices))
}

// handleGoraeNormalKey is the dispatcher for vim-style normal mode in chat.
// It must always return; falling through would hand the key to the input box.
func (m *Model) handleGoraeNormalKey(key string) (tea.Model, tea.Cmd) {
	// Two-key sequence resolution (currently only "gg").
	if m.aiLastNavKey == "g" && time.Since(m.aiLastNavAt) <= aiNavSeqTTL {
		m.aiLastNavKey = ""
		if key == "g" {
			m.aiMsgCursor = 0
			m.ensureMessageCursorVisible()
			return m, nil
		}
		// Anything else cancels the prefix and falls through.
	}

	switch key {
	case "i", "a":
		m.enterAIInsertMode()
		return m, nil
	case "esc":
		// Already normal — clear any pending prefix; do not exit chat.
		m.aiLastNavKey = ""
		return m, nil
	case "q":
		m.exitGoraeChat()
		return m, nil
	case "/":
		m.enterAIInsertMode()
		m.aiTextarea.SetValue("/")
		m.aiTextarea.CursorEnd()
		return m, nil
	case "?":
		m.appendAISystem(goraeHelpText())
		m.aiMsgCursor = len(m.aiMessages) - 1
		m.ensureMessageCursorVisible()
		return m, nil
	case "j", "down":
		m.moveAIMsgCursor(1)
		return m, nil
	case "k", "up":
		m.moveAIMsgCursor(-1)
		return m, nil
	case "l", "right":
		m.jumpToUserMessage(1)
		return m, nil
	case "h", "left":
		m.jumpToUserMessage(-1)
		return m, nil
	case "g":
		m.aiLastNavKey = "g"
		m.aiLastNavAt = time.Now()
		return m, nil
	case "G":
		m.aiMsgCursor = len(m.aiMessages) - 1
		m.ensureMessageCursorVisible()
		return m, nil
	case " ", "space":
		m.toggleAIMsgMark()
		return m, nil
	case "y":
		m.setStatus(m.yankAIMessages())
		return m, nil
	case "c":
		// Clear marks.
		if len(m.aiMsgMarks) > 0 {
			n := len(m.aiMsgMarks)
			m.aiMsgMarks = nil
			m.setStatus(fmt.Sprintf("Cleared %d mark(s)", n))
		}
		return m, nil
	}

	// Safety net: if the user typed a printable letter that has no normal-mode
	// binding (e.g. "t", "x", "e"), explain once where they are. Filters out
	// special keys like "enter"/"tab"/"ctrl+x" which have len > 1.
	if !m.aiUnknownHintShown && len(key) == 1 {
		m.aiUnknownHintShown = true
		m.setStatus("You're in NORMAL mode — press i to type, j/k to navigate, gg/G for top/bottom, q to quit, ? for help")
	}
	return m, nil
}

// ── scroll helpers ────────────────────────────────────────────────────────────

// aiChatMaxScroll returns the largest valid value for aiChatScroll given the
// current chat content and viewport. Mirrors the layout maths in renderGoraeView.
func (m *Model) aiChatMaxScroll() int {
	width := m.width
	height := m.windowHeight
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	overlayLines := 0
	if !m.aiStreaming {
		muted := m.styles.Preview.Body
		accent := m.styles.StatusValue
		bright := m.styles.Preview.Info
		overlayLines = len(m.goraeCommandHint(muted, accent, bright))
	}

	const statusRows, sepRows = 1, 1
	chatHeight := height - m.goraeInputRegionRows() - statusRows - sepRows - overlayLines
	if chatHeight < 3 {
		chatHeight = 3
	}

	lines, _ := m.buildChatLines(width)
	max := len(lines) - chatHeight
	if max < 0 {
		max = 0
	}
	return max
}

// ensureMessageCursorVisible adjusts aiChatScroll so the message at
// aiMsgCursor stays on-screen with a few lines of context on each side.
// In normal mode the cursor — not aiFollowBottom — is the source of scroll
// truth, so this function takes over scroll bookkeeping for the duration.
func (m *Model) ensureMessageCursorVisible() {
	if m.aiMsgCursor < 0 || m.aiMsgCursor >= len(m.aiMessages) {
		return
	}
	width := m.width
	height := m.windowHeight
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	overlayLines := 0
	if !m.aiStreaming {
		muted := m.styles.Preview.Body
		accent := m.styles.StatusValue
		bright := m.styles.Preview.Info
		overlayLines = len(m.goraeCommandHint(muted, accent, bright))
	}
	const statusRows, sepRows = 1, 1
	chatHeight := height - m.goraeInputRegionRows() - statusRows - sepRows - overlayLines
	if chatHeight < 3 {
		chatHeight = 3
	}

	lines, offsets := m.buildChatLines(width)
	if m.aiMsgCursor >= len(offsets) {
		return
	}
	cursorLine := offsets[m.aiMsgCursor]

	maxScroll := len(lines) - chatHeight
	if maxScroll < 0 {
		maxScroll = 0
	}

	// If we were follow-bottom, the renderer was pegging scroll to maxScroll
	// regardless of m.aiChatScroll. Snap the field to that effective value
	// before doing the cursor-visibility math, then disable follow-bottom so
	// the renderer respects our updates.
	if m.aiFollowBottom {
		m.aiChatScroll = maxScroll
		m.aiFollowBottom = false
	}

	// Keep the cursor at least `scrolloff` lines from each edge — that's what
	// makes the screen feel like it scrolls smoothly under the cursor as you
	// press j/k, instead of the cursor sticking to the boundary.
	const scrolloff = 3
	topEdge := m.aiChatScroll + scrolloff
	bottomEdge := m.aiChatScroll + chatHeight - scrolloff - 1
	switch {
	case cursorLine < topEdge:
		m.aiChatScroll = cursorLine - scrolloff
	case cursorLine > bottomEdge:
		m.aiChatScroll = cursorLine - chatHeight + scrolloff + 1
	}
	if m.aiChatScroll < 0 {
		m.aiChatScroll = 0
	}
	if m.aiChatScroll > maxScroll {
		m.aiChatScroll = maxScroll
	}
}

// scrollAIChatBy moves the chat by delta lines. Negative scrolls up (and detaches
// from the bottom); positive scrolls down (and re-attaches once the bottom is
// reached so new tokens keep auto-scrolling).
func (m *Model) scrollAIChatBy(delta int) {
	if delta == 0 {
		return
	}
	max := m.aiChatMaxScroll()
	if delta < 0 {
		if m.aiFollowBottom {
			m.aiChatScroll = max
			m.aiFollowBottom = false
		}
		m.aiChatScroll += delta
		if m.aiChatScroll < 0 {
			m.aiChatScroll = 0
		}
		return
	}
	if m.aiFollowBottom {
		return
	}
	m.aiChatScroll += delta
	if m.aiChatScroll >= max {
		m.aiChatScroll = max
		m.aiFollowBottom = true
	}
}

// ── summarize ────────────────────────────────────────────────────────────────

func (m *Model) startSummarize() tea.Cmd {
	if m.aiClient == nil {
		m.appendAISystem("No AI provider configured.")
		return nil
	}
	focused := m.primaryFocusedPaper()
	if focused == "" {
		m.appendAISystem("No paper in context. Use /load or mention a paper to add one first.")
		return nil
	}
	if m.meta == nil {
		m.appendAISystem("Metadata store not available.")
		return nil
	}
	body, err := m.meta.GetFileContent(context.Background(), focused)
	if err != nil || strings.TrimSpace(body) == "" {
		m.appendAISystem("No indexed content found for this file. Run :index first.")
		return nil
	}

	title := filepath.Base(focused)
	if md, err2 := m.meta.Get(context.Background(), focused); err2 == nil && md != nil && strings.TrimSpace(md.Title) != "" {
		title = md.Title
	}

	systemPrompt := "You are a precise academic summarizer. " +
		"Produce a concise but complete summary that captures all key contributions, " +
		"methods, results, and conclusions. Do not omit important technical details. " +
		"Use plain prose; avoid bullet points."

	userMsg := fmt.Sprintf("Summarize the following document titled %q:\n\n%s", title, body)

	m.aiSummarizeTarget = focused
	m.aiMessages = append(m.aiMessages, ai.Message{Role: ai.RoleUser, Content: "/summarize " + title})
	m.aiInputHistory = append(m.aiInputHistory, "/summarize")
	m.aiHistoryCursor = -1
	m.aiFollowBottom = true
	m.aiStreaming = true
	m.aiRawBuf = ""
	m.aiStreamBuf = ""
	m.aiThinkBuf = ""
	m.aiSpinnerFrame = 0

	ctx, cancel := context.WithCancel(context.Background())
	m.aiCancelFunc = cancel

	client := m.aiClient
	msgs := []ai.Message{
		{Role: ai.RoleSystem, Content: systemPrompt},
		{Role: ai.RoleUser, Content: userMsg},
	}

	return tea.Batch(goraeSpinnerTick(), func() tea.Msg {
		ch := client.StreamChat(ctx, msgs, nil)
		return pumpTokenCmd(ch)()
	})
}

func (m *Model) saveSummaryToNote(filePath, summary string) {
	notePath, err := m.noteFilePath(filePath)
	if err != nil {
		m.setStatus("Summary generated but note path unavailable: " + err.Error())
		return
	}

	// Read existing note
	existing := ""
	if data, err2 := os.ReadFile(notePath); err2 == nil {
		existing = string(data)
	}

	header := "## Gorae Paper Summary\n*" + time.Now().Format("2006-01-02") + "*"
	block := "\n\n" + header + "\n\n" + strings.TrimSpace(summary) + "\n"

	var content string
	const marker = "## Gorae Paper Summary"
	if idx := strings.Index(existing, marker); idx >= 0 {
		// Replace existing summary section
		end := strings.Index(existing[idx:], "\n\n## ")
		if end == -1 {
			content = existing[:idx] + strings.TrimLeft(block, "\n")
		} else {
			content = existing[:idx] + strings.TrimLeft(block, "\n") + "\n\n" + existing[idx+end+4:]
		}
	} else {
		content = strings.TrimRight(existing, "\n") + block
	}

	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err == nil {
		if err2 := os.WriteFile(notePath, []byte(content), 0o644); err2 == nil {
			m.setStatus("Summary saved to note for " + filepath.Base(filePath))
			return
		}
	}
	m.setStatus("Summary generated but failed to write note.")
}

// ── input history ─────────────────────────────────────────────────────────────

func (m *Model) goraeHistoryBack() {
	if len(m.aiInputHistory) == 0 {
		return
	}
	if m.aiHistoryCursor == -1 {
		m.aiHistoryDraft = m.aiTextarea.Value()
		m.aiHistoryCursor = len(m.aiInputHistory) - 1
	} else if m.aiHistoryCursor > 0 {
		m.aiHistoryCursor--
	}
	m.aiTextarea.SetValue(m.aiInputHistory[m.aiHistoryCursor])
	m.aiTextarea.CursorEnd()
}

func (m *Model) goraeHistoryForward() {
	if m.aiHistoryCursor == -1 {
		return
	}
	m.aiHistoryCursor++
	if m.aiHistoryCursor >= len(m.aiInputHistory) {
		m.aiHistoryCursor = -1
		m.aiTextarea.SetValue(m.aiHistoryDraft)
	} else {
		m.aiTextarea.SetValue(m.aiInputHistory[m.aiHistoryCursor])
	}
	m.aiTextarea.CursorEnd()
}

// ── message submission + RAG ──────────────────────────────────────────────────

func (m *Model) submitGoraeMessage(userText string) tea.Cmd {
	if m.aiClient == nil {
		m.appendAISystem("No AI provider configured. Add an \"ai\" block to config.json and restart.")
		return nil
	}
	m.aiMessages = append(m.aiMessages, ai.Message{Role: ai.RoleUser, Content: userText})
	m.aiInputHistory = append(m.aiInputHistory, userText)

	// Persist to session — create one lazily on the first message.
	if m.meta != nil {
		if m.aiSessionID == 0 {
			title := userText
			runes := []rune(title)
			if len(runes) > 60 {
				title = string(runes[:59]) + "…"
			}
			if id, err := m.meta.CreateSession(context.Background(), title); err == nil {
				m.aiSessionID = id
			}
		}
		if m.aiSessionID > 0 {
			_ = m.meta.SaveMessage(context.Background(), m.aiSessionID, string(ai.RoleUser), userText, "")
		}
	}
	m.aiHistoryCursor = -1
	m.aiHistoryDraft = ""
	m.aiFollowBottom = true // stick to bottom while answer streams in
	m.aiStreaming = true
	m.aiToolHops = 0
	m.aiRawBuf = ""
	m.aiStreamBuf = ""
	m.aiThinkBuf = ""
	m.aiSpinnerFrame = 0

	ctx, cancel := context.WithCancel(context.Background())
	m.aiCancelFunc = cancel

	client := m.aiClient
	store := m.meta
	cfg := m.cfg
	tools := m.activeToolSpecs()
	history := make([]ai.Message, len(m.aiMessages))
	copy(history, m.aiMessages)

	focusedFiles := append([]string(nil), m.aiFocusedFiles...)
	maxChars := m.maxPaperChars()

	return tea.Batch(goraeSpinnerTick(), func() tea.Msg {

		var sources []string
		var docContext string
		plan := planRetrieval(ctx, userText, cfg, client)
		if plan.local || plan.web {
			sources, docContext = goraeRetrieveContext(ctx, store, cfg, client, userText, plan)
		}

		// Prepend every focused paper as primary context, each capped so a few
		// full papers don't blow the model's context window.
		if len(focusedFiles) > 0 && store != nil {
			var blocks []string
			var focusSources []string
			for _, fp := range focusedFiles {
				body, err := store.GetFileContent(ctx, fp)
				if err != nil || strings.TrimSpace(body) == "" {
					continue
				}
				if r := []rune(body); len(r) > maxChars {
					body = string(r[:maxChars]) + "\n…[truncated]"
				}
				blocks = append(blocks, fmt.Sprintf("[Paper in focus: %s]\n%s", titleForPath(store, ctx, fp), body))
				focusSources = append(focusSources, fp)
			}
			if len(blocks) > 0 {
				joined := strings.Join(blocks, "\n\n")
				if docContext != "" {
					docContext = joined + "\n\n" + docContext
				} else {
					docContext = joined
				}
				sources = prependUnique(focusSources, sources)
			}
		}

		// No-tools fallback: when tool calling is off, resolve papers the user
		// referenced in natural language and inject them as whole-paper context.
		// (When tools are on, the model does this itself via find_papers/open_paper.)
		var resolved []string
		if len(tools) == 0 && store != nil && paperRefCues(userText) {
			for _, desc := range extractPaperRefs(ctx, userText, client) {
				if isGenericPaperRef(desc) {
					continue // "this paper" etc. refers to something already in context
				}
				hits := searchPapers(ctx, store, desc, 1)
				if len(hits) == 0 {
					continue
				}
				path := hits[0].Path
				if sliceContains(focusedFiles, path) || sliceContains(resolved, path) {
					continue
				}
				body, err := store.GetFileContent(ctx, path)
				if err != nil || strings.TrimSpace(body) == "" {
					continue
				}
				if r := []rune(body); len(r) > maxChars {
					body = string(r[:maxChars]) + "\n…[truncated]"
				}
				block := fmt.Sprintf("[Paper in focus: %s]\n%s", titleForPath(store, ctx, path), body)
				if docContext != "" {
					docContext = block + "\n\n" + docContext
				} else {
					docContext = block
				}
				sources = prependUnique([]string{path}, sources)
				resolved = append(resolved, path)
			}
		}

		systemMsg := ai.Message{
			Role:    ai.RoleSystem,
			Content: goraeSystemPrompt(cfg, docContext),
		}
		msgs := make([]ai.Message, 0, len(history)+1)
		msgs = append(msgs, systemMsg)
		msgs = append(msgs, history...)

		ch := client.StreamChat(ctx, msgs, tools)

		cmds := []tea.Cmd{func() tea.Msg { return aiSourcesMsg{paths: sources} }}
		if len(resolved) > 0 {
			rp := resolved
			cmds = append(cmds, func() tea.Msg { return aiFocusResolvedMsg{paths: rp} })
		}
		cmds = append(cmds, pumpTokenCmd(ch))
		return tea.Batch(cmds...)()
	})
}

// continueGoraeStream restarts streaming after a tool-call round-trip, with
// the updated m.aiMessages (which now contains the assistant tool-call request
// + tool replies). No RAG re-retrieval — the prior turn already gathered it.
func (m *Model) continueGoraeStream() tea.Cmd {
	if m.aiClient == nil {
		return nil
	}
	client := m.aiClient
	cfg := m.cfg
	tools := m.activeToolSpecs()
	history := make([]ai.Message, len(m.aiMessages))
	copy(history, m.aiMessages)

	ctx, cancel := context.WithCancel(context.Background())
	m.aiCancelFunc = cancel

	systemMsg := ai.Message{Role: ai.RoleSystem, Content: goraeSystemPrompt(cfg, "")}
	msgs := make([]ai.Message, 0, len(history)+1)
	msgs = append(msgs, systemMsg)
	msgs = append(msgs, history...)

	return func() tea.Msg {
		ch := client.StreamChat(ctx, msgs, tools)
		return pumpTokenCmd(ch)()
	}
}

func pumpTokenCmd(ch <-chan ai.StreamToken) tea.Cmd {
	return func() tea.Msg {
		tok, ok := <-ch
		if !ok {
			return aiTokenMsg{done: true}
		}
		return aiTokenMsg{
			text:      tok.Text,
			toolCalls: tok.ToolCalls,
			done:      tok.Done,
			err:       tok.Err,
			ch:        ch,
		}
	}
}

// ── streaming token handler ───────────────────────────────────────────────────

func (m *Model) handleAIToken(tok aiTokenMsg) tea.Cmd {
	if tok.err != nil {
		m.aiStreaming = false
		m.aiCancelFunc = nil
		m.flushStreamBuf()
		if !isContextCancelled(tok.err) {
			m.appendAISystem("Error: " + tok.err.Error())
		}
		return nil
	}
	if tok.done {
		if len(tok.toolCalls) > 0 {
			return m.handleToolCalls(tok.toolCalls)
		}
		m.aiStreaming = false
		m.aiCancelFunc = nil
		m.flushStreamBuf()
		if len(m.aiMessages) >= autoCompactThreshold && !m.aiCompacting {
			return m.compactChat()
		}
		return nil
	}
	m.aiRawBuf += tok.text
	m.aiStreamBuf, m.aiThinkBuf = parseThinkBlocks(m.aiRawBuf)
	return pumpTokenCmd(tok.ch)
}

// maxToolHopsPerTurn caps the number of tool-call iterations within a single
// user turn so a misbehaving model can't trap us in an infinite request loop.
const maxToolHopsPerTurn = 5

// handleToolCalls is invoked when the model finished with finish_reason=tool_calls.
// It records the request + each result in m.aiMessages (in-memory only — these
// rounds are deliberately not persisted to the session DB) and kicks off a
// follow-up stream so the model can see the results and reply.
func (m *Model) handleToolCalls(calls []ai.ToolCall) tea.Cmd {
	m.aiToolHops++
	if m.aiToolHops > maxToolHopsPerTurn {
		// Bail out: drop the partial tool exchange, surface an error, end the turn.
		m.aiStreaming = false
		m.aiCancelFunc = nil
		m.aiRawBuf = ""
		m.aiStreamBuf = ""
		m.aiThinkBuf = ""
		m.appendAISystem(fmt.Sprintf("Tool loop aborted: model exceeded %d tool-call iterations.", maxToolHopsPerTurn))
		return nil
	}

	// Capture any preface text the model produced alongside the tool call.
	display, _ := parseThinkBlocks(m.aiRawBuf)
	m.aiMessages = append(m.aiMessages, ai.Message{
		Role:      ai.RoleAssistant,
		Content:   strings.TrimSpace(display),
		ToolCalls: calls,
	})
	for _, call := range calls {
		result := m.runToolCall(call)
		m.aiMessages = append(m.aiMessages, ai.Message{
			Role:       ai.RoleTool,
			Content:    result,
			ToolCallID: call.ID,
			Name:       call.Func.Name,
		})
	}
	m.aiRawBuf = ""
	m.aiStreamBuf = ""
	m.aiThinkBuf = ""
	// Stay in streaming state so the spinner keeps ticking; continueGoraeStream
	// re-arms m.aiCancelFunc.
	m.aiFollowBottom = true
	return m.continueGoraeStream()
}

func (m *Model) flushStreamBuf() {
	if m.aiRawBuf == "" {
		return
	}
	display, thinking := parseThinkBlocks(m.aiRawBuf)
	m.aiMessages = append(m.aiMessages, ai.Message{
		Role:     ai.RoleAssistant,
		Content:  display,
		Thinking: thinking,
	})
	if m.aiSessionID > 0 && m.meta != nil {
		_ = m.meta.SaveMessage(context.Background(), m.aiSessionID, string(ai.RoleAssistant), display, thinking)
	}
	m.aiRawBuf = ""
	m.aiStreamBuf = ""
	m.aiThinkBuf = ""
	if m.aiSummarizeTarget != "" {
		m.saveSummaryToNote(m.aiSummarizeTarget, display)
		m.aiSummarizeTarget = ""
	}
}

// ── /compact ──────────────────────────────────────────────────────────────────

func (m *Model) compactChat() tea.Cmd {
	if m.aiClient == nil {
		m.appendAISystem("No AI provider configured.")
		return nil
	}
	n := len(m.aiMessages)
	const keepLast = 6
	toSummarise := n - keepLast
	if toSummarise < 4 {
		m.appendAISystem(fmt.Sprintf(
			"Not enough history to compact — need at least %d messages, have %d.", keepLast+4, n))
		return nil
	}

	var sb strings.Builder
	for _, msg := range m.aiMessages[:toSummarise] {
		switch msg.Role {
		case ai.RoleUser:
			sb.WriteString("User: ")
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")
		case ai.RoleAssistant:
			if msg.IsSummary {
				sb.WriteString("[Earlier summary]: ")
			} else {
				sb.WriteString("Assistant: ")
			}
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")
		}
	}

	client := m.aiClient
	sessionID := m.aiSessionID
	store := m.meta

	m.aiCompacting = true
	m.aiSpinnerFrame = 0

	return tea.Batch(goraeSpinnerTick(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		prompt := "Summarise the following conversation excerpt concisely.\n" +
			"Preserve: key questions, answers given, document references, and any conclusions.\n" +
			"Write in third-person past tense. This summary will replace the original messages " +
			"as context for an ongoing conversation — be complete but brief.\n\n" +
			"Conversation:\n" + sb.String()

		ch := client.StreamChat(ctx, []ai.Message{{Role: ai.RoleUser, Content: prompt}}, nil)
		var buf strings.Builder
		for tok := range ch {
			if tok.Err != nil {
				return aiCompactDoneMsg{err: tok.Err}
			}
			buf.WriteString(tok.Text)
			if tok.Done {
				break
			}
		}

		summary := strings.TrimSpace(buf.String())
		if summary == "" {
			return aiCompactDoneMsg{err: fmt.Errorf("model returned an empty summary")}
		}
		if sessionID > 0 && store != nil {
			_ = store.CompactSession(context.Background(), sessionID, summary, keepLast)
		}
		return aiCompactDoneMsg{summary: summary, kept: keepLast}
	})
}

// parseThinkBlocks splits raw streamed text into display content (outside
// <think>…</think> blocks) and thinking content (inside those blocks).
func parseThinkBlocks(raw string) (display, thinking string) {
	var dispBuf, thinkBuf strings.Builder
	rest := raw
	for {
		openIdx := strings.Index(rest, "<think>")
		if openIdx < 0 {
			dispBuf.WriteString(rest)
			break
		}
		dispBuf.WriteString(rest[:openIdx])
		rest = rest[openIdx+len("<think>"):]
		closeIdx := strings.Index(rest, "</think>")
		if closeIdx < 0 {
			// Still streaming inside a think block
			thinkBuf.WriteString(rest)
			break
		}
		if thinkBuf.Len() > 0 {
			thinkBuf.WriteString("\n\n")
		}
		thinkBuf.WriteString(strings.TrimSpace(rest[:closeIdx]))
		rest = rest[closeIdx+len("</think>"):]
	}
	return strings.TrimLeft(dispBuf.String(), "\n"), thinkBuf.String()
}

func (m *Model) appendAISystem(text string) {
	m.aiMessages = append(m.aiMessages, ai.Message{
		Role:    ai.RoleAssistant,
		Content: text,
	})
}

// ── retrieval planning ────────────────────────────────────────────────────────

type retrievalPlan struct {
	local bool
	web   bool
}

// planRetrieval decides which retrieval paths to activate for a query.
// Phase 1 — rule-based: handles obvious conversational, local-only, and
// web-only signals cheaply with no extra API calls.
// Phase 2 — LLM classifier: fires only when web search is enabled and no
// strong rule signal was found. Uses a short 3-second timeout so a slow model
// never blocks the main response for long.
func planRetrieval(ctx context.Context, query string, cfg *config.Config, client *ai.Client) retrievalPlan {
	words := strings.Fields(query)
	if len(words) <= 3 {
		return retrievalPlan{}
	}

	lower := strings.ToLower(strings.TrimRight(query, "!?."))

	// Purely conversational — skip all retrieval.
	conversational := []string{
		"hi", "hello", "hey", "thanks", "thank you", "ok", "okay",
		"sure", "yes", "no", "nope", "yep", "got it", "i see",
		"sounds good", "great", "nice", "cool", "perfect",
		"bye", "goodbye", "see you", "good morning", "good evening",
		"good afternoon", "how are you", "what's up", "whats up",
	}
	for _, phrase := range conversational {
		if lower == phrase {
			return retrievalPlan{}
		}
	}

	webEnabled := cfg != nil && cfg.WebSearch != nil && cfg.WebSearch.Enabled

	// Strong local signals → local only, never web.
	localSignals := []string{
		"my notes", "my library", "i highlighted", "i read", "i saved",
		"my document", "in my papers", "my books", "i annotated",
		"in my library", "my collection",
	}
	for _, sig := range localSignals {
		if strings.Contains(lower, sig) {
			return retrievalPlan{local: true}
		}
	}

	// Strong web signals → local + web (local context is still useful).
	if webEnabled {
		webSignals := []string{
			"latest", "recent", "current", "today", "news", "breaking",
			"just announced", "as of", "right now", "this week", "this month",
			"this year", "new research", "new study", "new paper",
		}
		for _, sig := range webSignals {
			if strings.Contains(lower, sig) {
				return retrievalPlan{local: true, web: true}
			}
		}

		// Ambiguous and substantial — ask the LLM to classify.
		if client != nil && len(words) > 5 {
			if plan, ok := llmClassifyRetrieval(ctx, query, client); ok {
				// Always keep local=true if the query is substantial.
				plan.local = true
				return plan
			}
		}
	}

	// Default: local retrieval only.
	return retrievalPlan{local: true}
}

// llmClassifyRetrieval sends a tiny classification prompt to the LLM and
// parses a JSON {local, web} response. It uses a 3-second timeout so a slow
// model never stalls the main answer for long.
func llmClassifyRetrieval(ctx context.Context, query string, client *ai.Client) (retrievalPlan, bool) {
	classCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	prompt := "You are a routing assistant. Classify this user query.\n" +
		"Reply with JSON only, no explanation: {\"local\": bool, \"web\": bool}\n" +
		"  local = true  → query is about the user's personal documents / saved library\n" +
		"  web   = true  → query needs current / real-time information from the internet\n" +
		"Query: \"" + query + "\""

	ch := client.StreamChat(classCtx, []ai.Message{{Role: ai.RoleUser, Content: prompt}}, nil)
	var buf strings.Builder
	for tok := range ch {
		if tok.Err != nil {
			return retrievalPlan{}, false
		}
		buf.WriteString(tok.Text)
		if tok.Done {
			break
		}
	}

	raw := buf.String()
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return retrievalPlan{}, false
	}
	var result struct {
		Local bool `json:"local"`
		Web   bool `json:"web"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &result); err != nil {
		return retrievalPlan{}, false
	}
	return retrievalPlan{local: result.Local, web: result.Web}, true
}

// ── RAG retrieval ─────────────────────────────────────────────────────────────

func goraeRetrieveContext(ctx context.Context, store *meta.Store, cfg *config.Config, client *ai.Client, query string, plan retrievalPlan) ([]string, string) {
	var sources []string
	var sb strings.Builder

	// Local retrieval (FTS or vector).
	if plan.local && store != nil {
		topK := 3
		if cfg != nil && cfg.AI != nil && cfg.AI.TopK > 0 {
			topK = cfg.AI.TopK
		}

		var results []meta.FTSMatch
		if cfg != nil && cfg.AI != nil && cfg.AI.VectorSearch && client != nil {
			embModel := strings.TrimSpace(cfg.AI.EmbeddingModel)
			if embModel == "" {
				embModel = "nomic-embed-text"
			}
			if vec, err := client.GetEmbedding(ctx, embModel, query); err == nil {
				// On error, results stays empty and we fall back to FTS below.
				results, _ = store.SearchSemantic(ctx, vec, topK)
			}
		}
		if len(results) == 0 {
			// Retrieval is best-effort: a failure here just means the answer is
			// ungrounded rather than fatal, so the error is intentionally dropped.
			results, _ = store.SearchFTS(ctx, query, topK)
		}

		seen := map[string]bool{}
		for _, r := range results {
			if !seen[r.Path] {
				seen[r.Path] = true
				sources = append(sources, r.Path)
			}
			sb.WriteString(fmt.Sprintf("[Local: %s]\n%s\n\n", filepath.Base(r.Path), strings.TrimSpace(r.Snippet)))
		}
	}

	// Web retrieval.
	if plan.web && cfg != nil && cfg.WebSearch != nil && cfg.WebSearch.Enabled {
		limit := cfg.WebSearch.Results
		if limit <= 0 {
			limit = 5
		}
		ws, err := search.NewWebSearcher(cfg.WebSearch.Provider, cfg.WebSearch.APIKey)
		if err == nil {
			webResults, err := ws.Search(ctx, query, limit)
			if err == nil {
				for _, r := range webResults {
					sources = append(sources, r.URL)
					snippet := strings.TrimSpace(r.Snippet)
					if len(snippet) > 500 {
						snippet = snippet[:500] + "…"
					}
					sb.WriteString(fmt.Sprintf("[Web: %s]\n%s\n\n", r.Title, snippet))
				}
			}
		}
	}

	if sb.Len() == 0 {
		return nil, ""
	}
	return sources, strings.TrimRight(sb.String(), "\n")
}

func goraeSystemPrompt(cfg *config.Config, docContext string) string {
	base := "You are Gorae, a helpful research and knowledge-base assistant."
	if cfg != nil && cfg.AI != nil && strings.TrimSpace(cfg.AI.SystemPrompt) != "" {
		base = strings.TrimSpace(cfg.AI.SystemPrompt)
	}
	var parts []string
	parts = append(parts, base)
	if cfg != nil && cfg.AI != nil && cfg.AI.EnableTools {
		parts = append(parts,
			"\nYou can search the user's paper library with the find_papers tool and pull a paper's full text "+
				"into the conversation with the open_paper tool. When the user refers to a paper by description "+
				"(\"the paper about X\", \"compare this with that\"), call find_papers to locate it, then call "+
				"open_paper with the exact path for each paper you need before answering. To compare or relate "+
				"papers, open each one. Papers shown as 'Paper in focus' below are already loaded — do not re-open them.")
	}
	if docContext != "" {
		parts = append(parts,
			"\nRelevant excerpts are provided below. [Local] entries are from the user's personal library; [Web] entries are live web results.",
			"Answer using these excerpts when relevant.",
			"If the answer is not in the excerpts, answer from general knowledge and say so.\n",
			docContext,
		)
	} else {
		parts = append(parts, "\nNo relevant documents were found for this query. Answer from general knowledge.")
	}
	return strings.Join(parts, "\n")
}

// ── export ────────────────────────────────────────────────────────────────────

func (m *Model) exportGoraeChat() tea.Cmd {
	path, err := m.exportGoraeChatTo("")
	if err != nil {
		m.setStatus(err.Error())
		return nil
	}
	m.setStatus("Exported to " + path)
	return nil
}

// exportGoraeChatTo writes the current conversation as a markdown transcript
// and returns the absolute path. filenameHint, if non-empty, overrides the
// auto-generated slug (extension is appended if missing).
func (m *Model) exportGoraeChatTo(filenameHint string) (string, error) {
	if len(m.aiMessages) == 0 {
		return "", fmt.Errorf("nothing to export")
	}
	var sb strings.Builder
	sb.WriteString("# Gorae Chat Export\n\n")
	for _, msg := range m.aiMessages {
		// Tool-call request/result rounds are not part of the user-facing
		// transcript — skip them so the export reads naturally.
		if len(msg.ToolCalls) > 0 || msg.Role == ai.RoleTool {
			continue
		}
		switch msg.Role {
		case ai.RoleUser:
			sb.WriteString("**You:** " + msg.Content + "\n\n")
		case ai.RoleAssistant:
			sb.WriteString("**Gorae:** " + msg.Content + "\n\n")
		}
	}
	if len(m.aiSources) > 0 {
		sb.WriteString("---\n**Sources**\n")
		for _, p := range m.aiSources {
			sb.WriteString("- " + filepath.Base(p) + "\n")
		}
	}
	return m.writeNoteMarkdown(filenameHint, sb.String())
}

// writeNoteMarkdown writes arbitrary markdown content to notes_dir under the
// given filename hint (sanitized; .md appended if missing; auto-named when
// empty) and returns the absolute path.
func (m *Model) writeNoteMarkdown(filenameHint, content string) (string, error) {
	notesDir := ""
	if m.cfg != nil {
		notesDir = strings.TrimSpace(m.cfg.NotesDir)
	}
	if notesDir == "" {
		return "", fmt.Errorf("no notes_dir configured — cannot save")
	}
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return "", fmt.Errorf("save failed: %w", err)
	}
	name := strings.TrimSpace(filenameHint)
	if name == "" {
		name = m.goraeExportFilename()
	} else {
		name = safeExportFilename(name)
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			name += ".md"
		}
	}
	path := filepath.Join(notesDir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("save failed: %w", err)
	}
	return path, nil
}

// safeExportFilename strips path separators and other unsafe characters from
// an LLM-supplied filename so a model can't escape the notes directory.
func safeExportFilename(name string) string {
	name = filepath.Base(name) // drop any directory components
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), ".-_")
	if out == "" {
		out = "gorae-chat-" + time.Now().Format("20060102-150405")
	}
	return out
}

// goraeExportFilename builds a unique export filename from the session title
// (when available) and the current timestamp.
// Format: gorae-chat-<slug>-YYYYMMDD-HHMMSS.md
func (m *Model) goraeExportFilename() string {
	slug := ""
	// Derive slug from the active session's title or from the first user message.
	title := ""
	if m.aiSessionID > 0 && m.meta != nil {
		if sessions, err := m.meta.ListSessions(context.Background()); err == nil {
			for _, s := range sessions {
				if s.ID == m.aiSessionID {
					title = s.Title
					break
				}
			}
		}
	}
	if title == "" {
		for _, msg := range m.aiMessages {
			if msg.Role == ai.RoleUser {
				title = msg.Content
				break
			}
		}
	}
	if title != "" {
		slug = slugify(title)
	}
	ts := time.Now().Format("20060102-150405")
	if slug == "" {
		return "gorae-chat-" + ts + ".md"
	}
	return "gorae-chat-" + slug + "-" + ts + ".md"
}

// slugify converts a string into a lowercase URL/filename-safe slug.
func slugify(s string) string {
	runes := []rune(strings.ToLower(strings.TrimSpace(s)))
	if len(runes) > 40 {
		runes = runes[:40]
	}
	var b strings.Builder
	prevDash := false
	for _, r := range runes {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func isContextCancelled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// ── user skills ───────────────────────────────────────────────────────────────

func (m *Model) handleSkillsCommand(raw string, parts []string) tea.Cmd {
	if len(parts) < 2 {
		m.appendAISystem(skillsHelpText())
		return nil
	}
	sub := strings.ToLower(parts[1])
	switch sub {
	case "edit":
		if len(parts) < 3 {
			if len(m.aiUserSkills) == 0 {
				m.appendAISystem("No skills yet.\n\nCreate a .md file in " + m.skillsDir)
				return nil
			}
			if len(m.aiUserSkills) == 1 {
				return m.openSkillEditor(m.aiUserSkills[0].Name)
			}
			var names []string
			for _, s := range m.aiUserSkills {
				names = append(names, s.Name)
			}
			m.appendAISystem("Which skill?\n\n  /skills edit <name>\n\nAvailable: " + strings.Join(names, ", "))
			return nil
		}
		name := strings.ToLower(parts[2])
		return m.openSkillEditor(name)

	case "list":
		if len(m.aiUserSkills) == 0 {
			m.appendAISystem("No skills yet.\n\nCreate a .md file in " + m.skillsDir)
			return nil
		}
		var sb strings.Builder
		sb.WriteString("Your skills:\n")
		for _, s := range m.aiUserSkills {
			desc := s.Description
			if desc == "" {
				desc = s.Prompt
			}
			if len([]rune(desc)) > 60 {
				desc = string([]rune(desc)[:59]) + "…"
			}
			sb.WriteString(fmt.Sprintf("\n  /%s\n    %s\n    %s\n", s.Name, desc, s.FilePath))
		}
		m.appendAISystem(strings.TrimRight(sb.String(), "\n"))
		return nil

	default:
		m.appendAISystem(skillsHelpText())
		return nil
	}
}

func (m *Model) openSkillEditor(name string) tea.Cmd {
	if err := os.MkdirAll(m.skillsDir, 0o755); err != nil {
		m.appendAISystem("Cannot create skills directory: " + err.Error())
		return nil
	}
	path := filepath.Join(m.skillsDir, name+".md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(skillTemplate), 0o644); err != nil {
			m.appendAISystem("Failed to create skill file: " + err.Error())
			return nil
		}
	}
	editor := m.configEditor()
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	m.setPersistentStatus(fmt.Sprintf("Editing skill %q with %s (save and exit to return)", name, editor))
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return skillEditFinishedMsg{err: err}
	})
}

func (m *Model) invokeUserSkill(skill UserSkill, extraArgs string) tea.Cmd {
	prompt := skill.Prompt
	prompt = strings.ReplaceAll(prompt, "{input}", extraArgs)

	needsFile := strings.Contains(prompt, "{focused_file}") || strings.Contains(prompt, "{title}")
	if needsFile {
		focused := m.primaryFocusedPaper()
		if focused == "" {
			m.appendAISystem(fmt.Sprintf("Skill %q needs a focused paper. Use /load to select one first.", skill.Name))
			return nil
		}
		if m.meta != nil {
			if body, err := m.meta.GetFileContent(context.Background(), focused); err == nil {
				prompt = strings.ReplaceAll(prompt, "{focused_file}", body)
			}
			title := filepath.Base(focused)
			if md, err := m.meta.Get(context.Background(), focused); err == nil && md != nil && strings.TrimSpace(md.Title) != "" {
				title = md.Title
			}
			prompt = strings.ReplaceAll(prompt, "{title}", title)
		}
	}

	return m.submitGoraeMessage(prompt)
}

func (m *Model) reloadUserSkills() {
	if skills, err := loadSkills(m.skillsDir); err == nil {
		m.aiUserSkills = skills
	}
}

func skillsHelpText() string {
	return strings.TrimSpace(`
Skills — custom prompt templates stored as .md files.

  /skills edit [name]   open a skill file in $EDITOR (creates if new)
  /skills list          show all skills

Placeholders:
  {input}         text typed after the skill name
  {focused_file}  full content of the focused file (use /load first)
  {title}         title of the focused file`)
}

func isValidSkillName(name string) bool {
	if name == "" || len(name) > 30 {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}
