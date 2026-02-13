package output

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
)

func TestFormatSearchJSON(t *testing.T) {
	result := &eutils.SearchResult{
		Count:            42,
		IDs:              []string{"123", "456", "789"},
		QueryTranslation: "test query",
	}

	var buf bytes.Buffer
	err := FormatSearchResult(&buf, result, nil, OutputConfig{JSON: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, buf.String())
	}

	if count, ok := parsed["count"].(float64); !ok || int(count) != 42 {
		t.Errorf("expected count 42, got %v", parsed["count"])
	}

	ids, ok := parsed["ids"].([]interface{})
	if !ok || len(ids) != 3 {
		t.Errorf("expected 3 ids, got %v", parsed["ids"])
	}
}

func TestFormatSearchPlain(t *testing.T) {
	result := &eutils.SearchResult{
		Count:            42,
		IDs:              []string{"123", "456"},
		QueryTranslation: "test query",
	}

	var buf bytes.Buffer
	err := FormatSearchResult(&buf, result, nil, OutputConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "42") {
		t.Error("expected output to contain count '42'")
	}
	if !strings.Contains(out, "123") {
		t.Error("expected output to contain PMID '123'")
	}
	if !strings.Contains(out, "456") {
		t.Error("expected output to contain PMID '456'")
	}
}

func TestFormatSearchEmpty(t *testing.T) {
	result := &eutils.SearchResult{
		Count: 0,
		IDs:   []string{},
	}

	var buf bytes.Buffer
	err := FormatSearchResult(&buf, result, nil, OutputConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No results") && !strings.Contains(out, "0") {
		t.Error("expected output to indicate no results")
	}
}

func TestFormatArticleJSON(t *testing.T) {
	articles := []eutils.Article{
		{
			PMID:     "12345",
			Title:    "Test Article",
			Abstract: "This is a test.",
			Authors: []eutils.Author{
				{LastName: "Smith", ForeName: "John", Initials: "J"},
			},
			Journal:          "Test Journal",
			Year:             "2024",
			DOI:              "10.1234/test",
			MeSHTerms:        []eutils.MeSHTerm{{Descriptor: "Humans", MajorTopic: false}},
			PublicationTypes: []string{"Journal Article"},
			Language:         "eng",
		},
	}

	var buf bytes.Buffer
	err := FormatArticles(&buf, articles, OutputConfig{JSON: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, buf.String())
	}

	if len(parsed) != 1 {
		t.Fatalf("expected 1 article, got %d", len(parsed))
	}
	if parsed[0]["pmid"] != "12345" {
		t.Errorf("expected PMID '12345', got %v", parsed[0]["pmid"])
	}
}

func TestFormatArticlePlain(t *testing.T) {
	articles := []eutils.Article{
		{
			PMID:     "12345",
			Title:    "Test Article Title",
			Abstract: "BACKGROUND: This is a test.\n\nMETHODS: We did things.",
			Authors: []eutils.Author{
				{LastName: "Smith", ForeName: "John", Initials: "J"},
				{LastName: "Doe", ForeName: "Jane", Initials: "J"},
			},
			Journal:       "Test Journal",
			JournalAbbrev: "Test J",
			Volume:        "10",
			Issue:         "2",
			Pages:         "100-110",
			Year:          "2024",
			DOI:           "10.1234/test",
			MeSHTerms: []eutils.MeSHTerm{
				{Descriptor: "Humans", MajorTopic: false},
				{Descriptor: "Test Term", MajorTopic: true},
			},
			PublicationTypes: []string{"Journal Article", "Review"},
			Language:         "eng",
		},
	}

	var buf bytes.Buffer
	err := FormatArticles(&buf, articles, OutputConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "12345") {
		t.Error("expected output to contain PMID")
	}
	if !strings.Contains(out, "Test Article Title") {
		t.Error("expected output to contain title")
	}
	if !strings.Contains(out, "Smith") {
		t.Error("expected output to contain author name")
	}
	if !strings.Contains(out, "10.1234/test") {
		t.Error("expected output to contain DOI")
	}
	if !strings.Contains(out, "Test Journal") || !strings.Contains(out, "2024") {
		t.Error("expected output to contain journal and year")
	}
}

func TestFormatArticleEmptyYear(t *testing.T) {
	articles := []eutils.Article{
		{
			PMID:             "99999",
			Title:            "Article with no year",
			Journal:          "Some Journal",
			Year:             "",
			Volume:           "10",
			PublicationTypes: []string{"Journal Article"},
			Language:         "eng",
		},
	}

	var buf bytes.Buffer
	err := FormatArticles(&buf, articles, OutputConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "()") {
		t.Errorf("output should not contain '()' when year is empty, got:\n%s", out)
	}
	if !strings.Contains(out, "Some Journal") {
		t.Error("expected output to contain journal name")
	}
}

func TestFormatArticleEmpty(t *testing.T) {
	var buf bytes.Buffer
	err := FormatArticles(&buf, []eutils.Article{}, OutputConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No articles") && !strings.Contains(out, "no articles") {
		t.Errorf("expected 'no articles' message, got %q", out)
	}
}

func TestFormatArticles_WithRISAndJSON(t *testing.T) {
	dir := t.TempDir()
	risPath := filepath.Join(dir, "articles.ris")

	articles := []eutils.Article{
		{
			PMID:             "12345",
			Title:            "RIS and JSON",
			Authors:          []eutils.Author{{LastName: "Smith", ForeName: "Jane"}},
			Journal:          "Test Journal",
			Year:             "2026",
			PublicationTypes: []string{"Journal Article"},
			Language:         "eng",
		},
	}

	var buf bytes.Buffer
	err := FormatArticles(&buf, articles, OutputConfig{JSON: true, RISFile: risPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 JSON article, got %d", len(parsed))
	}

	risData, err := os.ReadFile(risPath)
	if err != nil {
		t.Fatalf("failed reading RIS file: %v", err)
	}
	if !strings.Contains(string(risData), "TY  - JOUR") {
		t.Fatalf("expected RIS record in file, got:\n%s", string(risData))
	}
}

func TestFormatLinksJSON(t *testing.T) {
	result := &eutils.LinkResult{
		SourceID: "12345",
		Links: []eutils.LinkItem{
			{ID: "111", Score: 99},
			{ID: "222", Score: 88},
		},
	}

	var buf bytes.Buffer
	err := FormatLinks(&buf, result, "cited-by", OutputConfig{JSON: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestFormatLinksPlain(t *testing.T) {
	result := &eutils.LinkResult{
		SourceID: "12345",
		Links: []eutils.LinkItem{
			{ID: "111"},
			{ID: "222"},
		},
	}

	var buf bytes.Buffer
	err := FormatLinks(&buf, result, "cited-by", OutputConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "111") {
		t.Error("expected output to contain PMID '111'")
	}
	if !strings.Contains(out, "222") {
		t.Error("expected output to contain PMID '222'")
	}
}

func TestFormatLinksEmpty(t *testing.T) {
	result := &eutils.LinkResult{
		SourceID: "12345",
		Links:    []eutils.LinkItem{},
	}

	var buf bytes.Buffer
	err := FormatLinks(&buf, result, "cited-by", OutputConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No") && !strings.Contains(out, "no") {
		t.Errorf("expected 'no results' message, got %q", out)
	}
}
