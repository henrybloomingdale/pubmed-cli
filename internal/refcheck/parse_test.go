package refcheck

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testdataDir returns the absolute path to the testdata directory.
func testdataDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..", "testdata")
}

func TestParseReferences_Numbered(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(), "sample_references.txt"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	refs, err := ParseReferences(string(data))
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 18 {
		t.Fatalf("expected 18 references, got %d", len(refs))
	}

	// Ref 1: Smith JA, Johnson KL, Williams BR, et al. — no PMID
	r1 := refs[0]
	if r1.Index != 1 {
		t.Errorf("ref 1 Index = %d, want 1", r1.Index)
	}
	if r1.Year != "2023" {
		t.Errorf("ref 1 Year = %q, want %q", r1.Year, "2023")
	}
	if r1.DOI != "10.1136/jmg-2022-108796" {
		t.Errorf("ref 1 DOI = %q, want %q", r1.DOI, "10.1136/jmg-2022-108796")
	}
	if r1.PMID != "" {
		t.Errorf("ref 1 PMID = %q, want empty", r1.PMID)
	}
	wantAuthors1 := []string{"Smith", "Johnson", "Williams"}
	if !authorsContain(r1.Authors, wantAuthors1) {
		t.Errorf("ref 1 Authors = %v, want at least %v", r1.Authors, wantAuthors1)
	}
	if !strings.Contains(r1.Title, "Fragile X syndrome") {
		t.Errorf("ref 1 Title = %q, want to contain %q", r1.Title, "Fragile X syndrome")
	}
	if r1.Journal != "J Med Genet" {
		t.Errorf("ref 1 Journal = %q, want %q", r1.Journal, "J Med Genet")
	}

	// Ref 2: Hagerman — PMID 28960184
	r2 := refs[1]
	if r2.PMID != "28960184" {
		t.Errorf("ref 2 PMID = %q, want %q", r2.PMID, "28960184")
	}
	if r2.Year != "2017" {
		t.Errorf("ref 2 Year = %q, want %q", r2.Year, "2017")
	}
	wantAuthors2 := []string{"Hagerman", "Berry-Kravis", "Hazlett"}
	if !authorsContain(r2.Authors, wantAuthors2) {
		t.Errorf("ref 2 Authors = %v, want at least %v", r2.Authors, wantAuthors2)
	}

	// Ref 7: Bear MF, Huber KM, Warren ST — no et al, DOI present, no PMID
	r7 := refs[6]
	if r7.Index != 7 {
		t.Errorf("ref 7 Index = %d, want 7", r7.Index)
	}
	if r7.Year != "2004" {
		t.Errorf("ref 7 Year = %q, want %q", r7.Year, "2004")
	}
	if r7.DOI != "10.1016/j.tins.2004.04.009" {
		t.Errorf("ref 7 DOI = %q, want %q", r7.DOI, "10.1016/j.tins.2004.04.009")
	}
	if r7.PMID != "" {
		t.Errorf("ref 7 PMID = %q, want empty", r7.PMID)
	}
	wantAuthors7 := []string{"Bear", "Huber", "Warren"}
	if !authorsContain(r7.Authors, wantAuthors7) {
		t.Errorf("ref 7 Authors = %v, want exactly %v", r7.Authors, wantAuthors7)
	}
	if len(r7.Authors) != 3 {
		t.Errorf("ref 7 Authors length = %d, want 3", len(r7.Authors))
	}
}

func TestParseReferences_WithPMID(t *testing.T) {
	text := "1. Hagerman RJ, Berry-Kravis E. Fragile X syndrome. Nat Rev Dis Primers. 2017;3:17065. doi:10.1038/nrdp.2017.65. PMID: 28960184"
	refs, err := ParseReferences(text)
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refs))
	}
	if refs[0].PMID != "28960184" {
		t.Errorf("PMID = %q, want %q", refs[0].PMID, "28960184")
	}
	if refs[0].DOI != "10.1038/nrdp.2017.65" {
		t.Errorf("DOI = %q, want %q", refs[0].DOI, "10.1038/nrdp.2017.65")
	}
}

