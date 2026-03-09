package refcheck

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
)

func TestBuildReport_Summary(t *testing.T) {
	results := []VerifiedReference{
		{Parsed: ParsedReference{Index: 1}, Status: StatusVerifiedExact},
		{Parsed: ParsedReference{Index: 2}, Status: StatusVerifiedExact},
		{Parsed: ParsedReference{Index: 3}, Status: StatusVerifiedCorrected},
		{Parsed: ParsedReference{Index: 4}, Status: StatusVerifiedByTitle},
		{Parsed: ParsedReference{Index: 5}, Status: StatusCandidate},
		{Parsed: ParsedReference{Index: 6}, Status: StatusNotInPubMed},
		{Parsed: ParsedReference{Index: 7}, Status: StatusPossiblyFabricated},
	}

	report := BuildReport("test.docx", results, nil)

	if report.Summary.Total != 7 {
		t.Errorf("expected total 7, got %d", report.Summary.Total)
	}
	if report.Summary.VerifiedExact != 2 {
		t.Errorf("expected 2 exact, got %d", report.Summary.VerifiedExact)
	}
	if report.Summary.VerifiedCorrected != 1 {
		t.Errorf("expected 1 corrected, got %d", report.Summary.VerifiedCorrected)
	}
	if report.Summary.PossiblyFabricated != 1 {
		t.Errorf("expected 1 fabricated, got %d", report.Summary.PossiblyFabricated)
	}
}

func TestFormatJSON(t *testing.T) {
	results := []VerifiedReference{
		{
			Parsed:     ParsedReference{Index: 1, Title: "Test article"},
			Status:     StatusVerifiedExact,
			Confidence: 1.0,
			Match:      &eutils.Article{PMID: "12345", Title: "Test article", DOI: "10.1234/test"},
		},
	}

	report := BuildReport("test.docx", results, nil)
	var buf bytes.Buffer
	if err := FormatJSON(&buf, report); err != nil {
		t.Fatal(err)
	}

	// Should be valid JSON.
	var parsed Report
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed.RefCount != 1 {
		t.Errorf("expected ref_count 1, got %d", parsed.RefCount)
	}
}

func TestFormatHuman(t *testing.T) {
	results := []VerifiedReference{
		{
			Parsed:     ParsedReference{Index: 1, Raw: "Bear MF et al. The mGluR theory. Trends Neurosci. 2004."},
			Status:     StatusVerifiedExact,
			Confidence: 1.0,
			Match:      &eutils.Article{PMID: "15219735", Title: "The mGluR theory of fragile X mental retardation.", DOI: "10.1016/j.tins.2004.04.009"},
			QueryTiers: []string{"tier0_pmid"},
		},
		{
			Parsed:     ParsedReference{Index: 2, Raw: "Thompson RJ et al. Novel biomarkers. 2024."},
			Status:     StatusPossiblyFabricated,
			Confidence: 0.0,
			Notes:      "Author publishes in PubMed but has no papers matching this title/topic",
		},
	}

	report := BuildReport("test.docx", results, nil)
	var buf bytes.Buffer
	if err := FormatHuman(&buf, report); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "VERIFIED_EXACT") {
		t.Error("expected VERIFIED_EXACT in output")
	}
	if !strings.Contains(output, "POSSIBLY_FABRICATED") {
		t.Error("expected POSSIBLY_FABRICATED in output")
	}
	if !strings.Contains(output, "15219735") {
		t.Error("expected PMID in output")
	}
}

func TestFormatCSV(t *testing.T) {
	results := []VerifiedReference{
		{
			Parsed:     ParsedReference{Index: 1},
			Status:     StatusVerifiedExact,
			Confidence: 0.98,
			Match:      &eutils.Article{PMID: "12345", DOI: "10.1234/test", Title: "Test title"},
		},
	}

	report := BuildReport("test.docx", results, nil)
	var buf bytes.Buffer
	if err := FormatCSV(&buf, report); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "Index,Status") {
		t.Error("expected CSV header")
	}
	if !strings.Contains(output, "VERIFIED_EXACT") {
		t.Error("expected status in CSV")
	}
}

func TestFormatRIS(t *testing.T) {
	results := []VerifiedReference{
		{
			Parsed: ParsedReference{Index: 1},
			Status: StatusVerifiedExact,
			Match: &eutils.Article{
				PMID:    "15219735",
				Title:   "The mGluR theory of fragile X mental retardation.",
				Authors: []eutils.Author{{LastName: "Bear", ForeName: "Mark F"}},
				Year:    "2004",
				Journal: "Trends in neurosciences",
				Volume:  "27",
				Issue:   "7",
				Pages:   "370-7",
				DOI:     "10.1016/j.tins.2004.04.009",
			},
		},
		{
			// No match — should be skipped in RIS.
			Parsed: ParsedReference{Index: 2},
			Status: StatusNotInPubMed,
		},
	}

	report := BuildReport("test.docx", results, nil)
	var buf bytes.Buffer
	if err := FormatRIS(&buf, report); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "TY  - JOUR") {
		t.Error("expected RIS type tag")
	}
	if !strings.Contains(output, "Bear, Mark F") {
		t.Error("expected author in RIS")
	}
	if !strings.Contains(output, "DO  - 10.1016") {
		t.Error("expected DOI in RIS")
	}
	// Only one entry (second has no match).
	if strings.Count(output, "TY  - JOUR") != 1 {
		t.Errorf("expected exactly 1 RIS entry, got %d", strings.Count(output, "TY  - JOUR"))
	}
}

func TestCsvEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"has,comma", `"has,comma"`},
		{`has"quote`, `"has""quote"`},
		{"has\nnewline", `"has` + "\n" + `newline"`},
	}

	for _, tt := range tests {
		got := csvEscape(tt.input)
		if got != tt.want {
			t.Errorf("csvEscape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
