package refcheck

import (
	"math"
	"testing"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
)

func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestScoreMatch_PerfectDOI(t *testing.T) {
	ref := ParsedReference{
		DOI:   "10.1001/jama.2020.1234",
		Title: "Some Article Title",
	}
	art := eutils.Article{
		DOI:   "10.1001/jama.2020.1234",
		Title: "Some Article Title",
	}
	score := ScoreMatch(ref, art)
	if score.DOI != 1.0 {
		t.Errorf("DOI score = %f, want 1.0", score.DOI)
	}
	if score.Total < 0.95 {
		t.Errorf("Total = %f, want near 1.0 for perfect DOI match", score.Total)
	}
}

func TestScoreMatch_PerfectDOI_WithURLPrefix(t *testing.T) {
	ref := ParsedReference{
		DOI: "https://doi.org/10.1001/jama.2020.1234",
	}
	art := eutils.Article{
		DOI: "10.1001/JAMA.2020.1234",
	}
	score := ScoreMatch(ref, art)
	if score.DOI != 1.0 {
		t.Errorf("DOI score = %f, want 1.0 (URL prefix + case normalization)", score.DOI)
	}
	if score.Total != 1.0 {
		t.Errorf("Total = %f, want 1.0 for DOI fast path", score.Total)
	}
}

func TestScoreMatch_PerfectPMID(t *testing.T) {
	ref := ParsedReference{
		PMID: "12345678",
	}
	art := eutils.Article{
		PMID: "12345678",
	}
	score := ScoreMatch(ref, art)
	if score.PMID != 1.0 {
		t.Errorf("PMID score = %f, want 1.0", score.PMID)
	}
	if score.Total != 1.0 {
		t.Errorf("Total = %f, want 1.0 for PMID fast path", score.Total)
	}
}

func TestScoreMatch_TitleAuthorsYear(t *testing.T) {
	ref := ParsedReference{
		Title:   "Effectiveness of Cognitive Behavioral Therapy for Insomnia",
		Authors: []string{"Smith", "Johnson", "Williams"},
		Year:    "2019",
		Journal: "Sleep Medicine Reviews",
	}
	art := eutils.Article{
		Title:   "Effectiveness of Cognitive Behavioral Therapy for Insomnia",
		Authors: []eutils.Author{
			{LastName: "Smith", ForeName: "John"},
			{LastName: "Johnson", ForeName: "Alice"},
			{LastName: "Williams", ForeName: "Bob"},
		},
		Year:    "2019",
		Journal: "Sleep Medicine Reviews",
	}
	score := ScoreMatch(ref, art)
	if score.Title < 0.9 {
		t.Errorf("Title = %f, want >= 0.9", score.Title)
	}
	if score.AuthorHit < 0.9 {
		t.Errorf("AuthorHit = %f, want >= 0.9", score.AuthorHit)
	}
	if score.Year != 1.0 {
		t.Errorf("Year = %f, want 1.0", score.Year)
	}
	if score.Total < 0.85 {
		t.Errorf("Total = %f, want >= 0.85 for strong title+author+year match", score.Total)
	}
}

func TestScoreMatch_PartialAuthorMatch(t *testing.T) {
	ref := ParsedReference{
		Authors: []string{"Smith", "Johnson", "Williams"},
	}
	art := eutils.Article{
		Authors: []eutils.Author{
			{LastName: "Smith", ForeName: "John"},
			{LastName: "Johnson", ForeName: "Alice"},
			{LastName: "Brown", ForeName: "Charlie"},
			{LastName: "Davis", ForeName: "Diana"},
			{LastName: "Lee", ForeName: "Evan"},
			{LastName: "Martinez", ForeName: "Fiona"},
			{LastName: "Taylor", ForeName: "George"},
			{LastName: "Anderson", ForeName: "Helen"},
			{LastName: "Thomas", ForeName: "Ian"},
			{LastName: "White", ForeName: "Julia"},
		},
	}
	score := ScoreMatch(ref, art)
	// 2 out of 3 ref authors found (Smith, Johnson)
	expected := 2.0 / 3.0
	if !approxEqual(score.AuthorHit, expected, 0.01) {
		t.Errorf("AuthorHit = %f, want ~%f (2 of 3 ref authors matched)", score.AuthorHit, expected)
	}
}

func TestScoreMatch_YearOffByOne(t *testing.T) {
	ref := ParsedReference{
		Year: "2017",
	}
	art := eutils.Article{
		Year: "2018",
	}
	score := ScoreMatch(ref, art)
	if score.Year != 0.5 {
		t.Errorf("Year = %f, want 0.5 for off-by-one", score.Year)
	}
}

func TestScoreMatch_YearOffByTwo(t *testing.T) {
	ref := ParsedReference{
		Year: "2017",
	}
	art := eutils.Article{
		Year: "2019",
	}
	score := ScoreMatch(ref, art)
	if score.Year != 0.0 {
		t.Errorf("Year = %f, want 0.0 for off-by-two+", score.Year)
	}
}

