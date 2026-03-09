package refcheck

import (
	"context"
	"strings"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
)

// HallucinationDetector checks whether unresolved references may be fabricated.
type HallucinationDetector struct {
	client *eutils.Client
}

// NewHallucinationDetector creates a detector backed by the given eutils client.
func NewHallucinationDetector(client *eutils.Client) *HallucinationDetector {
	return &HallucinationDetector{client: client}
}

// Check examines an unresolved reference for hallucination signals.
// It returns true if the reference is likely fabricated, along with a reason.
//
// Hallucination heuristics:
//  1. Author exists in PubMed but has no paper matching this title/topic
//  2. DOI format is valid but doesn't resolve (already caught in resolve)
//  3. Journal name doesn't exist or is very rare in PubMed
//  4. Year is in the future or very recent with no match
func (h *HallucinationDetector) Check(ctx context.Context, ref ParsedReference, vr *VerifiedReference) {
	if vr.Status != StatusNotInPubMed {
		return // Only check unresolved references.
	}

	signals := h.gatherSignals(ctx, ref)

	if signals.authorPublishes && !signals.authorPublishesOnTopic {
		vr.Status = StatusPossiblyFabricated
		vr.Notes = "Author publishes in PubMed but has no papers matching this title/topic"
		return
	}

	if signals.doiPrefix && !signals.doiResolves {
		vr.Status = StatusPossiblyFabricated
		vr.Notes = "DOI prefix matches a real publisher but DOI does not resolve"
		return
	}

	if signals.authorPublishes && signals.recentYear && !signals.anyMatch {
		vr.Status = StatusPossiblyFabricated
		vr.Notes = "Known author, recent year, but no matching publication found"
		return
	}
}

type hallucinationSignals struct {
	authorPublishes        bool // First author has papers in PubMed
	authorPublishesOnTopic bool // First author has papers on a related topic
	doiPrefix              bool // DOI starts with a known publisher prefix
	doiResolves            bool // DOI search returns a result
	recentYear             bool // Year is 2023 or later
	anyMatch               bool // Any search returned results
}

func (h *HallucinationDetector) gatherSignals(ctx context.Context, ref ParsedReference) hallucinationSignals {
	var s hallucinationSignals

	// Check if year is recent.
	if ref.Year >= "2023" {
		s.recentYear = true
	}

	// Check DOI prefix.
	if ref.DOI != "" {
		s.doiPrefix = hasKnownDOIPrefix(ref.DOI)
	}

	if len(ref.Authors) == 0 {
		return s
	}

	firstAuthor := ref.Authors[0]

	// Check if first author has any PubMed publications.
	authorResult, err := h.client.Search(ctx, firstAuthor+"[au]", &eutils.SearchOptions{Limit: 1})
	if err == nil && authorResult.Count > 0 {
		s.authorPublishes = true
	}

	// Check if author publishes on a related topic (using title keywords).
	if s.authorPublishes && ref.Title != "" {
		keywords := significantWords(ref.Title, 2)
		if len(keywords) > 0 {
			topicQuery := firstAuthor + "[au] AND " + strings.Join(keywords, " AND ")
			topicResult, err := h.client.Search(ctx, topicQuery, &eutils.SearchOptions{Limit: 1})
			if err == nil && topicResult.Count > 0 {
				s.authorPublishesOnTopic = true
				s.anyMatch = true
			}
		}
	}

	return s
}

// hasKnownDOIPrefix checks if a DOI starts with a recognized publisher prefix.
var knownDOIPrefixes = []string{
	"10.1038/",  // Nature
	"10.1016/",  // Elsevier
	"10.1126/",  // Science
	"10.1001/",  // JAMA
	"10.1056/",  // NEJM
	"10.1002/",  // Wiley
	"10.1371/",  // PLOS
	"10.1186/",  // BMC/Springer
	"10.1093/",  // Oxford
	"10.1007/",  // Springer
	"10.3389/",  // Frontiers
	"10.1177/",  // SAGE
	"10.1542/",  // Pediatrics
	"10.1136/",  // BMJ
	"10.3390/",  // MDPI
	"10.1080/",  // Taylor & Francis
	"10.1073/",  // PNAS
	"10.1155/",  // Hindawi
	"10.1097/",  // Lippincott
	"10.1111/",  // Wiley-Blackwell
}

func hasKnownDOIPrefix(doi string) bool {
	normalized := strings.ToLower(doi)
	// Strip URL prefix if present.
	normalized = strings.TrimPrefix(normalized, "https://doi.org/")
	normalized = strings.TrimPrefix(normalized, "http://doi.org/")
	for _, prefix := range knownDOIPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}
