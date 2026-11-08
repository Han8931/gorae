package meta

import (
	"strings"
	"unicode"
)

// maxProseTerms caps how many terms one question contributes to an FTS query,
// so a pasted paragraph can't build a pathological MATCH expression.
const maxProseTerms = 24

// proseStopwords are function words carrying no retrieval signal. They are
// dropped rather than searched: FTS5's implicit operator is AND, so leaving
// "how"/"does"/"the" in a question would require the document to contain them
// all and quietly reduce recall to nothing.
var proseStopwords = map[string]bool{
	"a": true, "about": true, "above": true, "after": true, "again": true,
	"all": true, "also": true, "am": true, "an": true, "and": true, "any": true,
	"are": true, "as": true, "at": true, "be": true, "because": true, "been": true,
	"before": true, "being": true, "below": true, "between": true, "both": true,
	"but": true, "by": true, "can": true, "could": true, "did": true, "do": true,
	"does": true, "doing": true, "done": true, "down": true, "during": true,
	"each": true, "else": true, "for": true, "from": true, "further": true,
	"get": true, "give": true, "had": true, "has": true, "have": true, "having": true,
	"he": true, "her": true, "here": true, "hers": true, "him": true, "his": true,
	"how": true, "i": true, "if": true, "in": true, "into": true, "is": true,
	"it": true, "its": true, "just": true, "let": true, "like": true, "may": true,
	"me": true, "might": true, "mine": true, "more": true, "most": true, "much": true,
	"must": true, "my": true, "no": true, "nor": true, "not": true, "now": true,
	"of": true, "off": true, "on": true, "once": true, "only": true, "or": true,
	"other": true, "ought": true, "our": true, "ours": true, "out": true, "over": true,
	"own": true, "please": true, "same": true, "say": true, "shall": true, "she": true,
	"should": true, "since": true, "so": true, "some": true, "such": true, "tell": true,
	"than": true, "that": true, "the": true, "their": true, "theirs": true, "them": true,
	"then": true, "there": true, "these": true, "they": true, "this": true, "those": true,
	"through": true, "to": true, "too": true, "under": true, "until": true, "up": true,
	"us": true, "use": true, "very": true, "was": true, "we": true, "were": true,
	"what": true, "when": true, "where": true, "which": true, "while": true, "who": true,
	"whom": true, "why": true, "will": true, "with": true, "would": true, "you": true,
	"your": true, "yours": true,
}

// ProseQuery turns a natural-language question into a safe FTS5 MATCH
// expression, or "" when the question carries no searchable terms.
//
// Two things make raw prose unusable as a MATCH expression. Punctuation is
// syntax — a trailing "?" or an apostrophe is a syntax error, not a character
// to match — so only letters and digits are kept. And the implicit operator
// between bare terms is AND, which for a sentence means "document contains
// every one of these words"; terms are therefore OR-ed and left to `ORDER BY
// rank` (bm25) to sort, so a document matching more of them still wins.
//
// Each term is double-quoted so that a word which happens to be an FTS5
// keyword ("near", "and") is treated as a string literal rather than an
// operator.
func ProseQuery(text string) string {
	terms := ProseTerms(text)
	if len(terms) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(terms))
	for _, t := range terms {
		quoted = append(quoted, `"`+t+`"`)
	}
	return strings.Join(quoted, " OR ")
}

// ProseTerms extracts the lowercased, de-duplicated content words of a
// natural-language string: punctuation and stopwords removed, capped at
// maxProseTerms. It is the shared notion of "what is searchable about this
// sentence" — callers also use the term count to judge whether a message says
// enough to stand on its own.
func ProseTerms(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	var terms []string
	seen := map[string]bool{}
	for _, f := range fields {
		term := strings.ToLower(f)
		if proseStopwords[term] || seen[term] {
			continue
		}
		// Single characters are almost always list markers or stray initials.
		if len([]rune(term)) < 2 {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
		if len(terms) == maxProseTerms {
			break
		}
	}
	return terms
}
