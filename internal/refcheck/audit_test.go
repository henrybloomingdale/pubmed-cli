package refcheck

import (
	"testing"
)

func TestAuditCitations_NumberedRefs(t *testing.T) {
	body := `Introduction
Fragile X syndrome is the most common inherited cause of intellectual disability [1].
Recent studies have confirmed the mGluR theory [2] and expanded on sensory processing [3].
The combined evidence from multiple trials [1,2,3] supports targeted intervention.
Some authors have proposed biomarker approaches [5].`

	refs := []ParsedReference{
		{Index: 1, Authors: []string{"Hagerman"}, Year: "2017", Title: "Fragile X syndrome"},
		{Index: 2, Authors: []string{"Bear"}, Year: "2004", Title: "mGluR theory"},
		{Index: 3, Authors: []string{"Baranek"}, Year: "2008", Title: "Sensory processing"},
	}

	result := AuditCitations(body, refs)

	// All 3 should be cited.
	if len(result.Uncited) != 0 {
		t.Errorf("expected no uncited refs, got %v", result.Uncited)
	}

	// Check ref [1] is found in multiple places.
	found1 := false
	for _, c := range result.Citations {
		if c.RefIndex == 1 {
			found1 = true
			if c.Count < 2 {
				t.Errorf("expected ref [1] cited at least 2 times, got %d", c.Count)
			}
		}
	}
	if !found1 {
		t.Error("expected to find citation for ref [1]")
	}

	// [5] is an orphan (not in reference list of 3).
	if len(result.OrphanMarkers) == 0 {
		t.Error("expected orphan marker [5]")
	}
}

func TestAuditCitations_AuthorYear(t *testing.T) {
	body := `The mGluR theory (Bear et al., 2004) fundamentally changed our understanding.
Bear et al. (2004) demonstrated that mGluR5 antagonism could rescue phenotypes.
Hagerman (2017) provided a comprehensive review.`

	refs := []ParsedReference{
		{Index: 1, Authors: []string{"Bear"}, Year: "2004", Title: "mGluR theory"},
		{Index: 2, Authors: []string{"Hagerman"}, Year: "2017", Title: "Fragile X syndrome"},
	}

	result := AuditCitations(body, refs)

	if len(result.Uncited) != 0 {
		t.Errorf("expected no uncited refs, got %v", result.Uncited)
	}

	// Bear should be found twice.
	for _, c := range result.Citations {
		if c.RefIndex == 1 && c.Count < 2 {
			t.Errorf("expected Bear cited at least 2 times, got %d", c.Count)
		}
	}
}

func TestAuditCitations_UncitedRef(t *testing.T) {
	body := `Only one reference is mentioned here [1].`

	refs := []ParsedReference{
		{Index: 1, Authors: []string{"Bear"}, Year: "2004"},
		{Index: 2, Authors: []string{"Hagerman"}, Year: "2017"},
		{Index: 3, Authors: []string{"Verkerk"}, Year: "1991"},
	}

	result := AuditCitations(body, refs)

	if len(result.Uncited) != 2 {
		t.Errorf("expected 2 uncited refs, got %d: %v", len(result.Uncited), result.Uncited)
	}
}

func TestAuditCitations_EmptyBody(t *testing.T) {
	refs := []ParsedReference{
		{Index: 1, Authors: []string{"Bear"}, Year: "2004"},
	}

	result := AuditCitations("", refs)

	if len(result.Uncited) != 1 {
		t.Errorf("expected 1 uncited ref, got %d", len(result.Uncited))
	}
	if len(result.OrphanMarkers) != 0 {
		t.Errorf("expected no orphans, got %v", result.OrphanMarkers)
	}
}

func TestAuditCitations_NoRefs(t *testing.T) {
	body := "Some text with a citation [1]."
	result := AuditCitations(body, nil)

	if len(result.Citations) != 0 {
		t.Errorf("expected no citations, got %d", len(result.Citations))
	}
}

func TestBuildNumberedPattern(t *testing.T) {
	re := buildNumberedPattern(2)
	if re == nil {
		t.Fatal("expected non-nil pattern")
	}

	tests := []struct {
		input string
		match bool
	}{
		{"[2]", true},
		{"[1,2,3]", true},
		{"[1-5]", false}, // 2 is in range 1-5 but our regex looks for literal "2"
		{"[12]", false},  // 12 != 2 (word boundary)
		{"[22]", false},
		{"text", false},
	}

	for _, tt := range tests {
		got := re.MatchString(tt.input)
		if got != tt.match {
			t.Errorf("pattern for [2] on %q: got %v, want %v", tt.input, got, tt.match)
		}
	}
}

func TestBuildAuthorYearPatterns(t *testing.T) {
	patterns := buildAuthorYearPatterns("Bear", "2004")
	if len(patterns) == 0 {
		t.Fatal("expected at least one pattern")
	}

	shouldMatch := []string{
		"(Bear et al., 2004)",
		"(Bear et al. 2004)",
		"Bear et al. (2004)",
		"(Bear, 2004)",
		"Bear (2004)",
	}

	for _, s := range shouldMatch {
		matched := false
		for _, p := range patterns {
			if p.MatchString(s) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("expected %q to match an author-year pattern", s)
		}
	}
}
