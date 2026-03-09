package refcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
	"github.com/henrybloomingdale/pubmed-cli/internal/ncbi"
)

func newTestDetector(t *testing.T, handler http.HandlerFunc) (*HallucinationDetector, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	base := ncbi.NewBaseClient(ncbi.WithBaseURL(srv.URL))
	client := eutils.NewClientWithBase(base)
	return NewHallucinationDetector(client), srv
}

func TestHallucinationDetector_AuthorExistsButNoPaper(t *testing.T) {
	detector, srv := newTestDetector(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("term")
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(query, "[au]") && !strings.Contains(query, " AND ") {
			// Author exists — has publications.
			json.NewEncoder(w).Encode(map[string]interface{}{
				"esearchresult": map[string]interface{}{
					"count":  "42",
					"idlist": []string{"12345678"},
				},
			})
			return
		}

		if strings.Contains(query, "[au]") && strings.Contains(query, " AND ") {
			// Author + topic = no results.
			json.NewEncoder(w).Encode(map[string]interface{}{
				"esearchresult": map[string]interface{}{
					"count":  "0",
					"idlist": []string{},
				},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"esearchresult": map[string]interface{}{
				"count":  "0",
				"idlist": []string{},
			},
		})
	})
	defer srv.Close()

	ref := ParsedReference{
		Index:   14,
		Authors: []string{"Thompson", "Nguyen", "Williams"},
		Year:    "2024",
		Title:   "Novel biomarkers for treatment response monitoring in fragile X syndrome",
		DOI:     "10.1038/s41380-024-02445-8",
	}

	vr := &VerifiedReference{
		Parsed: ref,
		Status: StatusNotInPubMed,
	}

	detector.Check(context.Background(), ref, vr)

	if vr.Status != StatusPossiblyFabricated {
		t.Errorf("expected POSSIBLY_FABRICATED, got %s", vr.Status)
	}
	if vr.Notes == "" {
		t.Error("expected non-empty notes explaining fabrication signal")
	}
}

func TestHallucinationDetector_AuthorExistsAndPublishesOnTopic(t *testing.T) {
	detector, srv := newTestDetector(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Author exists and publishes on this topic — not fabricated.
		json.NewEncoder(w).Encode(map[string]interface{}{
			"esearchresult": map[string]interface{}{
				"count":  "5",
				"idlist": []string{"11111111"},
			},
		})
	})
	defer srv.Close()

	ref := ParsedReference{
		Index:   1,
		Authors: []string{"Bear"},
		Year:    "2004",
		Title:   "The mGluR theory of fragile X mental retardation",
	}

	vr := &VerifiedReference{
		Parsed: ref,
		Status: StatusNotInPubMed,
	}

	detector.Check(context.Background(), ref, vr)

	// Should remain NOT_IN_PUBMED since author publishes on topic.
	if vr.Status != StatusNotInPubMed {
		t.Errorf("expected NOT_IN_PUBMED (not fabricated), got %s", vr.Status)
	}
}

func TestHallucinationDetector_AlreadyVerified(t *testing.T) {
	detector, srv := newTestDetector(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not make any HTTP requests for already-verified references")
		http.NotFound(w, r)
	})
	defer srv.Close()

	ref := ParsedReference{Index: 1, Title: "Some title"}
	vr := &VerifiedReference{
		Parsed: ref,
		Status: StatusVerifiedExact,
	}

	detector.Check(context.Background(), ref, vr)

	if vr.Status != StatusVerifiedExact {
		t.Errorf("status should remain VERIFIED_EXACT, got %s", vr.Status)
	}
}

func TestHallucinationDetector_NoAuthors(t *testing.T) {
	detector, srv := newTestDetector(t, func(w http.ResponseWriter, r *http.Request) {
		// Should not be called since there are no authors to check.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"esearchresult": map[string]interface{}{
				"count":  "0",
				"idlist": []string{},
			},
		})
	})
	defer srv.Close()

	ref := ParsedReference{
		Index: 1,
		Title: "Some article without authors",
		Year:  "2024",
	}
	vr := &VerifiedReference{
		Parsed: ref,
		Status: StatusNotInPubMed,
	}

	detector.Check(context.Background(), ref, vr)

	// Without authors, can't detect hallucination — stays NOT_IN_PUBMED.
	if vr.Status != StatusNotInPubMed {
		t.Errorf("expected NOT_IN_PUBMED, got %s", vr.Status)
	}
}

func TestHasKnownDOIPrefix(t *testing.T) {
	tests := []struct {
		doi  string
		want bool
	}{
		{"10.1038/nrdp.2017.65", true},
		{"10.1016/j.tins.2004.04.009", true},
		{"https://doi.org/10.1186/s13229-025-00641-5", true},
		{"10.9999/unknown-publisher", false},
		{"not-a-doi", false},
		{"", false},
	}

	for _, tt := range tests {
		got := hasKnownDOIPrefix(tt.doi)
		if got != tt.want {
			t.Errorf("hasKnownDOIPrefix(%q) = %v, want %v", tt.doi, got, tt.want)
		}
	}
}
