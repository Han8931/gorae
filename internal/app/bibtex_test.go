package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorae/internal/meta"
)

func TestBuildBibtexEntryWithMetadata(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "attention.pdf")
	if err := os.WriteFile(pdfPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create temp pdf: %v", err)
	}

	md := &meta.Metadata{
		Title:     "Attention Is All You Need",
		Author:    "Vaswani, Ashish and others",
		Venue:     "NeurIPS",
		Year:      "2017",
		Published: "2017-06-12",
		URL:       "https://arxiv.org/abs/1706.03762",
		DOI:       "10.48550/arXiv.1706.03762",
		Tag:       "transformers, attention",
	}

	entry, err := buildBibtexEntry(md, pdfPath)
	if err != nil {
		t.Fatalf("buildBibtexEntry returned error: %v", err)
	}
	if !strings.Contains(entry, "@article{Vaswani2017Attention,") {
		t.Fatalf("expected cite key to include Vaswani2017Attention, got %q", entry)
	}
	if !strings.Contains(entry, "title = {Attention Is All You Need}") {
		t.Fatalf("entry missing title: %q", entry)
	}
	if !strings.Contains(entry, "journal = {NeurIPS}") {
		t.Fatalf("entry missing venue: %q", entry)
	}
	if !strings.Contains(entry, "published = {2017-06-12}") {
		t.Fatalf("entry missing published date: %q", entry)
	}
	if !strings.Contains(entry, "url = {https://arxiv.org/abs/1706.03762}") {
		t.Fatalf("entry missing url: %q", entry)
	}
	if !strings.Contains(entry, "doi = {10.48550/arXiv.1706.03762}") {
		t.Fatalf("entry missing doi: %q", entry)
	}
	if !strings.Contains(entry, "keywords = {transformers, attention}") {
		t.Fatalf("entry missing keywords: %q", entry)
	}
}

func TestBuildBibtexEntryFallback(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "sample-file.pdf")
	if err := os.WriteFile(pdfPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create temp pdf: %v", err)
	}

	entry, err := buildBibtexEntry(nil, pdfPath)
	if err != nil {
		t.Fatalf("buildBibtexEntry returned error: %v", err)
	}
	if !strings.Contains(entry, "@misc{samplefile,") {
		t.Fatalf("expected cite key to fallback to filename, got %q", entry)
	}
	if !strings.Contains(entry, "title = {sample-file}") {
		t.Fatalf("fallback title missing: %q", entry)
	}
	if !strings.Contains(entry, "published = {}") {
		t.Fatalf("expected published field even when empty: %q", entry)
	}
	if !strings.Contains(entry, "url = {}") {
		t.Fatalf("expected url field even when empty: %q", entry)
	}
	if !strings.Contains(entry, "file = {"+pdfPath+"}") {
		t.Fatalf("entry missing file path reference: %q", entry)
	}
}
