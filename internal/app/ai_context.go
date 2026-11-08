package app

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/Han8931/gorae/internal/ai"
	"github.com/Han8931/gorae/internal/meta"
)

// ── follow-up condensation ────────────────────────────────────────────────────
//
// Retrieval runs against one string, but conversation does not: "how does it
// compare to the baseline?" names its subject nowhere, so retrieving on it
// literally returns noise. Before retrieving, a message that leans on earlier
// turns is rewritten into a standalone query. The rewrite is used *only* for
// retrieval — the model still receives the user's own words.

// condenseTimeout bounds the rewrite. It runs before the answer can start
// streaming, so a slow model must degrade rather than stall the reply.
const condenseTimeout = 4 * time.Second

// maxCondensedChars rejects a rewrite that has started explaining itself
// instead of returning a query.
const maxCondensedChars = 240

// condenseHistoryTurns is how many trailing messages are shown to the rewriter.
// Anaphora almost always resolves against the last turn or two; more history
// mostly adds tokens and lets stale topics bleed into the query.
const condenseHistoryTurns = 4

// condenseHistoryChars truncates each quoted message — an assistant answer can
// be thousands of tokens, and only its opening is needed to fix a referent.
const condenseHistoryChars = 500

// followUpMarkers are words whose referent lives in an earlier turn. Their
// presence means the message cannot be retrieved on as written.
var followUpMarkers = map[string]bool{
	"it": true, "its": true, "this": true, "that": true, "these": true,
	"those": true, "they": true, "them": true, "their": true, "he": true,
	"she": true, "his": true, "her": true, "one": true, "ones": true,
	"former": true, "latter": true, "same": true, "above": true, "there": true,
	"such": true, "else": true, "instead": true, "too": true, "also": true,
	"again": true, "more": true, "another": true, "both": true, "either": true,
}

// followUpOpeners are sentence-initial continuations — "and the limitations?",
// "so why does that help?" — which signal a message that extends the last one.
var followUpOpeners = map[string]bool{
	"and": true, "but": true, "so": true, "or": true, "then": true,
	"why": true, "how": true, "what": true, "which": true, "who": true,
	"compare": true, "contrast": true, "elaborate": true, "continue": true,
	"explain": true, "expand": true, "summarize": true, "really": true,
}

// minStandaloneTerms is how many content words a message needs before it is
// treated as self-sufficient. Below this it is mostly function words — "why?",
// "explain more" — and says too little to retrieve against.
const minStandaloneTerms = 3

// openerStandaloneTerms is the higher bar applied when a message *starts* with
// a continuation word. "How"/"what"/"summarize" open follow-ups and ordinary
// standalone questions alike, so the opener only counts as evidence when the
// message is also thin on content: "and the limitations?" needs the preceding
// turn, "summarize the Chinchilla scaling laws paper" does not.
const openerStandaloneTerms = 4

// conversationalPhrases are complete messages that are social rather than
// substantive. They short-circuit both condensation and retrieval: "thanks" is
// short enough to look like a follow-up, but rewriting it into a research query
// would retrieve against a question the user never asked.
var conversationalPhrases = map[string]bool{
	"hi": true, "hello": true, "hey": true, "thanks": true, "thank you": true,
	"ok": true, "okay": true, "sure": true, "yes": true, "no": true,
	"nope": true, "yep": true, "got it": true, "i see": true,
	"sounds good": true, "great": true, "nice": true, "cool": true,
	"perfect": true, "bye": true, "goodbye": true, "see you": true,
	"good morning": true, "good evening": true, "good afternoon": true,
	"how are you": true, "what's up": true, "whats up": true,
	"thanks!": true, "ok thanks": true, "thanks, that helps": true,
	"that helps": true, "makes sense": true, "understood": true,
}

// isConversational reports whether a whole message is social filler.
func isConversational(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	lower = strings.TrimRight(lower, "!?. ")
	return conversationalPhrases[lower]
}

// needsCondensation reports whether a message depends on earlier turns to make
// sense as a retrieval query. It is deliberately cheap: a self-contained
// question ("what does FlashAttention say about memory?") returns false and
// skips the rewrite entirely, so the common case costs nothing.
func needsCondensation(text string) bool {
	if isConversational(text) {
		return false
	}
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(words) == 0 {
		return false
	}
	// A referential word anywhere means something must be resolved, however
	// much else the message says.
	for _, w := range words {
		if followUpMarkers[w] {
			return true
		}
	}
	terms := len(meta.ProseTerms(text))
	if followUpOpeners[words[0]] {
		return terms < openerStandaloneTerms
	}
	return terms < minStandaloneTerms
}

