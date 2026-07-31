package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Han8931/gorae/internal/ai"
)

// toolHandler runs an in-app action requested by the model. The string result
// is what gets sent back to the model as the tool's reply — make it short and
// informative ("Saved to /Users/.../chat.md", "Error: notes_dir not configured").
type toolHandler func(m *Model, rawArgs string) string

// goraeTool bundles the spec (sent to the model) with its executor.
type goraeTool struct {
	spec    ai.Tool
	handler toolHandler
}

// goraeTools is the registry of tools available to the model. Slice 1 starts
// with save_markdown; future entries (focus_file, find_files, summarize_focused)
// drop in here without changing the streaming loop.
var goraeTools = map[string]goraeTool{
	"find_papers": {
		spec: ai.Tool{
			Name: "find_papers",
			Description: "Search the user's paper library by title, filename, and full-text content. " +
				"Use this to locate a paper the user refers to by description (e.g. \"the paper about diffusion planning\") " +
				"before discussing it. Returns a numbered list of matches with their titles, exact paths, and a snippet. " +
				"Follow up with open_paper (using the exact path) to pull a paper into the conversation.",
			Schema: json.RawMessage(`{
              "type": "object",
              "properties": {
                "query": { "type": "string", "description": "Keywords, title fragment, author, or topic to search for." },
                "limit": { "type": "integer", "description": "Max results (default 8)." }
              },
              "required": ["query"],
              "additionalProperties": false
            }`),
		},
		handler: func(m *Model, rawArgs string) string {
			var p struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			if rawArgs != "" {
				if err := json.Unmarshal([]byte(rawArgs), &p); err != nil {
					return "Error: could not parse tool arguments: " + err.Error()
				}
			}
			if strings.TrimSpace(p.Query) == "" {
				return "Error: 'query' is required."
			}
			if m.meta == nil {
				return "Error: no library index available (run :index)."
			}
			limit := p.Limit
			if limit <= 0 || limit > 15 {
				limit = 8
			}
			results := searchPapers(context.Background(), m.meta, p.Query, limit)
			if len(results) == 0 {
				return fmt.Sprintf("No papers found matching %q.", p.Query)
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Found %d paper(s) for %q:\n", len(results), p.Query)
			for i, r := range results {
				fmt.Fprintf(&b, "%d. %s\n   path: %s\n", i+1, r.Title, r.Path)
				if strings.TrimSpace(r.Snippet) != "" {
					fmt.Fprintf(&b, "   match: %s\n", r.Snippet)
				}
			}
			b.WriteString("\nCall open_paper with the exact path to bring a paper into the conversation.")
			return b.String()
		},
	},

	"open_paper": {
		spec: ai.Tool{
			Name: "open_paper",
			Description: "Pull a paper's full text into the conversation as focused context and return it so you can " +
				"reason about it now. Prefer passing the exact 'path' from find_papers. To analyze a relationship " +
				"between papers, call open_paper once per paper. Papers already listed as 'Paper in focus' above are " +
				"already loaded — do not re-open them.",
			Schema: json.RawMessage(`{
              "type": "object",
              "properties": {
                "path": { "type": "string", "description": "Exact file path of the paper (preferred; get it from find_papers)." },
                "query": { "type": "string", "description": "Fallback: a description to resolve to the single best-matching paper when no path is known." }
              },
              "additionalProperties": false
            }`),
		},
		handler: func(m *Model, rawArgs string) string {
			var p struct {
				Path  string `json:"path"`
				Query string `json:"query"`
			}
			if rawArgs != "" {
				if err := json.Unmarshal([]byte(rawArgs), &p); err != nil {
					return "Error: could not parse tool arguments: " + err.Error()
				}
			}
			if m.meta == nil {
				return "Error: no library index available (run :index)."
			}
			ctx := context.Background()
			path := strings.TrimSpace(p.Path)
			if path == "" && strings.TrimSpace(p.Query) != "" {
				res := searchPapers(ctx, m.meta, p.Query, 1)
				if len(res) == 0 {
					return fmt.Sprintf("No paper found matching %q. Use find_papers to see options.", p.Query)
				}
				path = res[0].Path
			}
			if path == "" {
				return "Error: provide 'path' (preferred) or 'query'."
			}
			body, err := m.meta.GetFileContent(ctx, path)
			if err != nil || strings.TrimSpace(body) == "" {
				return fmt.Sprintf("Error: no indexed content for %q (run :index).", path)
			}
			m.addFocusedPaper(path)
			m.persistFocusedPapers()
			if m.aiClient != nil {
				m.updateGoraeStatus(m.aiClient.Model())
			}
			title := titleForPath(m.meta, ctx, path)
			if r := []rune(body); len(r) > m.maxPaperChars() {
				body = string(r[:m.maxPaperChars()]) + "\n…[truncated]"
			}
			return fmt.Sprintf("Opened %q — now in the conversation context. Full text:\n\n%s", title, body)
		},
	},

	"save_markdown": {
		spec: ai.Tool{
			Name: "save_markdown",
			Description: "Save markdown to a file in the user's notes directory. " +
				"STRONGLY PREFER passing the actual content the user wants kept (the summary, analysis, written-up answer, etc.) via the `content` parameter — this produces a clean standalone document. " +
				"Only omit `content` when the user explicitly asked to save the conversation/chat history; in that case the full transcript is written instead. " +
				"Always include a top-level heading in `content` when you provide it.",
			Schema: json.RawMessage(`{
              "type": "object",
              "properties": {
                "filename": {
                  "type": "string",
                  "description": "Optional filename (no path, no extension). If omitted a timestamped name is generated."
                },
                "content": {
                  "type": "string",
                  "description": "Markdown body to write verbatim. Pass the AI-generated artifact the user wants to keep (summary, analysis, notes). Include a top-level heading. If omitted, the full chat transcript is saved instead — only do that when the user explicitly asked to save the conversation."
                }
              },
              "additionalProperties": false
            }`),
		},
		handler: func(m *Model, rawArgs string) string {
			var p struct {
				Filename string `json:"filename"`
				Content  string `json:"content"`
			}
			if rawArgs != "" {
				if err := json.Unmarshal([]byte(rawArgs), &p); err != nil {
					return "Error: could not parse tool arguments: " + err.Error()
				}
			}
			if strings.TrimSpace(p.Content) != "" {
				path, err := m.writeNoteMarkdown(p.Filename, p.Content)
				if err != nil {
					return "Error: " + err.Error()
				}
				return "Saved to " + path
			}
			path, err := m.exportGoraeChatTo(p.Filename)
			if err != nil {
				return "Error: " + err.Error()
			}
			return "Saved to " + path
		},
	},
}

// activeToolSpecs returns the tool specs to send with the next StreamChat call,
// or nil when tool calling is disabled in config.
func (m *Model) activeToolSpecs() []ai.Tool {
	if m.cfg == nil || m.cfg.AI == nil || !m.cfg.AI.EnableTools {
		return nil
	}
	specs := make([]ai.Tool, 0, len(goraeTools))
	for _, t := range goraeTools {
		specs = append(specs, t.spec)
	}
	return specs
}

// runToolCall executes a single tool call and returns the reply content.
func (m *Model) runToolCall(call ai.ToolCall) string {
	t, ok := goraeTools[call.Func.Name]
	if !ok {
		return fmt.Sprintf("Error: unknown tool %q", call.Func.Name)
	}
	return t.handler(m, call.Func.Arguments)
}
