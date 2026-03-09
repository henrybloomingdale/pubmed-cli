package refcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
	"github.com/henrybloomingdale/pubmed-cli/internal/ncbi"
)

// newTestResolver creates a Resolver backed by a mock HTTP server.
// The handler function receives requests and can serve search/fetch responses.
func newTestResolver(t *testing.T, handler http.HandlerFunc) (*Resolver, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	base := ncbi.NewBaseClient(ncbi.WithBaseURL(srv.URL))
	client := eutils.NewClientWithBase(base)
	return NewResolver(client), srv
}

// bearArticleXML is a sample EFetch XML response for PMID 15219735.
const bearArticleXML = `<?xml version="1.0"?>
<!DOCTYPE PubmedArticleSet PUBLIC "-//NLM//DTD PubMedArticle, 1st January 2024//EN" "https://dtd.nlm.nih.gov/ncbi/pubmed/out/pubmed_240101.dtd">
<PubmedArticleSet>
  <PubmedArticle>
    <MedlineCitation>
      <PMID>15219735</PMID>
      <Article>
        <Journal>
          <ISOAbbreviation>Trends Neurosci</ISOAbbreviation>
          <JournalIssue>
            <Volume>27</Volume>
            <Issue>7</Issue>
            <PubDate><Year>2004</Year><Month>Jul</Month></PubDate>
          </JournalIssue>
          <Title>Trends in neurosciences</Title>
        </Journal>
        <ArticleTitle>The mGluR theory of fragile X mental retardation.</ArticleTitle>
        <Pagination><MedlinePgn>370-7</MedlinePgn></Pagination>
        <Abstract><AbstractText>Fragile X syndrome is the most common inherited form of mental retardation.</AbstractText></Abstract>
        <AuthorList>
          <Author><LastName>Bear</LastName><ForeName>Mark F</ForeName><Initials>MF</Initials></Author>
          <Author><LastName>Huber</LastName><ForeName>Kimberly M</ForeName><Initials>KM</Initials></Author>
          <Author><LastName>Warren</LastName><ForeName>Stephen T</ForeName><Initials>ST</Initials></Author>
        </AuthorList>
        <ELocationID EIdType="doi">10.1016/j.tins.2004.04.009</ELocationID>
      </Article>
    </MedlineCitation>
    <PubmedData>
      <ArticleIdList>
        <ArticleId IdType="pubmed">15219735</ArticleId>
        <ArticleId IdType="doi">10.1016/j.tins.2004.04.009</ArticleId>
      </ArticleIdList>
    </PubmedData>
  </PubmedArticle>
</PubmedArticleSet>`

func TestResolve_Tier0_PMID(t *testing.T) {
	resolver, srv := newTestResolver(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "efetch") {
			w.Header().Set("Content-Type", "text/xml")
			fmt.Fprint(w, bearArticleXML)
			return
		}
		http.NotFound(w, r)
	})
	defer srv.Close()

	ref := ParsedReference{
		Index:   1,
		PMID:    "15219735",
		Authors: []string{"Bear", "Huber", "Warren"},
		Year:    "2004",
		Title:   "The mGluR theory of fragile X mental retardation.",
		DOI:     "10.1016/j.tins.2004.04.009",
	}

	result := resolver.Resolve(context.Background(), ref)

	if result.Status != StatusVerifiedExact {
		t.Errorf("expected VERIFIED_EXACT, got %s", result.Status)
	}
	if result.Match == nil {
		t.Fatal("expected match, got nil")
	}
	if result.Match.PMID != "15219735" {
		t.Errorf("expected PMID 15219735, got %s", result.Match.PMID)
	}
	if result.Confidence < 0.95 {
		t.Errorf("expected confidence >= 0.95, got %f", result.Confidence)
	}
	if len(result.QueryTiers) == 0 || result.QueryTiers[0] != "tier0_pmid" {
		t.Errorf("expected tier0_pmid, got %v", result.QueryTiers)
	}
}

func TestResolve_Tier0_DOI(t *testing.T) {
	resolver, srv := newTestResolver(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "esearch") {
			// Return a search result with one PMID.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"esearchresult": map[string]interface{}{
					"count":            "1",
					"idlist":           []string{"15219735"},
					"querytranslation": "",
				},
			})
			return
		}
		if strings.Contains(r.URL.Path, "efetch") {
			w.Header().Set("Content-Type", "text/xml")
			fmt.Fprint(w, bearArticleXML)
			return
		}
		http.NotFound(w, r)
	})
	defer srv.Close()

	ref := ParsedReference{
		Index:   1,
		Authors: []string{"Bear", "Huber", "Warren"},
		Year:    "2004",
		Title:   "The mGluR theory of fragile X mental retardation.",
		DOI:     "10.1016/j.tins.2004.04.009",
	}

	result := resolver.Resolve(context.Background(), ref)

	if result.Status != StatusVerifiedExact {
		t.Errorf("expected VERIFIED_EXACT, got %s", result.Status)
	}
	if result.Match == nil {
		t.Fatal("expected match, got nil")
	}
}

