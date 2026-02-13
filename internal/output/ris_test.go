package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
)

func TestWriteArticlesRIS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "articles.ris")

	articles := []eutils.Article{
		{
			PMID:     "38000001",
			Title:    "Testing RIS Export",
			Abstract: "Line one.\nLine two.",
			Authors: []eutils.Author{
				{LastName: "Smith", ForeName: "Jane"},
				{CollectiveName: "PubMed CLI Consortium"},
			},
			Journal: "Journal of CLI Testing",
			Year:    "2026",
			Volume:  "12",
			Issue:   "3",
			Pages:   "101-110",
			DOI:     "10.1000/example",
		},
	}

	if err := writeArticlesRIS(path, articles); err != nil {
		t.Fatalf("unexpected error writing RIS: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read RIS output: %v", err)
	}
	out := string(body)

	expected := []string{
		"TY  - JOUR",
		"TI  - Testing RIS Export",
		"AU  - Smith, Jane",
		"AU  - PubMed CLI Consortium",
		"PY  - 2026",
		"JO  - Journal of CLI Testing",
		"VL  - 12",
		"IS  - 3",
		"SP  - 101",
		"EP  - 110",
		"DO  - 10.1000/example",
		"AB  - Line one. Line two.",
		"ID  - PMID:38000001",
		"UR  - https://pubmed.ncbi.nlm.nih.gov/38000001/",
		"ER  -",
	}
	for _, want := range expected {
		if !strings.Contains(out, want) {
			t.Fatalf("expected RIS output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestSplitPages(t *testing.T) {
	tests := []struct {
		in     string
		wantSP string
		wantEP string
	}{
		{"100-110", "100", "110"},
		{"200", "200", ""},
		{" e11-e19 ", "e11", "e19"},
	}

	for _, tt := range tests {
		sp, ep := splitPages(tt.in)
		if sp != tt.wantSP || ep != tt.wantEP {
			t.Fatalf("splitPages(%q) => (%q, %q), expected (%q, %q)", tt.in, sp, ep, tt.wantSP, tt.wantEP)
		}
	}
}
