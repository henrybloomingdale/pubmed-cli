package main

import (
	"testing"
)

func TestBuildQuery_Basic(t *testing.T) {
	flagType = ""
	flagYear = ""

	got := buildQuery([]string{"fragile", "x", "syndrome"})
	expected := "fragile x syndrome"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_TypeReview(t *testing.T) {
	flagType = "review"
	flagYear = ""
	defer func() { flagType = "" }()

	got := buildQuery([]string{"asthma"})
	expected := `asthma AND "review"[pt]`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_TypeTrial(t *testing.T) {
	flagType = "trial"
	flagYear = ""
	defer func() { flagType = "" }()

	got := buildQuery([]string{"asthma"})
	expected := `asthma AND "clinical trial"[pt]`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_TypeRandomized(t *testing.T) {
	flagType = "randomized"
	flagYear = ""
	defer func() { flagType = "" }()

	got := buildQuery([]string{"asthma"})
	expected := `asthma AND "randomized controlled trial"[pt]`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_TypeMetaAnalysis(t *testing.T) {
	flagType = "meta-analysis"
	flagYear = ""
	defer func() { flagType = "" }()

	got := buildQuery([]string{"asthma"})
	expected := `asthma AND "meta-analysis"[pt]`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_TypeCustom(t *testing.T) {
	flagType = "editorial"
	flagYear = ""
	defer func() { flagType = "" }()

	got := buildQuery([]string{"asthma"})
	expected := `asthma AND "editorial"[pt]`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_YearRange(t *testing.T) {
	flagType = ""
	flagYear = "2020-2025"
	defer func() { flagYear = "" }()

	got := buildQuery([]string{"asthma"})
	expected := "asthma AND 2020:2025[pdat]"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_SingleYear(t *testing.T) {
	flagType = ""
	flagYear = "2024"
	defer func() { flagYear = "" }()

	got := buildQuery([]string{"asthma"})
	expected := "asthma AND 2024[pdat]"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestBuildQuery_TypeAndYear(t *testing.T) {
	flagType = "trial"
	flagYear = "2020-2025"
	defer func() {
		flagType = ""
		flagYear = ""
	}()

	got := buildQuery([]string{"asthma"})
	expected := `asthma AND "clinical trial"[pt] AND 2020:2025[pdat]`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestBuildQuery_MultiWordTypesAreQuoted verifies that multi-word publication
// types are properly quoted so PubMed applies [pt] to the full phrase.
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
			flagType = tt.typeFlag
			flagYear = ""
			defer func() { flagType = "" }()

			got := buildQuery([]string{"test"})
			if !contains(got, tt.want) {
				t.Errorf("query %q does not contain properly quoted type %q", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
