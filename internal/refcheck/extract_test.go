package refcheck

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestFindDocxReview(t *testing.T) {
	path, err := FindDocxReview()
	if err != nil {
		t.Skipf("docx-review not found, skipping: %v", err)
	}
	if path == "" {
		t.Fatal("FindDocxReview returned empty path without error")
	}
	// Verify the binary actually exists and is executable.
	if _, err := exec.LookPath(path); err != nil {
		t.Errorf("returned path %q is not executable: %v", path, err)
	}
}

func TestParseDocxJSON(t *testing.T) {
	input := []byte(`{
		"file": "test.docx",
		"paragraphs": [
			{"index": 0, "style": "Heading1", "text": "Introduction", "tracked_changes": []},
			{"index": 1, "style": "Normal", "text": "Some body text with citation [1].", "tracked_changes": []},
			{"index": 2, "style": "Normal", "text": "More text referencing (Bear et al., 2004).", "tracked_changes": []},
			{"index": 3, "style": "Heading1", "text": "References", "tracked_changes": []},
			{"index": 4, "style": "Normal", "text": "1. Bear MF, Huber KM, Warren ST. The mGluR theory of fragile X mental retardation. Trends Neurosci. 2004;27(7):370-377.", "tracked_changes": []},
			{"index": 5, "style": "Normal", "text": "2. Hagerman RJ, et al. Fragile X syndrome. Nat Rev Dis Primers. 2017;3:17065. PMID: 28960184", "tracked_changes": []}
		]
	}`)

	doc, err := ParseDocxJSON(input)
	if err != nil {
		t.Fatalf("ParseDocxJSON: %v", err)
	}
	if len(doc.Paragraphs) != 6 {
		t.Fatalf("expected 6 paragraphs, got %d", len(doc.Paragraphs))
	}

	// Check first paragraph.
	p0 := doc.Paragraphs[0]
	if p0.Index != 0 {
		t.Errorf("paragraph 0 Index = %d, want 0", p0.Index)
	}
	if p0.Style != "Heading1" {
		t.Errorf("paragraph 0 Style = %q, want %q", p0.Style, "Heading1")
	}
	if p0.Text != "Introduction" {
		t.Errorf("paragraph 0 Text = %q, want %q", p0.Text, "Introduction")
	}

	// Check last paragraph.
	p5 := doc.Paragraphs[5]
	if p5.Index != 5 {
		t.Errorf("paragraph 5 Index = %d, want 5", p5.Index)
	}
	if !strings.Contains(p5.Text, "Hagerman") {
		t.Errorf("paragraph 5 Text = %q, want to contain %q", p5.Text, "Hagerman")
	}
}