func TestResolve_Tier1_Title(t *testing.T) {
	resolver, srv := newTestResolver(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "esearch") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"esearchresult": map[string]interface{}{
					"count":  "1",
					"idlist": []string{"15219735"},
				},
			})
			return
		}
		if strings.Contains(r.URL.Path, "efetch") {
			w.Header().Set("Content-Type", "text/xml")
			fmt.Fprint(w, bearArticleXML)
			return
		}
		http.NotFound(w, r)
	})
	defer srv.Close()

	// No PMID or DOI — just title + authors + year.
	ref := ParsedReference{
		Index:   1,
		Authors: []string{"Bear", "Huber", "Warren"},
		Year:    "2004",
		Title:   "The mGluR theory of fragile X mental retardation",
	}

	result := resolver.Resolve(context.Background(), ref)

	if result.Status != StatusVerifiedExact && result.Status != StatusVerifiedByTitle {
		t.Errorf("expected VERIFIED_EXACT or VERIFIED_BY_TITLE, got %s", result.Status)
	}
	if result.Match == nil {
		t.Fatal("expected match, got nil")
	}
}

func TestResolve_NotInPubMed(t *testing.T) {
	// Server returns empty results for everything.
	resolver, srv := newTestResolver(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "esearch") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"esearchresult": map[string]interface{}{
					"count":  "0",
					"idlist": []string{},
				},
			})
			return
		}
		if strings.Contains(r.URL.Path, "efetch") {
			w.Header().Set("Content-Type", "text/xml")
			fmt.Fprint(w, `<?xml version="1.0"?><PubmedArticleSet></PubmedArticleSet>`)
			return
		}
		http.NotFound(w, r)
	})
	defer srv.Close()

	ref := ParsedReference{
		Index:   14,
		Authors: []string{"Thompson", "Nguyen", "Williams"},
		Year:    "2024",
		Title:   "Novel biomarkers for treatment response monitoring in fragile X syndrome",
		DOI:     "10.1038/s41380-024-02445-8",
	}

	result := resolver.Resolve(context.Background(), ref)

	if result.Status != StatusNotInPubMed {
		t.Errorf("expected NOT_IN_PUBMED, got %s", result.Status)
	}
	if result.Match != nil {
		t.Errorf("expected no match, got PMID %s", result.Match.PMID)
	}
}

func TestResolve_ContextCancelled(t *testing.T) {
	resolver, srv := newTestResolver(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	refs := []ParsedReference{
		{Index: 1, Title: "Test Article"},
		{Index: 2, Title: "Another Article"},
	}

	results := resolver.ResolveAll(ctx, refs)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != StatusNotInPubMed {
			t.Errorf("expected NOT_IN_PUBMED for cancelled context, got %s", r.Status)
		}
	}
}

func TestResolve_PMIDFetchFails(t *testing.T) {
	// Fetch returns error, search returns empty.
	resolver, srv := newTestResolver(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "efetch") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.Contains(r.URL.Path, "esearch") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"esearchresult": map[string]interface{}{
					"count":  "0",
					"idlist": []string{},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	defer srv.Close()

	ref := ParsedReference{
		Index: 1,
		PMID:  "99999999",
		Title: "Nonexistent article",
	}

	result := resolver.Resolve(context.Background(), ref)

	// Should fall through tiers and end up NOT_IN_PUBMED.
	if result.Status != StatusNotInPubMed {
		t.Errorf("expected NOT_IN_PUBMED when fetch fails, got %s", result.Status)
	}
}

func TestSignificantWords(t *testing.T) {
	tests := []struct {
		text string
		n    int
		want int // At least this many words
	}{
		{"The mGluR theory of fragile X mental retardation", 3, 3},
		{"A", 3, 0},
		{"", 3, 0},
		{"Fragile X syndrome in mice", 5, 3}, // "fragile", "syndrome", "mice"
	}

	for _, tt := range tests {
		got := significantWords(tt.text, tt.n)
		if len(got) < tt.want {
			t.Errorf("significantWords(%q, %d) = %v, want at least %d words", tt.text, tt.n, got, tt.want)
		}
	}
}

func TestDescribeDiffs(t *testing.T) {
	ref := ParsedReference{
		DOI:   "10.1234/wrong",
		Year:  "2023",
		Title: "Some title",
		Pages: "1-10",
	}
	art := eutils.Article{
		DOI:   "10.1234/correct",
		Year:  "2024",
		Title: "Some different title",
		Pages: "1-15",
	}

	diffs := describeDiffs(ref, art)
	if len(diffs) < 2 {
		t.Errorf("expected at least 2 corrections, got %d: %v", len(diffs), diffs)
	}

	hasDOI := false
	hasYear := false
	for _, d := range diffs {
		if strings.Contains(d, "DOI") {
			hasDOI = true
		}
		if strings.Contains(d, "Year") {
			hasYear = true
		}
	}
	if !hasDOI {
		t.Error("expected DOI correction")
	}
	if !hasYear {
		t.Error("expected Year correction")
	}
}