func TestParseReferences_DOIOnly(t *testing.T) {
	text := "1. Berry-Kravis E, Des Portes V, Bhatt A, et al. Mavoglurant in fragile X syndrome: Results of two randomized, double-blind, placebo-controlled trials. Sci Transl Med. 2016;8(321):321ra5. doi:10.1126/scitranslmed.aab4109"
	refs, err := ParseReferences(text)
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1, got %d", len(refs))
	}
	r := refs[0]
	if r.DOI != "10.1126/scitranslmed.aab4109" {
		t.Errorf("DOI = %q, want %q", r.DOI, "10.1126/scitranslmed.aab4109")
	}
	if r.PMID != "" {
		t.Errorf("PMID = %q, want empty", r.PMID)
	}
	if r.Year != "2016" {
		t.Errorf("Year = %q, want %q", r.Year, "2016")
	}
	wantAuthors := []string{"Berry-Kravis", "Des Portes", "Bhatt"}
	if !authorsContain(r.Authors, wantAuthors) {
		t.Errorf("Authors = %v, want at least %v", r.Authors, wantAuthors)
	}
}

func TestParseReferences_NeitherDOINorPMID(t *testing.T) {
	text := "1. Dolen G, Osterweil E, Rao BS, et al. Correction of fragile X syndrome in mice. Neuron. 2007;56(6):955-962."
	refs, err := ParseReferences(text)
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1, got %d", len(refs))
	}
	r := refs[0]
	if r.DOI != "" {
		t.Errorf("DOI = %q, want empty", r.DOI)
	}
	if r.PMID != "" {
		t.Errorf("PMID = %q, want empty", r.PMID)
	}
	if r.Year != "2007" {
		t.Errorf("Year = %q, want %q", r.Year, "2007")
	}
	if !strings.Contains(r.Title, "Correction of fragile X syndrome in mice") {
		t.Errorf("Title = %q, missing expected text", r.Title)
	}
	wantAuthors := []string{"Dolen", "Osterweil", "Rao"}
	if !authorsContain(r.Authors, wantAuthors) {
		t.Errorf("Authors = %v, want at least %v", r.Authors, wantAuthors)
	}
}

func TestParseReferences_MinimalSet(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(), "sample_references_minimal.txt"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	refs, err := ParseReferences(string(data))
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 4 {
		t.Fatalf("expected 4 references, got %d", len(refs))
	}
	// Spot check first and last
	if refs[0].Index != 1 {
		t.Errorf("first ref Index = %d, want 1", refs[0].Index)
	}
	if refs[3].Index != 4 {
		t.Errorf("last ref Index = %d, want 4", refs[3].Index)
	}
	if refs[0].PMID != "28960184" {
		t.Errorf("ref 1 PMID = %q, want %q", refs[0].PMID, "28960184")
	}
	if refs[2].PMID != "1710175" {
		t.Errorf("ref 3 PMID = %q, want %q", refs[2].PMID, "1710175")
	}
	if refs[3].PMID != "25287460" {
		t.Errorf("ref 4 PMID = %q, want %q", refs[3].PMID, "25287460")
	}
}

func TestParseReferences_EmptyInput(t *testing.T) {
	refs, err := ParseReferences("")
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected 0 references, got %d", len(refs))
	}

	refs2, err := ParseReferences("   \n\n  \n")
	if err != nil {
		t.Fatalf("ParseReferences whitespace: %v", err)
	}
	if len(refs2) != 0 {
		t.Errorf("expected 0 references for whitespace, got %d", len(refs2))
	}
}

func TestParseReferences_APAStyle(t *testing.T) {
	text := `Bear, M. F., Huber, K. M., & Warren, S. T. (2004). The mGluR theory of fragile X mental retardation. Trends in Neurosciences, 27(7), 370-377.

Hagerman, R. J., & Berry-Kravis, E. (2017). Fragile X syndrome. Nature Reviews Disease Primers, 3, 17065. doi:10.1038/nrdp.2017.65`
	refs, err := ParseReferences(text)
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2, got %d", len(refs))
	}
	r1 := refs[0]
	if r1.Year != "2004" {
		t.Errorf("ref 1 Year = %q, want %q", r1.Year, "2004")
	}
	wantAuthors := []string{"Bear", "Huber", "Warren"}
	if !authorsContain(r1.Authors, wantAuthors) {
		t.Errorf("ref 1 Authors = %v, want at least %v", r1.Authors, wantAuthors)
	}
	if !strings.Contains(r1.Title, "mGluR theory") {
		t.Errorf("ref 1 Title = %q, want to contain %q", r1.Title, "mGluR theory")
	}

	r2 := refs[1]
	if r2.Year != "2017" {
		t.Errorf("ref 2 Year = %q, want %q", r2.Year, "2017")
	}
	if r2.DOI != "10.1038/nrdp.2017.65" {
		t.Errorf("ref 2 DOI = %q, want %q", r2.DOI, "10.1038/nrdp.2017.65")
	}
}