func TestScoreMatch_FuzzyTitleMatch(t *testing.T) {
	ref := ParsedReference{
		Title: "the Effectiveness of Cognitive Behavioral Therapy for Insomnia.",
	}
	art := eutils.Article{
		Title: "Effectiveness of Cognitive Behavioral Therapy for Insomnia",
	}
	score := ScoreMatch(ref, art)
	if score.Title < 0.8 {
		t.Errorf("Title = %f, want >= 0.8 for fuzzy title match (case, 'the', period)", score.Title)
	}
}

func TestScoreMatch_NoMatchAtAll(t *testing.T) {
	ref := ParsedReference{
		Title:   "Quantum Entanglement in Photonic Crystals",
		Authors: []string{"Einstein", "Bohr"},
		Year:    "1935",
		Journal: "Physical Review",
	}
	art := eutils.Article{
		Title:   "Effectiveness of Cognitive Behavioral Therapy for Insomnia",
		Authors: []eutils.Author{
			{LastName: "Smith", ForeName: "John"},
			{LastName: "Johnson", ForeName: "Alice"},
		},
		Year:    "2019",
		Journal: "Sleep Medicine Reviews",
	}
	score := ScoreMatch(ref, art)
	if score.Total > 0.15 {
		t.Errorf("Total = %f, want near 0.0 for no match", score.Total)
	}
	if score.DOI != 0.0 {
		t.Errorf("DOI = %f, want 0.0", score.DOI)
	}
	if score.PMID != 0.0 {
		t.Errorf("PMID = %f, want 0.0", score.PMID)
	}
}

func TestScoreMatch_MixedSignals(t *testing.T) {
	// DOI doesn't match but title and authors do
	ref := ParsedReference{
		DOI:     "10.9999/wrong.doi",
		Title:   "Effectiveness of Cognitive Behavioral Therapy for Insomnia",
		Authors: []string{"Smith", "Johnson"},
		Year:    "2019",
	}
	art := eutils.Article{
		DOI:     "10.1001/jama.2020.1234",
		Title:   "Effectiveness of Cognitive Behavioral Therapy for Insomnia",
		Authors: []eutils.Author{
			{LastName: "Smith", ForeName: "John"},
			{LastName: "Johnson", ForeName: "Alice"},
		},
		Year: "2019",
	}
	score := ScoreMatch(ref, art)
	if score.DOI != 0.0 {
		t.Errorf("DOI = %f, want 0.0 (different DOIs)", score.DOI)
	}
	if score.Total < 0.7 {
		t.Errorf("Total = %f, want >= 0.7 (title+author+year match despite DOI mismatch)", score.Total)
	}
	// Not fast-path since DOI didn't match
	if score.Total >= 1.0 {
		t.Errorf("Total = %f, want < 1.0 (no fast path)", score.Total)
	}
}

func TestScoreMatch_EmptyReferenceFields(t *testing.T) {
	ref := ParsedReference{} // everything empty
	art := eutils.Article{
		DOI:   "10.1001/jama.2020.1234",
		PMID:  "12345678",
		Title: "Some Title",
		Authors: []eutils.Author{
			{LastName: "Smith", ForeName: "John"},
		},
		Year:    "2020",
		Journal: "JAMA",
	}
	score := ScoreMatch(ref, art)
	// Should not crash, scores should be 0
	if score.DOI != 0.0 {
		t.Errorf("DOI = %f, want 0.0 for empty ref", score.DOI)
	}
	if score.PMID != 0.0 {
		t.Errorf("PMID = %f, want 0.0 for empty ref", score.PMID)
	}
	if score.AuthorHit != 0.0 {
		t.Errorf("AuthorHit = %f, want 0.0 for empty ref authors", score.AuthorHit)
	}
}

// Helper function tests

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello, World!", "hello world"},
		{"  The  Quick   Brown  Fox ", "the quick brown fox"},
		{"Title: With-Punctuation (and) Stuff.", "title withpunctuation and stuff"},
		{"", ""},
	}
	for _, tt := range tests {
		got := NormalizeTitle(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTokenJaccard(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
		tol  float64
	}{
		{"hello world", "hello world", 1.0, 0.001},
		{"hello world", "goodbye world", 0.333, 0.05},
		{"", "", 0.0, 0.001},
		{"abc", "", 0.0, 0.001},
		{"the quick brown fox", "quick brown fox jumps", 0.6, 0.05},
	}
	for _, tt := range tests {
		got := TokenJaccard(tt.a, tt.b)
		if !approxEqual(got, tt.want, tt.tol) {
			t.Errorf("TokenJaccard(%q, %q) = %f, want ~%f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNormalizeDOI(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"10.1001/jama.2020.1234", "10.1001/jama.2020.1234"},
		{"https://doi.org/10.1001/JAMA.2020.1234", "10.1001/jama.2020.1234"},
		{"http://doi.org/10.1001/jama.2020.1234", "10.1001/jama.2020.1234"},
		{"https://dx.doi.org/10.1001/JAMA.2020.1234", "10.1001/jama.2020.1234"},
		{"", ""},
	}
	for _, tt := range tests {
		got := NormalizeDOI(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeDOI(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
