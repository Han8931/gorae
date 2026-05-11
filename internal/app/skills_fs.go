package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UserSkill is a named prompt template loaded from a .md file.
type UserSkill struct {
	Name        string
	Description string
	Prompt      string
	FilePath    string
}

// loadSkills scans dir for *.md files and parses each as a skill.
func loadSkills(dir string) ([]UserSkill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []UserSkill
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		if !isValidSkillName(name) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		desc, prompt := parseSkillFile(string(data))
		out = append(out, UserSkill{
			Name:        name,
			Description: desc,
			Prompt:      strings.TrimSpace(prompt),
			FilePath:    path,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// parseSkillFile splits optional YAML frontmatter from prompt content.
// Frontmatter is delimited by --- lines; description: key is extracted.
//
//	---
//	description: My skill
//	---
//	Prompt text here…
func parseSkillFile(content string) (description, prompt string) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", content
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", content
	}
	for _, line := range strings.Split(strings.TrimSpace(rest[:idx]), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "description:"); ok {
			description = strings.Trim(strings.TrimSpace(after), `"'`)
		}
	}
	prompt = strings.TrimSpace(rest[idx+4:])
	return description, prompt
}

// skillTemplate is the content written when creating a new skill.
const skillTemplate = `---
description: A short description of what this skill does
---

Your prompt here. Available placeholders:

  {input}         — text typed after the skill name
  {focused_file}  — full content of the focused file (use /load first)
  {title}         — title of the focused file
`
