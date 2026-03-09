package refcheck

import (
	"context"
	"fmt"
	"strings"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
)

// Resolver verifies parsed references against PubMed.
type Resolver struct {
	client *eutils.Client
}

// NewResolver creates a Resolver backed by the given eutils client.
func NewResolver(client *eutils.Client) *Resolver {
	return &Resolver{client: client}
}

// Resolve attempts to verify a single reference via tiered PubMed queries.
// It returns a VerifiedReference with the best match, candidates, and status.
func (r *Resolver) Resolve(ctx context.Context, ref ParsedReference) VerifiedReference {
	vr := VerifiedReference{Parsed: ref}

	// Tier 0: Direct lookup by PMID or DOI.
	if ref.PMID != "" {
		if art := r.fetchByPMID(ctx, ref.PMID); art != nil {
			score := ScoreMatch(ref, *art)
			vr.Match = art
			vr.Confidence = score.Total
			vr.QueryTiers = append(vr.QueryTiers, "tier0_pmid")
			if score.Total >= 0.95 {
				vr.Status = StatusVerifiedExact
			} else {
				vr.Status = StatusVerifiedCorrected
				vr.Corrections = describeDiffs(ref, *art)
			}
			return vr
		}
		vr.QueryTiers = append(vr.QueryTiers, "tier0_pmid_miss")
	}

	if ref.DOI != "" {
		if art := r.searchByDOI(ctx, ref.DOI); art != nil {
			score := ScoreMatch(ref, *art)
			vr.Match = art
			vr.Confidence = score.Total
			vr.QueryTiers = append(vr.QueryTiers, "tier0_doi")
			if score.Total >= 0.95 {
				vr.Status = StatusVerifiedExact
			} else {
				vr.Status = StatusVerifiedCorrected
				vr.Corrections = describeDiffs(ref, *art)
			}
			return vr
		}
		vr.QueryTiers = append(vr.QueryTiers, "tier0_doi_miss")
	}

	// Tier 1: Title search.
	if ref.Title != "" {
		arts := r.searchByTitle(ctx, ref.Title)
		if best, score := r.bestMatch(ref, arts); best != nil && score.Total >= 0.75 {
			vr.Match = best
			vr.Confidence = score.Total
			vr.Candidates = filterCandidates(arts, best)
			vr.QueryTiers = append(vr.QueryTiers, "tier1_title")
			if score.Total >= 0.95 {
				vr.Status = StatusVerifiedExact
			} else if score.Title >= 0.8 {
				vr.Status = StatusVerifiedByTitle
			} else {
				vr.Status = StatusVerifiedCorrected
				vr.Corrections = describeDiffs(ref, *best)
			}
			return vr
		}
		vr.QueryTiers = append(vr.QueryTiers, "tier1_title_miss")
	}

	// Tier 2: Author + year + keywords from title.
	if len(ref.Authors) > 0 && ref.Year != "" {
		arts := r.searchByAuthorYear(ctx, ref)
		if best, score := r.bestMatch(ref, arts); best != nil && score.Total >= 0.65 {
			vr.Match = best
			vr.Confidence = score.Total
			vr.Candidates = filterCandidates(arts, best)
			vr.QueryTiers = append(vr.QueryTiers, "tier2_author_year")
			if score.Total >= 0.95 {
				vr.Status = StatusVerifiedExact
			} else if score.Title >= 0.7 {
				vr.Status = StatusVerifiedByTitle
			} else {
				vr.Status = StatusCandidate
			}
			return vr
		}
		vr.QueryTiers = append(vr.QueryTiers, "tier2_author_year_miss")
	}

	// Tier 3: Relaxed search — first author + key title words.
	if len(ref.Authors) > 0 || ref.Title != "" {
		arts := r.searchRelaxed(ctx, ref)
		if best, score := r.bestMatch(ref, arts); best != nil && score.Total >= 0.55 {
			vr.Match = best
			vr.Confidence = score.Total
			vr.Candidates = filterCandidates(arts, best)
			vr.QueryTiers = append(vr.QueryTiers, "tier3_relaxed")
			vr.Status = StatusCandidate
			return vr
		}
		vr.QueryTiers = append(vr.QueryTiers, "tier3_relaxed_miss")
	}

	// No match found.
	vr.Status = StatusNotInPubMed
	vr.QueryTiers = append(vr.QueryTiers, "unresolved")
	return vr
}

// ResolveAll verifies a batch of references sequentially.
// It respects context cancellation between references.
func (r *Resolver) ResolveAll(ctx context.Context, refs []ParsedReference) []VerifiedReference {
	results := make([]VerifiedReference, 0, len(refs))
	for _, ref := range refs {
		if ctx.Err() != nil {
			// Fill remaining with error status.
			vr := VerifiedReference{Parsed: ref, Status: StatusNotInPubMed, Notes: "cancelled"}
			results = append(results, vr)
			continue
		}
		results = append(results, r.Resolve(ctx, ref))
	}
	return results
}

// fetchByPMID fetches a single article by PMID.
func (r *Resolver) fetchByPMID(ctx context.Context, pmid string) *eutils.Article {
	arts, err := r.client.Fetch(ctx, []string{pmid})
	if err != nil || len(arts) == 0 {
		return nil
	}
	return &arts[0]
}

// searchByDOI searches PubMed for an article by DOI.
func (r *Resolver) searchByDOI(ctx context.Context, doi string) *eutils.Article {
	normalized := NormalizeDOI(doi)
	query := fmt.Sprintf("%s[doi]", normalized)
	return r.searchOne(ctx, query)
}

