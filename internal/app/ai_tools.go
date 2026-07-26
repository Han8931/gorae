package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorae/internal/ai"
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