// condenseQuery rewrites userText into a standalone retrieval query using the
// tail of the conversation. prior must exclude userText itself.
//
// Every failure path returns a usable query rather than an error: no history,
// no client, a timeout, or a rewrite that looks like commentary all fall back
// to splicing in the recent topic locally, and the worst case is the original
// text. Retrieval quality degrades; the answer still happens.
func condenseQuery(ctx context.Context, prior []ai.Message, userText string, client *ai.Client) string {
	recent := recentDialogue(prior)
	if len(recent) == 0 {
		return userText
	}
	if client == nil {
		return localCondense(recent, userText)
	}

	condCtx, cancel := context.WithTimeout(ctx, condenseTimeout)
	defer cancel()

	var sb strings.Builder
	sb.WriteString("Rewrite the user's final message as a standalone search query for a " +
		"document library. Resolve pronouns and references using the conversation. " +
		"Keep the user's terminology — titles, author names, and technical terms " +
		"especially. Reply with the query text only: no quotes, no explanation, no label.\n\n" +
		"Conversation:\n")
	for _, msg := range recent {
		speaker := "User"
		if msg.Role == ai.RoleAssistant {
			speaker = "Assistant"
		}
		sb.WriteString(speaker + ": " + truncateRunes(msg.Content, condenseHistoryChars) + "\n")
	}
	sb.WriteString("\nFinal message: " + userText + "\nStandalone query:")

	ch := client.StreamChat(condCtx, []ai.Message{{Role: ai.RoleUser, Content: sb.String()}}, nil)
	var buf strings.Builder
	for tok := range ch {
		if tok.Err != nil {
			return localCondense(recent, userText)
		}
		buf.WriteString(tok.Text)
		if tok.Done {
			break
		}
	}

	if rewritten, ok := cleanCondensed(buf.String()); ok {
		return rewritten
	}
	return localCondense(recent, userText)
}

// cleanCondensed normalises a rewrite and reports whether it is usable as a
// query. A reasoning model may narrate before answering, and a small model may
// ignore the instruction altogether, so the result is checked rather than
// trusted.
func cleanCondensed(raw string) (string, bool) {
	if display, _ := parseThinkBlocks(raw); strings.TrimSpace(display) != "" {
		raw = display
	}
	out := strings.TrimSpace(raw)

	// Keep only the last non-empty line: models that preface the answer put the
	// query last, and a single-line reply is unaffected.
	if lines := strings.Split(out, "\n"); len(lines) > 1 {
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) != "" {
				out = strings.TrimSpace(lines[i])
				break
			}
		}
	}

	// Shed a leading label ("Standalone query: …") and wrapping quotes.
	if idx := strings.Index(out, ":"); idx >= 0 && idx < 24 {
		if head := strings.ToLower(out[:idx]); strings.Contains(head, "query") || strings.Contains(head, "search") {
			out = strings.TrimSpace(out[idx+1:])
		}
	}
	out = strings.Trim(out, "\"'` ")

	if out == "" || len([]rune(out)) > maxCondensedChars {
		return "", false
	}
	// A rewrite with no content words retrieves no better than the original.
	if len(meta.ProseTerms(out)) == 0 {
		return "", false
	}
	return out, true
}

// localCondense is the offline fallback: prepend the content terms of the most
// recent user message so the query at least carries the standing topic. Terms
// are used rather than the whole sentence to avoid dragging that turn's own
// function words and pronouns back in.
func localCondense(recent []ai.Message, userText string) string {
	for i := len(recent) - 1; i >= 0; i-- {
		if recent[i].Role != ai.RoleUser {
			continue
		}
		terms := meta.ProseTerms(recent[i].Content)
		if len(terms) == 0 {
			continue
		}
		if len(terms) > 8 {
			terms = terms[:8]
		}
		return strings.Join(terms, " ") + " " + userText
	}
	return userText
}

// recentDialogue returns the trailing user/assistant exchange, skipping system
// notices, compaction summaries, and tool plumbing — none of which help
// resolve a referent.
func recentDialogue(prior []ai.Message) []ai.Message {
	var out []ai.Message
	for i := len(prior) - 1; i >= 0 && len(out) < condenseHistoryTurns; i-- {
		msg := prior[i]
		if msg.Role != ai.RoleUser && msg.Role != ai.RoleAssistant {
			continue
		}
		if msg.IsSummary || len(msg.ToolCalls) > 0 || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		out = append(out, msg)
	}
	// Collected newest-first; flip so the transcript reads in order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func truncateRunes(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
