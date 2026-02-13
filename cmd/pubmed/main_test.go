package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func resetGlobalFlags() {
	flagType = ""
	flagYear = ""
	flagSort = ""
	flagRIS = ""
	flagLimit = 20
}

func TestBuildQuery_Basic(t *testing.T) {
	resetGlobalFlags()

	got := buildQuery([]string{"fragile", "x", "syndrome"})
	expected := "fragile x syndrome"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_TypeReview(t *testing.T) {
	resetGlobalFlags()
	flagType = "review"

	got := buildQuery([]string{"asthma"})
	expected := `asthma AND "review"[pt]`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_TypeTrial(t *testing.T) {
	resetGlobalFlags()
	flagType = "trial"

	got := buildQuery([]string{"asthma"})
	expected := `asthma AND "clinical trial"[pt]`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_TypeRandomized(t *testing.T) {
	resetGlobalFlags()
	flagType = "randomized"

	got := buildQuery([]string{"asthma"})
	expected := `asthma AND "randomized controlled trial"[pt]`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_TypeMetaAnalysis(t *testing.T) {
	resetGlobalFlags()
	flagType = "meta-analysis"

	got := buildQuery([]string{"asthma"})
	expected := `asthma AND "meta-analysis"[pt]`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_TypeCustom(t *testing.T) {
	resetGlobalFlags()
	flagType = "editorial"

	got := buildQuery([]string{"asthma"})
	expected := `asthma AND "editorial"[pt]`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_MultiWordTypesAreQuoted(t *testing.T) {
	tests := []struct {
		typeFlag string
		want     string
	}{
		{"trial", `"clinical trial"[pt]`},
		{"randomized", `"randomized controlled trial"[pt]`},
		{"case-report", `"case reports"[pt]`},
	}

	for _, tt := range tests {
		t.Run(tt.typeFlag, func(t *testing.T) {
			resetGlobalFlags()
			flagType = tt.typeFlag

			got := buildQuery([]string{"test"})
			if !strings.Contains(got, tt.want) {
				t.Errorf("query %q does not contain properly quoted type %q", got, tt.want)
			}
		})
	}
}

func TestParseYearRange(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantMin string
		wantMax string
		wantErr bool
	}{
		{name: "single", in: "2024", wantMin: "2024", wantMax: "2024"},
		{name: "range", in: "2020-2025", wantMin: "2020", wantMax: "2025"},
		{name: "desc range", in: "2025-2020", wantErr: true},
		{name: "invalid format", in: "20-2025", wantErr: true},
		{name: "non numeric", in: "abcd-2025", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minDate, maxDate, err := parseYearRange(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.in, err)
			}
			if minDate != tt.wantMin || maxDate != tt.wantMax {
				t.Fatalf("expected %s-%s, got %s-%s", tt.wantMin, tt.wantMax, minDate, maxDate)
			}
		})
	}
}

func TestValidateGlobalFlags(t *testing.T) {
	resetGlobalFlags()
	flagLimit = 0
	if err := validateGlobalFlags(&cobra.Command{Use: "search"}); err == nil {
		t.Fatal("expected error for non-positive limit")
	}

	resetGlobalFlags()
	flagSort = "newest"
	if err := validateGlobalFlags(&cobra.Command{Use: "search"}); err == nil {
		t.Fatal("expected error for invalid sort")
	}

	resetGlobalFlags()
	flagYear = "2025-2020"
	if err := validateGlobalFlags(&cobra.Command{Use: "search"}); err == nil {
		t.Fatal("expected error for descending year range")
	}

	resetGlobalFlags()
	flagLimit = 5
	flagSort = "date"
	flagYear = "2024"
	if err := validateGlobalFlags(&cobra.Command{Use: "fetch"}); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateGlobalFlags_RISScope(t *testing.T) {
	resetGlobalFlags()
	flagRIS = "/tmp/out.ris"
	if err := validateGlobalFlags(&cobra.Command{Use: "search"}); err == nil {
		t.Fatal("expected --ris to be rejected for search")
	}

	resetGlobalFlags()
	flagRIS = "/tmp/out.ris"
	if err := validateGlobalFlags(&cobra.Command{Use: "mesh"}); err == nil {
		t.Fatal("expected --ris to be rejected for mesh")
	}

	resetGlobalFlags()
	flagRIS = "/tmp/out.ris"
	if err := validateGlobalFlags(&cobra.Command{Use: "fetch"}); err != nil {
		t.Fatalf("expected --ris to be accepted for fetch, got: %v", err)
	}
}

func TestNormalizePMIDArgs(t *testing.T) {
	pmids, err := normalizePMIDArgs([]string{"38000001, 38000002", "38000003"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"38000001", "38000002", "38000003"}
	if len(pmids) != len(expected) {
		t.Fatalf("expected %d PMIDs, got %d", len(expected), len(pmids))
	}
	for i := range expected {
		if pmids[i] != expected[i] {
			t.Fatalf("expected PMID[%d]=%s, got %s", i, expected[i], pmids[i])
		}
	}

	if _, err := normalizePMIDArgs([]string{"abc123"}); err == nil {
		t.Fatal("expected invalid PMID error")
	}
}
