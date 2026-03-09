package refcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DocumentContent holds parsed document structure from docx-review.
type DocumentContent struct {
	File       string      `json:"file"`
	Paragraphs []Paragraph `json:"paragraphs"`
}

// Paragraph represents a single paragraph from docx-review output.
type Paragraph struct {
	Index          int              `json:"index"`
	Style          string           `json:"style"`
	Text           string           `json:"text"`
	TrackedChanges []TrackedChange  `json:"tracked_changes"`
}

// TrackedChange represents a tracked change within a paragraph.
type TrackedChange struct {
	Type   string `json:"type"`
	Author string `json:"author"`
	Text   string `json:"text"`
}

// referenceHeadingPatterns are case-insensitive patterns that indicate a reference section heading.
var referenceHeadingPatterns = []string{
	"references",
	"bibliography",
	"works cited",
	"literature cited",
}

// FindDocxReview locates the docx-review binary on the system.
// It checks PATH first, then falls back to /usr/local/bin/docx-review.
func FindDocxReview() (string, error) {
	if path, err := exec.LookPath("docx-review"); err == nil {
		return path, nil
	}
	const fallback = "/usr/local/bin/docx-review"
	if _, err := os.Stat(fallback); err == nil {
		return fallback, nil
	}
	return "", fmt.Errorf("docx-review not found: install it or add it to PATH (also checked %s)", fallback)
}

// ExtractFromFile runs docx-review on a .docx file and returns parsed content.
// The binary is located via FindDocxReview and invoked as:
//
//	docx-review <path> --read --json
func ExtractFromFile(ctx context.Context, docxPath string) (*DocumentContent, error) {
	binPath, err := FindDocxReview()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, binPath, docxPath, "--read", "--json")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("docx-review failed: %w\nstderr: %s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("docx-review failed: %w", err)
	}

	return ParseDocxJSON(out)
}

// ParseDocxJSON parses the JSON output of docx-review --read --json.
func ParseDocxJSON(data []byte) (*DocumentContent, error) {
	var doc DocumentContent
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse docx-review JSON: %w", err)
	}
	return &doc, nil
}

// SplitBodyAndReferences separates body text from the references section.
// It walks paragraphs looking for a reference-heading paragraph, then splits:
// everything before the heading is body, everything after is references.
// Both are returned as newline-joined text. The heading itself is excluded.
func SplitBodyAndReferences(doc *DocumentContent) (body string, refs string) {
	splitIdx := -1
	for i, p := range doc.Paragraphs {
		if isReferenceHeading(p) {
			splitIdx = i
			break
		}
	}

	if splitIdx < 0 {
		// No reference section found; everything is body.
		var parts []string
		for _, p := range doc.Paragraphs {
			if t := strings.TrimSpace(p.Text); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n"), ""
	}

	var bodyParts, refParts []string
	for i, p := range doc.Paragraphs {
		t := strings.TrimSpace(p.Text)
		if t == "" {
			continue
		}
		if i < splitIdx {
			bodyParts = append(bodyParts, t)
		} else if i > splitIdx {
			refParts = append(refParts, t)
		}
		// i == splitIdx is the heading itself; skip it.
	}
	return strings.Join(bodyParts, "\n"), strings.Join(refParts, "\n")
}

// isReferenceHeading returns true if the paragraph looks like a reference section heading.
// For heading-styled paragraphs, it checks if the trimmed text exactly matches a known pattern.
// For non-heading styles, it also requires an exact match (to avoid false positives
// on body text that merely mentions the word "references").
func isReferenceHeading(p Paragraph) bool {
	text := strings.ToLower(strings.TrimSpace(p.Text))
	for _, pattern := range referenceHeadingPatterns {
		if text == pattern {
			return true
		}
	}
	return false
}