// searchByTitle searches PubMed using the reference title.
func (r *Resolver) searchByTitle(ctx context.Context, title string) []eutils.Article {
	// Use quoted title for phrase search, fall back to unquoted.
	clean := strings.TrimRight(strings.TrimSpace(title), ".")
	query := fmt.Sprintf(`"%s"`, clean)
	arts := r.searchUpTo(ctx, query, 5)
	if len(arts) == 0 {
		// Retry without quotes for fuzzy match.
		arts = r.searchUpTo(ctx, clean, 5)
	}
	return arts
}

// searchByAuthorYear builds a query from first author + year + title keywords.
func (r *Resolver) searchByAuthorYear(ctx context.Context, ref ParsedReference) []eutils.Article {
	var parts []string
	if len(ref.Authors) > 0 {
		parts = append(parts, ref.Authors[0]+"[au]")
	}
	if ref.Year != "" {
		parts = append(parts, ref.Year+"[dp]")
	}
	// Add 2-3 significant title words.
	if ref.Title != "" {
		keywords := significantWords(ref.Title, 3)
		for _, kw := range keywords {
			parts = append(parts, kw)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	query := strings.Join(parts, " AND ")
	return r.searchUpTo(ctx, query, 10)
}

// searchRelaxed tries a broad search with just author or title keywords.
func (r *Resolver) searchRelaxed(ctx context.Context, ref ParsedReference) []eutils.Article {
	var parts []string
	if len(ref.Authors) > 0 {
		parts = append(parts, ref.Authors[0]+"[au]")
	}
	if ref.Title != "" {
		keywords := significantWords(ref.Title, 5)
		parts = append(parts, keywords...)
	}
	if len(parts) == 0 {
		return nil
	}
	query := strings.Join(parts, " AND ")
	return r.searchUpTo(ctx, query, 10)
}

// searchOne returns the first article matching a query, or nil.
func (r *Resolver) searchOne(ctx context.Context, query string) *eutils.Article {
	arts := r.searchUpTo(ctx, query, 1)
	if len(arts) == 0 {
		return nil
	}
	return &arts[0]
}

// searchUpTo searches PubMed and fetches up to limit article details.
func (r *Resolver) searchUpTo(ctx context.Context, query string, limit int) []eutils.Article {
	result, err := r.client.Search(ctx, query, &eutils.SearchOptions{Limit: limit})
	if err != nil || len(result.IDs) == 0 {
		return nil
	}
	arts, err := r.client.Fetch(ctx, result.IDs)
	if err != nil {
		return nil
	}
	return arts
}

// bestMatch finds the article with the highest score above threshold.
func (r *Resolver) bestMatch(ref ParsedReference, articles []eutils.Article) (*eutils.Article, MatchScore) {
	var (
		best      *eutils.Article
		bestScore MatchScore
	)
	for i := range articles {
		score := ScoreMatch(ref, articles[i])
		if score.Total > bestScore.Total {
			bestScore = score
			best = &articles[i]
		}
	}
	return best, bestScore
}

// filterCandidates removes the best match from the list, keeping top candidates.
func filterCandidates(articles []eutils.Article, best *eutils.Article) []eutils.Article {
	if best == nil {
		return articles
	}
	var candidates []eutils.Article
	for i := range articles {
		if articles[i].PMID != best.PMID {
			candidates = append(candidates, articles[i])
		}
	}
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	return candidates
}

// describeDiffs produces human-readable correction notes between ref and article.
func describeDiffs(ref ParsedReference, art eutils.Article) []string {
	var corrections []string
	if ref.DOI != "" && art.DOI != "" && NormalizeDOI(ref.DOI) != NormalizeDOI(art.DOI) {
		corrections = append(corrections, fmt.Sprintf("DOI: %s → %s", ref.DOI, art.DOI))
	}
	if ref.Year != "" && art.Year != "" && ref.Year != art.Year {
		corrections = append(corrections, fmt.Sprintf("Year: %s → %s", ref.Year, art.Year))
	}
	if ref.Title != "" && art.Title != "" {
		refNorm := NormalizeTitle(ref.Title)
		artNorm := NormalizeTitle(art.Title)
		if refNorm != artNorm && TokenJaccard(refNorm, artNorm) < 0.95 {
			corrections = append(corrections, fmt.Sprintf("Title differs: %q", art.Title))
		}
	}
	if ref.Pages != "" && art.Pages != "" && ref.Pages != art.Pages {
		corrections = append(corrections, fmt.Sprintf("Pages: %s → %s", ref.Pages, art.Pages))
	}
	return corrections
}

// significantWords extracts up to n non-stopword words from text.
func significantWords(text string, n int) []string {
	stop := map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "or": true,
		"of": true, "in": true, "to": true, "for": true, "with": true,
		"on": true, "at": true, "by": true, "from": true, "is": true,
		"are": true, "was": true, "were": true, "be": true, "been": true,
		"has": true, "have": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "can": true, "shall": true, "not": true,
		"no": true, "nor": true, "but": true, "if": true, "then": true,
		"than": true, "that": true, "this": true, "these": true, "those": true,
		"it": true, "its": true, "as": true, "into": true, "about": true,
	}

	words := strings.Fields(strings.ToLower(text))
	var result []string
	for _, w := range words {
		// Strip punctuation.
		w = strings.Trim(w, ".,;:!?\"'()-[]")
		if len(w) < 3 || stop[w] {
			continue
		}
		result = append(result, w)
		if len(result) >= n {
			break
		}
	}
	return result
}
