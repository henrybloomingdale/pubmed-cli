// Package refcheck verifies document references against PubMed.
package refcheck

import "github.com/henrybloomingdale/pubmed-cli/internal/eutils"

// VerificationStatus classifies the result of PubMed verification.
type VerificationStatus string

const (
	StatusVerifiedExact       VerificationStatus = "VERIFIED_EXACT"
	StatusVerifiedCorrected   VerificationStatus = "VERIFIED_WITH_CORRECTION"
	StatusVerifiedByTitle     VerificationStatus = "VERIFIED_BY_TITLE"
	StatusCandidate           VerificationStatus = "CANDIDATE_FROM_INCOMPLETE_CITATION"
	StatusNotInPubMed         VerificationStatus = "NOT_IN_PUBMED"
	StatusPossiblyFabricated  VerificationStatus = "POSSIBLY_FABRICATED"
)

// ParsedReference holds fields extracted from a single reference string.
type ParsedReference struct {
	Raw     string   // Original full text of the reference
	Index   int      // Position in the reference list (1-based)
	Authors []string // Last names extracted (e.g., ["Bear", "Huber", "Warren"])
	Year    string   // 4-digit year
	Title   string   // Article title
	Journal string   // Journal name/abbreviation
	Volume  string
	Issue   string
	Pages   string
	DOI     string // Extracted DOI
	PMID    string // Extracted PMID
}

// MatchScore breaks down how well a PubMed article matches a parsed reference.
type MatchScore struct {
	Total      float64 // Weighted sum [0,1]
	DOI        float64 // 1.0 if exact match
	PMID       float64 // 1.0 if exact match
	Title      float64 // Normalized similarity [0,1]
	AuthorHit  float64 // Fraction of ref authors found in article [0,1]
	Year       float64 // 1.0 if exact, 0.5 if ±1 year
	Journal    float64 // Normalized similarity [0,1]
}

// Weights for combining MatchScore components into Total.
var ScoreWeights = struct {
	DOI     float64
	PMID    float64
	Title   float64
	Author  float64
	Year    float64
	Journal float64
}{
	DOI:     1.00,
	PMID:    1.00,
	Title:   0.40,
	Author:  0.25,
	Year:    0.20,
	Journal: 0.10,
}

// VerifiedReference is the final result for one reference.
type VerifiedReference struct {
	Parsed      ParsedReference    `json:"parsed"`
	Status      VerificationStatus `json:"status"`
	Confidence  float64            `json:"confidence"` // Best MatchScore.Total
	Corrections []string           `json:"corrections,omitempty"`
	Match       *eutils.Article    `json:"match,omitempty"`    // Best PubMed match
	Candidates  []eutils.Article   `json:"candidates,omitempty"` // Runner-up matches
	QueryTiers  []string           `json:"query_tiers,omitempty"` // Tiers attempted
	Notes       string             `json:"notes,omitempty"`
}

// CitationUsage tracks where an in-text citation appears in the document body.
type CitationUsage struct {
	RefIndex    int      `json:"ref_index"`    // Which reference (1-based)
	Markers     []string `json:"markers"`      // Matched citation markers (e.g., "[1]", "(Bear et al., 2004)")
	Paragraphs  []int    `json:"paragraphs"`   // Paragraph indices where found
	Count       int      `json:"count"`        // Total occurrences
}

// AuditResult holds the complete in-text citation audit.
type AuditResult struct {
	Citations     []CitationUsage `json:"citations"`
	Uncited       []int           `json:"uncited"`        // Reference indices not cited in text
	OrphanMarkers []string        `json:"orphan_markers"` // In-text citations with no matching reference
}

// Report is the top-level result of a reference check.
type Report struct {
	DocumentPath string              `json:"document_path"`
	RefCount     int                 `json:"ref_count"`
	Results      []VerifiedReference `json:"results"`
	Audit        *AuditResult        `json:"audit,omitempty"`
	Summary      ReportSummary       `json:"summary"`
}

// ReportSummary aggregates counts by status.
type ReportSummary struct {
	Total             int `json:"total"`
	VerifiedExact     int `json:"verified_exact"`
	VerifiedCorrected int `json:"verified_with_correction"`
	VerifiedByTitle   int `json:"verified_by_title"`
	Candidate         int `json:"candidate"`
	NotInPubMed       int `json:"not_in_pubmed"`
	PossiblyFabricated int `json:"possibly_fabricated"`
}