func TestParseReferences_EtAl(t *testing.T) {
	text := "1. Smith JA, Johnson KL, Williams BR, et al. Some title here. J Test. 2020;1(1):1-10."
	refs, err := ParseReferences(text)
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1, got %d", len(refs))
	}
	wantAuthors := []string{"Smith", "Johnson", "Williams"}
	if !authorsContain(refs[0].Authors, wantAuthors) {
		t.Errorf("Authors = %v, want at least %v", refs[0].Authors, wantAuthors)
	}
	// "et al" should not appear as an author
	for _, a := range refs[0].Authors {
		lower := strings.ToLower(a)
		if lower == "et" || lower == "al" || lower == "et al" {
			t.Errorf("Authors contains %q, should not include et al", a)
		}
	}
}

func TestParseReferences_TitleWithColonAndQuestion(t *testing.T) {
	text := "1. Author AB. Is this a question: a systematic review? Some Journal. 2021;5(2):100-110."
	refs, err := ParseReferences(text)
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1, got %d", len(refs))
	}
	if !strings.Contains(refs[0].Title, "question") {
		t.Errorf("Title = %q, want to contain 'question'", refs[0].Title)
	}
	if refs[0].Year != "2021" {
		t.Errorf("Year = %q, want %q", refs[0].Year, "2021")
	}
}

func TestParseReferences_WithPMCID(t *testing.T) {
	text := "1. Author AB. Some article title. J Test. 2019;10(1):50-60. PMID: 12345678. PMCID: PMC6543210"
	refs, err := ParseReferences(text)
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1, got %d", len(refs))
	}
	if refs[0].PMID != "12345678" {
		t.Errorf("PMID = %q, want %q", refs[0].PMID, "12345678")
	}
}

func TestParseReferences_BracketNumbered(t *testing.T) {
	text := `[1] Smith JA, Johnson KL. Some title. J Test. 2020;1(1):1-10.
[2] Bear MF, Huber KM. Another title. Trends Neurosci. 2004;27(7):370-377.`
	refs, err := ParseReferences(text)
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2, got %d", len(refs))
	}
	if refs[0].Index != 1 {
		t.Errorf("ref 1 Index = %d, want 1", refs[0].Index)
	}
	if refs[1].Index != 2 {
		t.Errorf("ref 2 Index = %d, want 2", refs[1].Index)
	}
}

func TestExtractDOI(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"doi:10.1136/jmg-2022-108796", "10.1136/jmg-2022-108796"},
		{"doi: 10.1038/nrdp.2017.65.", "10.1038/nrdp.2017.65"},
		{"DOI:10.1542/peds.2016-1159F", "10.1542/peds.2016-1159F"},
		{"doi:10.1016/j.tins.2004.04.009", "10.1016/j.tins.2004.04.009"},
		{"https://doi.org/10.1038/s41380-024-02445-8", "10.1038/s41380-024-02445-8"},
		{"no doi here", ""},
		{"doi:10.1126/scitranslmed.aab4109.", "10.1126/scitranslmed.aab4109"},
	}
	for _, tt := range tests {
		got := ExtractDOI(tt.input)
		if got != tt.want {
			t.Errorf("ExtractDOI(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractPMID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"PMID: 28960184", "28960184"},
		{"PMID:28960184", "28960184"},
		{"PMID 1710175", "1710175"},
		{"some text PMID: 25287460 more text", "25287460"},
		{"no pmid here", ""},
		{"PMID: 28960184. PMCID: PMC6543210", "28960184"},
	}
	for _, tt := range tests {
		got := ExtractPMID(tt.input)
		if got != tt.want {
			t.Errorf("ExtractPMID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseReferences_VolumeIssuePages(t *testing.T) {
	text := "1. Smith JA. Some title. J Test. 2020;60(3):145-158. doi:10.1234/test"
	refs, err := ParseReferences(text)
	if err != nil {
		t.Fatalf("ParseReferences: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1, got %d", len(refs))
	}
	r := refs[0]
	if r.Volume != "60" {
		t.Errorf("Volume = %q, want %q", r.Volume, "60")
	}
	if r.Issue != "3" {
		t.Errorf("Issue = %q, want %q", r.Issue, "3")
	}
	if r.Pages != "145-158" {
		t.Errorf("Pages = %q, want %q", r.Pages, "145-158")
	}
}

// authorsContain checks that all expected author last names appear in the parsed authors.
func authorsContain(got []string, want []string) bool {
	set := make(map[string]bool)
	for _, a := range got {
		set[a] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}
