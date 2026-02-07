package synth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateRIS(t *testing.T) {
	tests := []struct {
		name     string
		refs     []Reference
		contains []string
		notEmpty bool
	}{
		{
			name:     "empty references",
			refs:     []Reference{},
			contains: nil,
			notEmpty: false,
		},
		{
			name:     "nil references",
			refs:     nil,
			contains: nil,
			notEmpty: false,
		},
		{
			name: "single reference with all fields",
			refs: []Reference{
				{
					Title:    "Test Article Title",
					Authors:  "Smith, John",
					Journal:  "Nature",
					Year:     "2024",
					DOI:      "10.1234/test",
					PMID:     "12345678",
					Abstract: "This is a test abstract.",
				},
			},
			contains: []string{
				"TY  - JOUR",
				"AU  - Smith, John",
				"TI  - Test Article Title",
				"JO  - Nature",
				"PY  - 2024",
				"DO  - 10.1234/test",
				"AN  - 12345678",
				"AB  - This is a test abstract.",
				"DB  - PubMed",
				"UR  - https://pubmed.ncbi.nlm.nih.gov/12345678/",
				"ER  -",
			},
			notEmpty: true,
		},
		{
			name: "multiple authors with ampersand",
			refs: []Reference{
				{
					Title:   "Collaborative Study",
					Authors: "Smith, John & Jones, Jane",
					Journal: "Science",
					Year:    "2023",
				},
			},
			contains: []string{
				"AU  - Smith, John",
				"AU  - Jones, Jane",
			},
			notEmpty: true,
		},
		{
			name: "et al. author format",
			refs: []Reference{
				{
					Title:   "Large Team Study",
					Authors: "Smith, John et al.",
					Journal: "Cell",
					Year:    "2022",
				},
			},
			contains: []string{
				"AU  - Smith, John",
			},
			notEmpty: true,
		},
		{
			name: "missing optional fields",
			refs: []Reference{
				{
					Title:   "Minimal Reference",
					Authors: "Doe, Jane",
				},
			},
			contains: []string{
				"TY  - JOUR",
				"TI  - Minimal Reference",
				"AU  - Doe, Jane",
				"ER  -",
			},
			notEmpty: true,
		},
		{
			name: "multiple references",
			refs: []Reference{
				{Title: "First Article", Authors: "Author One"},
				{Title: "Second Article", Authors: "Author Two"},
			},
			contains: []string{
				"TI  - First Article",
				"TI  - Second Article",
			},
			notEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateRIS(tc.refs)

			if tc.notEmpty && result == "" {
				t.Error("expected non-empty result")
			}
			if !tc.notEmpty && result != "" {
				t.Errorf("expected empty result, got: %s", result)
			}

			for _, expected := range tc.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("result should contain %q\nGot: %s", expected, result)
				}
			}
		})
	}
}

func TestGenerateRISEntry(t *testing.T) {
	ref := Reference{
		Title:    "Test Title",
		Authors:  "Smith, John",
		Journal:  "Test Journal",
		Year:     "2024",
		DOI:      "10.1234/test",
		PMID:     "99999999",
		Abstract: "Test abstract content.",
	}

	result := generateRISEntry(ref)

	// Check structure
	lines := strings.Split(result, "\n")
	if len(lines) < 3 {
		t.Errorf("expected at least 3 lines, got %d", len(lines))
	}

	// Must start with TY and end with ER
	if !strings.HasPrefix(result, "TY  - JOUR") {
		t.Error("entry should start with TY  - JOUR")
	}
	if !strings.HasSuffix(result, "ER  -") {
		t.Error("entry should end with ER  -")
	}
}

func TestSanitizeRIS(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain text", "hello world", "hello world"},
		{"with newlines", "line1\nline2", "line1 line2"},
		{"with carriage return", "line1\rline2", "line1 line2"},
		{"with CRLF", "line1\r\nline2", "line1 line2"},
		{"with tabs", "col1\tcol2", "col1 col2"},
		{"leading/trailing whitespace", "  hello  ", "hello"},
		{"empty string", "", ""},
		{"multiple newlines", "a\n\n\nb", "a   b"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeRIS(tc.input)
			if got != tc.expected {
				t.Errorf("sanitizeRIS(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestParseAuthorsForRIS(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty string", "", []string{"Unknown"}},
		{"whitespace only", "   ", []string{"Unknown"}},
		{"single author", "Smith, John", []string{"Smith, John"}},
		{"two authors with ampersand", "Smith, John & Jones, Jane", []string{"Smith, John", "Jones, Jane"}},
		{"et al. format", "Smith, John et al.", []string{"Smith, John"}},
		{"et al. with extra text", "Smith, John et al. (2024)", []string{"Smith, John"}},
		{"three authors with ampersand", "A & B & C", []string{"A", "B", "C"}},
		{"author name only", "Unknown Author", []string{"Unknown Author"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseAuthorsForRIS(tc.input)

			if len(got) != len(tc.expected) {
				t.Errorf("parseAuthorsForRIS(%q) returned %d authors, want %d", tc.input, len(got), len(tc.expected))
				return
			}

			for i, author := range got {
				if author != tc.expected[i] {
					t.Errorf("parseAuthorsForRIS(%q)[%d] = %q, want %q", tc.input, i, author, tc.expected[i])
				}
			}
		})
	}
}

func TestWriteRISFile(t *testing.T) {
	t.Run("empty filename", func(t *testing.T) {
		err := WriteRISFile("", []Reference{})
		if err == nil {
			t.Error("expected error for empty filename")
		}
	})

	t.Run("successful write", func(t *testing.T) {
		tempDir := t.TempDir()
		filename := filepath.Join(tempDir, "test.ris")

		refs := []Reference{
			{
				Title:   "Test Article",
				Authors: "Test Author",
				Year:    "2024",
			},
		}

		err := WriteRISFile(filename, refs)
		if err != nil {
			t.Fatalf("WriteRISFile failed: %v", err)
		}

		// Verify file exists and has content
		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		if !strings.Contains(string(content), "TY  - JOUR") {
			t.Error("file should contain RIS content")
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		tempDir := t.TempDir()
		filename := filepath.Join(tempDir, "subdir", "nested", "test.ris")

		refs := []Reference{{Title: "Test"}}

		err := WriteRISFile(filename, refs)
		if err != nil {
			t.Fatalf("WriteRISFile failed: %v", err)
		}

		if _, err := os.Stat(filename); os.IsNotExist(err) {
			t.Error("file should exist")
		}
	})
}

func TestGenerateRIS_LongAbstract(t *testing.T) {
	// Abstract longer than 5000 runes should be truncated
	longAbstract := strings.Repeat("a", 6000)

	refs := []Reference{
		{
			Title:    "Test",
			Authors:  "Author",
			Abstract: longAbstract,
		},
	}

	result := GenerateRIS(refs)

	// The abstract line should contain the truncated text
	if !strings.Contains(result, "AB  -") {
		t.Error("should contain abstract field")
	}

	// Extract the abstract line and verify it's truncated
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "AB  - ") {
			abstractContent := strings.TrimPrefix(line, "AB  - ")
			if len([]rune(abstractContent)) > 5010 { // 5000 + "..."
				t.Errorf("abstract should be truncated to ~5000 runes, got %d", len([]rune(abstractContent)))
			}
		}
	}
}
