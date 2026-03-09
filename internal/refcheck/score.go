package refcheck

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
)

// punctuationRe matches non-alphanumeric, non-space characters.
var punctuationRe = regexp.MustCompile(`[^\p{L}\p{N}\s]`)

// doiPrefixRe strips common DOI URL prefixes.
var doiPrefixRe = regexp.MustCompile(`^https?://(dx\.)?doi\.org/`)

// NormalizeDOI lowercases and strips URL prefixes from a DOI string.
func NormalizeDOI(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = doiPrefixRe.ReplaceAllString(s, "")
	return strings.ToLower(s)
}

// NormalizeTitle lowercases, strips punctuation, and collapses whitespace.
func NormalizeTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ToLower(s)
	s = punctuationRe.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// TokenJaccard computes Jaccard similarity on whitespace-split word tokens.
func TokenJaccard(a, b string) float64 {
	tokensA := tokenize(a)
	tokensB := tokenize(b)
	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 0.0
	}
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0.0
	}

	setA := make(map[string]bool, len(tokensA))
	for _, t := range tokensA {
		setA[t] = true
	}
	setB := make(map[string]bool, len(tokensB))
	for _, t := range tokensB {
		setB[t] = true
	}

	intersection := 0
	for t := range setA {
		if setB[t] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

func tokenize(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

// ScoreMatch computes how well a PubMed article matches a parsed reference.
func ScoreMatch(ref ParsedReference, article eutils.Article) MatchScore {
	var ms MatchScore

	// DOI scoring
	if ref.DOI != "" && article.DOI != "" {
		refDOI := NormalizeDOI(ref.DOI)
		artDOI := NormalizeDOI(article.DOI)
		if refDOI == artDOI {
			ms.DOI = 1.0
		}
	}

	// PMID scoring
	if ref.PMID != "" && article.PMID != "" {
		if strings.TrimSpace(ref.PMID) == strings.TrimSpace(article.PMID) {
			ms.PMID = 1.0
		}
	}

	// Fast path: exact DOI or PMID match
	if ms.DOI == 1.0 || ms.PMID == 1.0 {
		ms.Total = 1.0
		// Still compute other scores for informational purposes
		ms.Title = scoreTitle(ref.Title, article.Title)
		ms.AuthorHit = scoreAuthors(ref.Authors, article.Authors)
		ms.Year = scoreYear(ref.Year, article.Year)
		ms.Journal = scoreJournal(ref.Journal, article.Journal, article.JournalAbbrev)
		return ms
	}

	// Title scoring
	ms.Title = scoreTitle(ref.Title, article.Title)

	// Author scoring
	ms.AuthorHit = scoreAuthors(ref.Authors, article.Authors)

	// Year scoring
	ms.Year = scoreYear(ref.Year, article.Year)

	// Journal scoring
	ms.Journal = scoreJournal(ref.Journal, article.Journal, article.JournalAbbrev)

	// Weighted total (excluding DOI/PMID from weighted sum since they're on the fast path)
	weightSum := ScoreWeights.Title + ScoreWeights.Author + ScoreWeights.Year + ScoreWeights.Journal
	if weightSum > 0 {
		ms.Total = (ms.Title*ScoreWeights.Title +
			ms.AuthorHit*ScoreWeights.Author +
			ms.Year*ScoreWeights.Year +
			ms.Journal*ScoreWeights.Journal) / weightSum
	}

	return ms
}

func scoreTitle(refTitle, artTitle string) float64 {
	if refTitle == "" || artTitle == "" {
		return 0.0
	}
	normRef := NormalizeTitle(refTitle)
	normArt := NormalizeTitle(artTitle)
	if normRef == "" || normArt == "" {
		return 0.0
	}
	return TokenJaccard(normRef, normArt)
}

func scoreAuthors(refAuthors []string, artAuthors []eutils.Author) float64 {
	if len(refAuthors) == 0 {
		return 0.0
	}

	// Build set of article author last names (lowercased)
	artLastNames := make(map[string]bool, len(artAuthors))
	for _, a := range artAuthors {
		if a.LastName != "" {
			artLastNames[strings.ToLower(a.LastName)] = true
		}
	}

	if len(artLastNames) == 0 {
		return 0.0
	}

	matched := 0
	for _, refAuthor := range refAuthors {
		if artLastNames[strings.ToLower(refAuthor)] {
			matched++
		}
	}
	return float64(matched) / float64(len(refAuthors))
}

func scoreYear(refYear, artYear string) float64 {
	if refYear == "" || artYear == "" {
		return 0.0
	}
	ry, err1 := strconv.Atoi(strings.TrimSpace(refYear))
	ay, err2 := strconv.Atoi(strings.TrimSpace(artYear))
	if err1 != nil || err2 != nil {
		return 0.0
	}
	diff := math.Abs(float64(ry - ay))
	switch {
	case diff == 0:
		return 1.0
	case diff == 1:
		return 0.5
	default:
		return 0.0
	}
}

func scoreJournal(refJournal, artJournal, artJournalAbbrev string) float64 {
	if refJournal == "" {
		return 0.0
	}
	normRef := normalizeJournal(refJournal)
	if normRef == "" {
		return 0.0
	}

	// Try full journal name first, then abbreviation, take the best
	best := 0.0
	if artJournal != "" {
		normArt := normalizeJournal(artJournal)
		if normArt != "" {
			j := TokenJaccard(normRef, normArt)
			if j > best {
				best = j
			}
		}
	}
	if artJournalAbbrev != "" {
		normAbbrev := normalizeJournal(artJournalAbbrev)
		if normAbbrev != "" {
			j := TokenJaccard(normRef, normAbbrev)
			if j > best {
				best = j
			}
		}
	}
	return best
}

func normalizeJournal(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = punctuationRe.ReplaceAllString(s, "")
	// Remove common stop words
	words := strings.Fields(s)
	var filtered []string
	stopWords := map[string]bool{"the": true, "of": true}
	for _, w := range words {
		if !stopWords[w] {
			filtered = append(filtered, w)
		}
	}
	return strings.Join(filtered, " ")
}
