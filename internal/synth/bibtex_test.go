package synth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateBibTeX(t *testing.T) {
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
					Title:       "Test Article Title",
					Authors:     "Smith, John",
					AuthorsList: []string{"Smith, John"},
					Journal:     "Nature",
					Year:        "2024",
					DOI:         "10.1234/test",
					PMID:        "12345678",
				},
			},
			contains: []string{
				"@article{",
				"author = {Smith, John}",
				"title = {Test Article Title}",
				"journal = {Nature}",
				"year = {2024}",
				"doi = {10.1234/test}",
				"pmid = {12345678}",
			},
			notEmpty: true,
		},
		{
			name: "multiple authors in AuthorsList",
			refs: []Reference{
				{
					Title:       "Collaborative Study",
					AuthorsList: []string{"Smith, John", "Jones, Jane", "Doe, Bob"},
					Year:        "2023",
				},
			},
			contains: []string{
				"author = {Smith, John and Jones, Jane and Doe, Bob}",
			},
			notEmpty: true,
		},
		{
			name: "fallback to Authors string",
			refs: []Reference{
				{
					Title:   "Study",
					Authors: "Smith, John & Jones, Jane",
					Year:    "2023",
				},
			},
			contains: []string{
				"author = {",
			},
			notEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateBibTeX(tc.refs)

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

func TestGenerateBibTeXEntry(t *testing.T) {
	ref := Reference{
		Title:       "Test Title",
		AuthorsList: []string{"Smith, John"},
		Journal:     "Test Journal",
		Year:        "2024",
		DOI:         "10.1234/test",
		PMID:        "99999999",
	}

	result := generateBibTeXEntry("Smith2024", ref)

	// Check structure
	if !strings.HasPrefix(result, "@article{Smith2024,") {
		t.Error("entry should start with @article{Smith2024,")
	}
	if !strings.HasSuffix(result, "}") {
		t.Error("entry should end with }")
	}
}

func TestLatexEscapeBibTeX(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain text", "hello world", "hello world"},
		{"ampersand", "Smith & Jones", "Smith \\& Jones"},
		{"percent", "50% of cases", "50\\% of cases"},
		{"dollar sign", "$100 cost", "\\$100 cost"},
		{"hash", "item #1", "item \\#1"},
		{"underscore", "var_name", "var\\_name"},
		{"curly braces", "{text}", "\\{text\\}"},
		{"backslash", "path\\to", "path\\\\to"},
		{"tilde", "~user", "\\~{}user"},
		{"caret", "x^2", "x\\^{}2"},
		{"multiple special chars", "A & B: 100% ($)", "A \\& B: 100\\% (\\$)"},
		{"newlines removed", "line1\nline2", "line1 line2"},
		{"tabs removed", "col1\tcol2", "col1 col2"},
		{"leading/trailing whitespace", "  hello  ", "hello"},
		{"empty string", "", ""},
		{"CRLF", "a\r\nb", "a b"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := latexEscapeBibTeX(tc.input)
			if got != tc.expected {
				t.Errorf("latexEscapeBibTeX(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestParseAuthorsForBibTeX(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty string", "", nil},
		{"whitespace only", "   ", nil},
		{"single author", "John Smith", []string{"Smith, John"}},
		{"two authors with ampersand", "John Smith & Jane Jones", []string{"Smith, John", "Jones, Jane"}},
		{"et al. format", "John Smith et al.", []string{"Smith, John"}},
		{"already in Last, First format", "Smith, John", []string{"Smith, John"}},
		{"single name (organization)", "WHO", []string{"WHO"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseAuthorsForBibTeX(tc.input)

			if len(got) != len(tc.expected) {
				t.Errorf("parseAuthorsForBibTeX(%q) returned %d authors, want %d\ngot: %v", tc.input, len(got), len(tc.expected), got)
				return
			}

			for i, author := range got {
				if author != tc.expected[i] {
					t.Errorf("parseAuthorsForBibTeX(%q)[%d] = %q, want %q", tc.input, i, author, tc.expected[i])
				}
			}
		})
	}
}

func TestBibtexAuthorFromName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", "Unknown"},
		{"whitespace", "   ", "Unknown"},
		{"first last", "John Smith", "Smith, John"},
		{"first middle last", "John Paul Smith", "Smith, John Paul"},
		{"already comma format", "Smith, John", "Smith, John"},
		{"single name", "Madonna", "Madonna"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := bibtexAuthorFromName(tc.input)
			if got != tc.expected {
				t.Errorf("bibtexAuthorFromName(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestGenerateBibTeXCitationKeys(t *testing.T) {
	tests := []struct {
		name     string
		refs     []Reference
		expected []string
	}{
		{
			name:     "empty refs",
			refs:     []Reference{},
			expected: []string{},
		},
		{
			name: "single author with year",
			refs: []Reference{
				{Authors: "Smith, John", Year: "2024"},
			},
			expected: []string{"Smith2024"},
		},
		{
			name: "duplicate keys get suffix",
			refs: []Reference{
				{Authors: "Smith, John", Year: "2024"},
				{Authors: "Smith, Jane", Year: "2024"},
			},
			expected: []string{"Smith2024", "Smith2024a"},
		},
		{
			name: "three duplicates",
			refs: []Reference{
				{Authors: "Smith, John", Year: "2024"},
				{Authors: "Smith, Jane", Year: "2024"},
				{Authors: "Smith, Bob", Year: "2024"},
			},
			expected: []string{"Smith2024", "Smith2024a", "Smith2024b"},
		},
		{
			name: "no year uses nd",
			refs: []Reference{
				{Authors: "Smith, John", Year: ""},
			},
			expected: []string{"Smithnd"},
		},
		{
			name: "no author uses Ref",
			refs: []Reference{
				{Authors: "", Year: "2024"},
			},
			expected: []string{"Unknown2024"},
		},
		{
			name: "AuthorsList takes precedence",
			refs: []Reference{
				{Authors: "Wrong Author", AuthorsList: []string{"Right, Person"}, Year: "2024"},
			},
			expected: []string{"Right2024"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := generateBibTeXCitationKeys(tc.refs)

			if len(got) != len(tc.expected) {
				t.Errorf("got %d keys, want %d: %v", len(got), len(tc.expected), got)
				return
			}

			for i, key := range got {
				if key != tc.expected[i] {
					t.Errorf("key[%d] = %q, want %q", i, key, tc.expected[i])
				}
			}
		})
	}
}

func TestBibtexCitationKeyBase(t *testing.T) {
	tests := []struct {
		name     string
		ref      Reference
		expected string
	}{
		{
			name:     "author and year",
			ref:      Reference{Authors: "Smith, John", Year: "2024"},
			expected: "Smith2024",
		},
		{
			name:     "et al. author",
			ref:      Reference{Authors: "Smith et al.", Year: "2024"},
			expected: "Smith2024",
		},
		{
			name:     "year with month",
			ref:      Reference{Authors: "Jones", Year: "2024 Jan"},
			expected: "Jones2024",
		},
		{
			name:     "no year",
			ref:      Reference{Authors: "Smith"},
			expected: "Smithnd",
		},
		{
			name:     "AuthorsList used",
			ref:      Reference{AuthorsList: []string{"Johnson, Mary"}, Year: "2023"},
			expected: "Johnson2023",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := bibtexCitationKeyBase(tc.ref)
			if got != tc.expected {
				t.Errorf("bibtexCitationKeyBase() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestBibtexKeyAuthorToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", "Unknown"},
		{"single name", "Smith", "Smith"},
		{"first last", "John Smith", "Smith"},
		{"comma format", "Smith, John", "Smith"},
		{"whitespace", "   ", "Unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := bibtexKeyAuthorToken(tc.input)
			if got != tc.expected {
				t.Errorf("bibtexKeyAuthorToken(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestYearForBibTeXKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple year", "2024", "2024"},
		{"year with month", "2024 Jan", "2024"},
		{"year in text", "Published in 2023", "2023"},
		{"empty", "", "nd"},
		{"no digits", "unknown", "nd"},
		{"partial year", "202", "nd"},
		{"year at end", "Jan 2022", "2022"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := yearForBibTeXKey(tc.input)
			if got != tc.expected {
				t.Errorf("yearForBibTeXKey(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestSanitizeBibTeXKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"alphanumeric", "Smith2024", "Smith2024"},
		{"with spaces", "Smith 2024", "Smith2024"},
		{"with special chars", "Smith-Jones_2024", "SmithJones2024"},
		{"starts with digit", "2024Smith", "Ref2024Smith"},
		{"unicode removed", "Müller2024", "Mller2024"},
		{"empty", "", ""},
		{"too long truncated", strings.Repeat("a", 100), strings.Repeat("a", 64)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeBibTeXKey(tc.input)
			if got != tc.expected {
				t.Errorf("sanitizeBibTeXKey(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestAlphaSuffix(t *testing.T) {
	tests := []struct {
		n        int
		expected string
	}{
		{0, ""},
		{1, "a"},
		{2, "b"},
		{26, "z"},
		{27, "aa"},
		{28, "ab"},
		{52, "az"},
		{53, "ba"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			got := alphaSuffix(tc.n)
			if got != tc.expected {
				t.Errorf("alphaSuffix(%d) = %q, want %q", tc.n, got, tc.expected)
			}
		})
	}
}

func TestBibtexAuthors(t *testing.T) {
	tests := []struct {
		name     string
		ref      Reference
		expected string
	}{
		{
			name:     "from AuthorsList",
			ref:      Reference{AuthorsList: []string{"Smith, John", "Jones, Jane"}},
			expected: "Smith, John and Jones, Jane",
		},
		{
			name:     "from Authors string",
			ref:      Reference{Authors: "John Smith & Jane Jones"},
			expected: "Smith, John and Jones, Jane",
		},
		{
			name:     "AuthorsList takes precedence",
			ref:      Reference{Authors: "Wrong", AuthorsList: []string{"Right, Person"}},
			expected: "Right, Person",
		},
		{
			name:     "empty",
			ref:      Reference{},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := bibtexAuthors(tc.ref)
			if got != tc.expected {
				t.Errorf("bibtexAuthors() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestWriteBibTeXFile(t *testing.T) {
	t.Run("empty filename", func(t *testing.T) {
		err := WriteBibTeXFile("", []Reference{})
		if err == nil {
			t.Error("expected error for empty filename")
		}
	})

	t.Run("whitespace filename", func(t *testing.T) {
		err := WriteBibTeXFile("   ", []Reference{})
		if err == nil {
			t.Error("expected error for whitespace filename")
		}
	})

	t.Run("successful write", func(t *testing.T) {
		tempDir := t.TempDir()
		filename := filepath.Join(tempDir, "test.bib")

		refs := []Reference{
			{
				Title:       "Test Article",
				AuthorsList: []string{"Author, Test"},
				Year:        "2024",
			},
		}

		err := WriteBibTeXFile(filename, refs)
		if err != nil {
			t.Fatalf("WriteBibTeXFile failed: %v", err)
		}

		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		if !strings.Contains(string(content), "@article{") {
			t.Error("file should contain BibTeX content")
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		tempDir := t.TempDir()
		filename := filepath.Join(tempDir, "subdir", "nested", "test.bib")

		refs := []Reference{{Title: "Test", AuthorsList: []string{"Author"}}}

		err := WriteBibTeXFile(filename, refs)
		if err != nil {
			t.Fatalf("WriteBibTeXFile failed: %v", err)
		}

		if _, err := os.Stat(filename); os.IsNotExist(err) {
			t.Error("file should exist")
		}
	})
}
