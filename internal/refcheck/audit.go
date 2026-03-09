package refcheck

import (
	"fmt"
	"regexp"
	"strings"
)

// AuditCitations cross-references in-text citations with the reference list.
// bodyText is the document body (everything before references section).
// refs are the parsed references from the reference list.
func AuditCitations(bodyText string, refs []ParsedReference) AuditResult {
	result := AuditResult{}
	cited := make(map[int]bool)

	// Build citation patterns for each reference.
	for _, ref := range refs {
		usage := findCitationUsage(bodyText, ref)
		if usage.Count > 0 {
			result.Citations = append(result.Citations, usage)
			cited[ref.Index] = true
		}
	}

	// Find uncited references.
	for _, ref := range refs {
		if !cited[ref.Index] {
			result.Uncited = append(result.Uncited, ref.Index)
		}
	}

	// Find orphan markers — numbered citations not in reference list.
	maxRef := 0
	for _, ref := range refs {
		if ref.Index > maxRef {
			maxRef = ref.Index
		}
	}
	orphans := findOrphanMarkers(bodyText, maxRef)
	result.OrphanMarkers = orphans

	return result
}

// findCitationUsage searches for in-text citations of a specific reference.
func findCitationUsage(body string, ref ParsedReference) CitationUsage {
	usage := CitationUsage{RefIndex: ref.Index}
	paragraphs := strings.Split(body, "\n")

	// Pattern 1: Numbered citations like [1], [1,2], [1-3].
	numPattern := buildNumberedPattern(ref.Index)

	// Pattern 2: Author-year citations like (Bear et al., 2004) or Bear et al. (2004).
	var authorPatterns []*regexp.Regexp
	if len(ref.Authors) > 0 && ref.Year != "" {
		authorPatterns = buildAuthorYearPatterns(ref.Authors[0], ref.Year)
	}

	seen := make(map[string]bool)

	for i, para := range paragraphs {
		if para == "" {
			continue
		}

		// Check numbered patterns.
		if numPattern != nil {
			matches := numPattern.FindAllString(para, -1)
			for _, m := range matches {
				if !seen[m] {
					usage.Markers = append(usage.Markers, m)
					seen[m] = true
				}
				usage.Count++
				usage.Paragraphs = appendUnique(usage.Paragraphs, i)
			}
		}

		// Check author-year patterns.
		for _, pat := range authorPatterns {
			matches := pat.FindAllString(para, -1)
			for _, m := range matches {
				if !seen[m] {
					usage.Markers = append(usage.Markers, m)
					seen[m] = true
				}
				usage.Count++
				usage.Paragraphs = appendUnique(usage.Paragraphs, i)
			}
		}
	}

	return usage
}

// buildNumberedPattern creates a regex that matches [N] citations where N is the reference index.
// Also handles ranges like [1-3] and lists like [1,2,3].
func buildNumberedPattern(index int) *regexp.Regexp {
	n := fmt.Sprintf("%d", index)
	// Match [N] exactly, or N within a range/list like [1,2,3] or [1-5].
	pattern := fmt.Sprintf(`\[(?:[^\]]*\b%s\b[^\]]*)\]`, regexp.QuoteMeta(n))
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}

// buildAuthorYearPatterns creates regexes for author-year citation styles.
func buildAuthorYearPatterns(firstAuthor string, year string) []*regexp.Regexp {
	var patterns []*regexp.Regexp

	escaped := regexp.QuoteMeta(firstAuthor)

	// (Author et al., Year)
	p1 := fmt.Sprintf(`\(%s\s+et\s+al\.?,?\s*%s\)`, escaped, regexp.QuoteMeta(year))
	if re, err := regexp.Compile("(?i)" + p1); err == nil {
		patterns = append(patterns, re)
	}

	// Author et al. (Year)
	p2 := fmt.Sprintf(`%s\s+et\s+al\.?\s*\(%s\)`, escaped, regexp.QuoteMeta(year))
	if re, err := regexp.Compile("(?i)" + p2); err == nil {
		patterns = append(patterns, re)
	}

	// (Author, Year) — for single-author references.
	p3 := fmt.Sprintf(`\(%s,?\s*%s\)`, escaped, regexp.QuoteMeta(year))
	if re, err := regexp.Compile("(?i)" + p3); err == nil {
		patterns = append(patterns, re)
	}

	// Author (Year)
	p4 := fmt.Sprintf(`%s\s*\(%s\)`, escaped, regexp.QuoteMeta(year))
	if re, err := regexp.Compile("(?i)" + p4); err == nil {
		patterns = append(patterns, re)
	}

	return patterns
}

// findOrphanMarkers detects numbered citation markers that reference indices beyond
// the reference list.
func findOrphanMarkers(body string, maxRef int) []string {
	// Find all [N] patterns.
	re := regexp.MustCompile(`\[(\d+)\]`)
	matches := re.FindAllStringSubmatch(body, -1)

	seen := make(map[string]bool)
	var orphans []string

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		numStr := m[1]
		var num int
		fmt.Sscanf(numStr, "%d", &num)
		if num > maxRef && !seen[m[0]] {
			orphans = append(orphans, m[0])
			seen[m[0]] = true
		}
	}

	return orphans
}

func appendUnique(slice []int, val int) []int {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}