func TestParseDocxJSON_InvalidJSON(t *testing.T) {
	_, err := ParseDocxJSON([]byte(`{not valid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseDocxJSON_EmptyParagraphs(t *testing.T) {
	input := []byte(`{"file": "test.docx", "paragraphs": []}`)
	doc, err := ParseDocxJSON(input)
	if err != nil {
		t.Fatalf("ParseDocxJSON: %v", err)
	}
	if len(doc.Paragraphs) != 0 {
		t.Errorf("expected 0 paragraphs, got %d", len(doc.Paragraphs))
	}
}

func TestDetectReferenceHeading(t *testing.T) {
	tests := []struct {
		name  string
		para  Paragraph
		want  bool
	}{
		{
			name: "Heading1 References",
			para: Paragraph{Style: "Heading1", Text: "References"},
			want: true,
		},
		{
			name: "Heading1 REFERENCES uppercase",
			para: Paragraph{Style: "Heading1", Text: "REFERENCES"},
			want: true,
		},
		{
			name: "Heading2 Bibliography",
			para: Paragraph{Style: "Heading2", Text: "Bibliography"},
			want: true,
		},
		{
			name: "Heading1 Works Cited",
			para: Paragraph{Style: "Heading1", Text: "Works Cited"},
			want: true,
		},
		{
			name: "Heading1 Literature Cited",
			para: Paragraph{Style: "Heading1", Text: "Literature Cited"},
			want: true,
		},
		{
			name: "Heading1 mixed case references",
			para: Paragraph{Style: "Heading1", Text: "references"},
			want: true,
		},
		{
			name: "Normal style but reference text",
			para: Paragraph{Style: "Normal", Text: "References"},
			want: true,
		},
		{
			name: "Normal body text mentioning references",
			para: Paragraph{Style: "Normal", Text: "See the references section for details."},
			want: false,
		},
		{
			name: "Heading1 Introduction",
			para: Paragraph{Style: "Heading1", Text: "Introduction"},
			want: false,
		},
		{
			name: "Heading1 References with trailing whitespace",
			para: Paragraph{Style: "Heading1", Text: "References "},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isReferenceHeading(tt.para)
			if got != tt.want {
				t.Errorf("isReferenceHeading(%+v) = %v, want %v", tt.para, got, tt.want)
			}
		})
	}
}

func TestSplitBodyAndReferences(t *testing.T) {
	doc := &DocumentContent{
		Paragraphs: []Paragraph{
			{Index: 0, Style: "Heading1", Text: "Introduction"},
			{Index: 1, Style: "Normal", Text: "Some body text with citation [1]."},
			{Index: 2, Style: "Normal", Text: "More text referencing (Bear et al., 2004)."},
			{Index: 3, Style: "Heading1", Text: "References"},
			{Index: 4, Style: "Normal", Text: "1. Bear MF, Huber KM, Warren ST. The mGluR theory of fragile X mental retardation. Trends Neurosci. 2004;27(7):370-377."},
			{Index: 5, Style: "Normal", Text: "2. Hagerman RJ, et al. Fragile X syndrome. Nat Rev Dis Primers. 2017;3:17065. PMID: 28960184"},
		},
	}

	body, refs := SplitBodyAndReferences(doc)

	// Body should contain body paragraphs, not references.
	if !strings.Contains(body, "Some body text") {
		t.Errorf("body missing expected text, got %q", body)
	}
	if !strings.Contains(body, "Introduction") {
		t.Errorf("body missing heading text, got %q", body)
	}
	if strings.Contains(body, "Bear MF") {
		t.Errorf("body should not contain reference text, got %q", body)
	}

	// References should contain reference paragraphs.
	if !strings.Contains(refs, "Bear MF") {
		t.Errorf("refs missing expected text, got %q", refs)
	}
	if !strings.Contains(refs, "Hagerman RJ") {
		t.Errorf("refs missing expected text, got %q", refs)
	}
	// The heading itself should not be in refs text.
	lines := strings.Split(refs, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "References" {
			t.Error("refs should not include the heading 'References' as a standalone line")
		}
	}
}

func TestSplitBodyAndReferences_VariousCasings(t *testing.T) {
	tests := []struct {
		name    string
		heading string
	}{
		{"uppercase", "REFERENCES"},
		{"lowercase", "references"},
		{"mixed", "References"},
		{"bibliography", "Bibliography"},
		{"works cited", "Works Cited"},
		{"literature cited", "Literature Cited"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &DocumentContent{
				Paragraphs: []Paragraph{
					{Index: 0, Style: "Normal", Text: "Body paragraph."},
					{Index: 1, Style: "Heading1", Text: tt.heading},
					{Index: 2, Style: "Normal", Text: "1. Some reference."},
				},
			}
			body, refs := SplitBodyAndReferences(doc)
			if !strings.Contains(body, "Body paragraph") {
				t.Errorf("body missing expected text for heading %q, got %q", tt.heading, body)
			}
			if !strings.Contains(refs, "Some reference") {
				t.Errorf("refs missing expected text for heading %q, got %q", tt.heading, refs)
			}
		})
	}
}

func TestSplitBodyAndReferences_NoReferencesSection(t *testing.T) {
	doc := &DocumentContent{
		Paragraphs: []Paragraph{
			{Index: 0, Style: "Heading1", Text: "Introduction"},
			{Index: 1, Style: "Normal", Text: "Some body text."},
			{Index: 2, Style: "Heading1", Text: "Methods"},
			{Index: 3, Style: "Normal", Text: "More body text."},
		},
	}

	body, refs := SplitBodyAndReferences(doc)
	if refs != "" {
		t.Errorf("expected empty refs, got %q", refs)
	}
	if !strings.Contains(body, "Some body text") {
		t.Errorf("body missing expected text, got %q", body)
	}
	if !strings.Contains(body, "More body text") {
		t.Errorf("body missing expected text, got %q", body)
	}
}

func TestSplitBodyAndReferences_EmptyDocument(t *testing.T) {
	doc := &DocumentContent{
		Paragraphs: []Paragraph{},
	}
	body, refs := SplitBodyAndReferences(doc)
	if body != "" {
		t.Errorf("expected empty body, got %q", body)
	}
	if refs != "" {
		t.Errorf("expected empty refs, got %q", refs)
	}
}

func TestSplitBodyAndReferences_ReferencesOnly(t *testing.T) {
	doc := &DocumentContent{
		Paragraphs: []Paragraph{
			{Index: 0, Style: "Heading1", Text: "References"},
			{Index: 1, Style: "Normal", Text: "1. Some reference."},
			{Index: 2, Style: "Normal", Text: "2. Another reference."},
		},
	}
	body, refs := SplitBodyAndReferences(doc)
	if body != "" {
		t.Errorf("expected empty body when only refs, got %q", body)
	}
	if !strings.Contains(refs, "Some reference") {
		t.Errorf("refs missing expected text, got %q", refs)
	}
}

func TestExtractFromFile(t *testing.T) {
	// Integration test: skip if docx-review not found.
	_, err := FindDocxReview()
	if err != nil {
		t.Skipf("docx-review not found, skipping integration test: %v", err)
	}

	// We need a real .docx file; skip if none available.
	// This test is primarily for manual/local runs.
	t.Skip("no test .docx fixture available; run manually with a real document")

	ctx := context.Background()
	doc, err := ExtractFromFile(ctx, "/path/to/test.docx")
	if err != nil {
		t.Fatalf("ExtractFromFile: %v", err)
	}
	if len(doc.Paragraphs) == 0 {
		t.Error("expected at least one paragraph")
	}
}

func TestExtractFromFile_MissingFile(t *testing.T) {
	_, err := FindDocxReview()
	if err != nil {
		t.Skipf("docx-review not found, skipping: %v", err)
	}

	ctx := context.Background()
	_, err = ExtractFromFile(ctx, "/nonexistent/path/fake.docx")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}
