package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/atotto/clipboard"

	"gorae/internal/meta"
)

var yearPattern = regexp.MustCompile(`\d{4}`)

func (m *Model) copyBibtexToClipboard() error {
	path := m.currentYankTarget()
	if path == "" {
		return fmt.Errorf("no file selected")
	}

	canonical := canonicalPath(path)
	if canonical == "" {
		canonical = path
	}

	var md *meta.Metadata
	if m.currentMeta != nil && m.currentMetaPath == canonical {
		md = m.currentMeta
	} else if m.meta != nil {
		ctx := context.Background()
		var err error
		md, err = m.meta.Get(ctx, canonical)
		if err != nil {
			return fmt.Errorf("failed to load metadata: %w", err)
		}
	}

	entry, err := buildBibtexEntry(md, canonical)
	if err != nil {
		return err
	}
	if err := clipboard.WriteAll(entry); err != nil {
		return fmt.Errorf("failed to access clipboard: %w", err)
	}
	return nil
}

// copyTitleAuthorYearToClipboard copies a simple "Title — Author — Year" string
// suitable for pasting into plain text editors or word processors. It falls
// back to file name when metadata is missing.
func (m *Model) copyTitleAuthorYearToClipboard() error {
	path := m.currentYankTarget()
	if path == "" {
		return fmt.Errorf("no file selected")
	}

	canonical := canonicalPath(path)
	if canonical == "" {
		canonical = path
	}

	var md *meta.Metadata
	if m.currentMeta != nil && m.currentMetaPath == canonical {
		md = m.currentMeta
	} else if m.meta != nil {
		ctx := context.Background()
		var err error
		md, err = m.meta.Get(ctx, canonical)
		if err != nil {
			return fmt.Errorf("failed to load metadata: %w", err)
		}
	}

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if base == "" {
		base = filepath.Base(path)
	}

	title := base
	author := ""
	year := ""
	if md != nil {
		if v := strings.TrimSpace(md.Title); v != "" {
			title = v
		}
		if v := strings.TrimSpace(md.Author); v != "" {
			author = v
		}
		if v := strings.TrimSpace(md.Year); v != "" {
			if y := extractYear(v); y != "" {
				year = y
			} else {
				year = v
			}
		}
	}

	parts := []string{title}
	if author != "" {
		parts = append(parts, author)
	}
	if year != "" {
		parts = append(parts, year)
	}
	summary := strings.Join(parts, ", ")
	if summary == "" {
		return fmt.Errorf("nothing to copy")
	}
	if err := clipboard.WriteAll(summary); err != nil {
		return fmt.Errorf("failed to access clipboard: %w", err)
	}
	return nil
}

func buildBibtexEntry(md *meta.Metadata, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to inspect %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("select a PDF file first")
	}
	if ext := strings.ToLower(filepath.Ext(info.Name())); ext != ".pdf" {
		return "", fmt.Errorf("BibTeX is only available for PDF files")
	}

	base := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
	if base == "" {
		base = info.Name()
	}
	title := base
	author := ""
	year := ""
	published := ""
	url := ""
	doi := ""
	abstract := ""
	keywords := ""
	if md != nil {
		if v := strings.TrimSpace(md.Title); v != "" {
			title = v
		}
		author = normalizeSpaces(md.Author)
		year = strings.TrimSpace(md.Year)
		published = normalizeSpaces(md.Published)
		url = strings.TrimSpace(md.URL)
		doi = strings.TrimSpace(md.DOI)
		abstract = normalizeSpaces(md.Abstract)
		keywords = normalizeKeywords(md.Tag)
	}

	entryType := determineBibtexType(author, published)
	citeKey := buildBibtexKey(md, title, path)
	normYear := extractYear(year)

	fields := make([]bibField, 0, 7)
	fields = append(fields, bibField{name: "title", value: title})
	if author != "" {
		fields = append(fields, bibField{name: "author", value: author})
	}
	if published != "" {
		fieldName := "journal"
		if entryType == "inproceedings" {
			fieldName = "booktitle"
		}
		fields = append(fields, bibField{name: fieldName, value: published})
	}
	if normYear != "" {
		fields = append(fields, bibField{name: "year", value: normYear})
	}
	fields = append(fields, bibField{name: "published", value: published})
	if keywords != "" {
		fields = append(fields, bibField{name: "keywords", value: keywords})
	}
	if abstract != "" {
		fields = append(fields, bibField{name: "abstract", value: abstract})
	}
	fields = append(fields, bibField{name: "url", value: url})
	if doi != "" {
		fields = append(fields, bibField{name: "doi", value: doi})
	}
	fields = append(fields, bibField{name: "file", value: path})

	var b strings.Builder
	fmt.Fprintf(&b, "@%s{%s,\n", entryType, citeKey)
	for i, field := range fields {
		fmt.Fprintf(&b, "  %s = {%s}", field.name, escapeBibtexValue(field.value))
		if i < len(fields)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.String(), nil
}

type bibField struct {
	name  string
	value string
}

func determineBibtexType(author, published string) string {
	author = strings.TrimSpace(author)
	published = strings.TrimSpace(published)
	if published != "" && author != "" {
		return "article"
	}
	if published != "" {
		return "inproceedings"
	}
	if author != "" {
		return "article"
	}
	return "misc"
}

func buildBibtexKey(md *meta.Metadata, title, path string) string {
	var parts []string
	if md != nil {
		if author := firstAuthorKey(md.Author); author != "" {
			parts = append(parts, author)
		}
		if year := extractYear(md.Year); year != "" {
			parts = append(parts, year)
		}
	}
	if w := firstTitleToken(title); w != "" {
		parts = append(parts, w)
	}
	candidate := sanitizeIdentifier(strings.Join(parts, ""))
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	baseCandidate := sanitizeIdentifier(base)
	if candidate == "" {
		candidate = baseCandidate
	} else if md == nil && baseCandidate != "" {
		candidate = baseCandidate
	}
	if candidate == "" {
		candidate = "entry"
	}
	runes := []rune(candidate)
	if len(runes) > 0 && !unicode.IsLetter(runes[0]) {
		candidate = "ref" + candidate
	}
	return candidate
}

func firstAuthorKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if idx := strings.Index(lower, " and "); idx >= 0 {
		raw = raw[:idx]
	} else if idx := strings.Index(raw, ";"); idx >= 0 {
		raw = raw[:idx]
	}
	if idx := strings.Index(raw, ","); idx >= 0 {
		raw = raw[:idx]
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return ""
	}
	return sanitizeIdentifier(parts[len(parts)-1])
}

func firstTitleToken(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	fields := strings.FieldsFunc(title, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	for _, word := range fields {
		if len(word) >= 3 {
			return word
		}
	}
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func extractYear(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if m := yearPattern.FindString(raw); m != "" {
		return m
	}
	return ""
}

func sanitizeIdentifier(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func escapeBibtexValue(s string) string {
	if s == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"{", "\\{",
		"}", "\\}",
	)
	return replacer.Replace(s)
}

func normalizeSpaces(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func normalizeKeywords(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.NewReplacer(";", ",", "|", ",").Replace(raw)
	parts := strings.Split(raw, ",")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		clean = append(clean, part)
	}
	return strings.Join(clean, ", ")
}
