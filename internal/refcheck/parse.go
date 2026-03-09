package refcheck

import (
	"regexp"
	"strings"
)

// Regex patterns compiled once at package init.
var (
	// PMID extraction: "PMID: 28960184" or "PMID:28960184" or "PMID 28960184"
	rePMID = regexp.MustCompile(`(?i)PMID[:\s]\s*(\d+)`)

	// DOI extraction: "doi:10.xxxx/..." or "doi: 10.xxxx/..." or "https://doi.org/10.xxxx/..."
	reDOI = regexp.MustCompile(`(?i)(?:doi[:\s]\s*|doi\.org/)(10\.\d{4,}[^\s,;]*)`)

	// Numbered reference: "1. " at start of line
	reNumberedDot = regexp.MustCompile(`^\s*(\d+)\.\s+`)
	// Bracket-numbered: "[1] " at start of line
	reNumberedBracket = regexp.MustCompile(`^\s*\[(\d+)\]\s+`)
	// Parenthesis-numbered: "1) " at start of line
	reNumberedParen = regexp.MustCompile(`^\s*(\d+)\)\s+`)

	// Header line
	reHeader = regexp.MustCompile(`(?i)^\s*references?\s*$`)

	// Year in parentheses (APA): "(2004)"
	reYearParen = regexp.MustCompile(`\((\d{4})\)`)
	// Year after semicolon (Vancouver): "2017;" or standalone year
	reYear = regexp.MustCompile(`(?:^|[\s;(,])(\d{4})(?:[);.,\s]|$)`)

	// Volume(Issue):Pages — e.g., "60(3):145-158" or "27(7):370-377"
	reVolIssuePage = regexp.MustCompile(`(\d+)\(([^)]+)\)[:\s]*([A-Za-z]?\d+[-–]\w*\d+|[A-Za-z]?\d+)`)

	// APA author: "Bear, M. F." or "Bear, M. F., & "
	reAPAAuthor = regexp.MustCompile(`([A-Z][a-zA-Z'-]+),\s+[A-Z]\.\s*[A-Z]?\.?`)
)

// ExtractPMID returns the PubMed ID from a string, or empty if not found.
func ExtractPMID(s string) string {
	m := rePMID.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// ExtractDOI returns the DOI from a string, or empty if not found.
func ExtractDOI(s string) string {
	m := reDOI.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	doi := m[1]
	// Strip trailing punctuation that is not part of the DOI
	doi = strings.TrimRight(doi, ".,;:) ")
	return doi
}

// ParseReferences takes the raw text of a references section and returns
// parsed references. Parsing is best-effort: partial extraction is
// preferred over returning an error.
func ParseReferences(text string) ([]ParsedReference, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	lines := strings.Split(text, "\n")

	// Strip header line (e.g., "References")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		if reHeader.MatchString(line) {
			continue
		}
		cleaned = append(cleaned, line)
	}

	// Detect reference format and split into blocks
	blocks := splitIntoBlocks(cleaned)
	if len(blocks) == 0 {
		return nil, nil
	}

	refs := make([]ParsedReference, 0, len(blocks))
	for i, block := range blocks {
		raw := strings.TrimSpace(block)
		if raw == "" {
			continue
		}

		ref := ParsedReference{
			Raw:   raw,
			Index: i + 1, // will be overridden if numbered
		}

		// Strip leading number prefix
		body := raw
		if m := reNumberedDot.FindStringSubmatch(body); m != nil {
			ref.Index = atoi(m[1])
			body = body[len(m[0]):]
		} else if m := reNumberedBracket.FindStringSubmatch(body); m != nil {
			ref.Index = atoi(m[1])
			body = body[len(m[0]):]
		} else if m := reNumberedParen.FindStringSubmatch(body); m != nil {
			ref.Index = atoi(m[1])
			body = body[len(m[0]):]
		}

		ref.PMID = ExtractPMID(body)
		ref.DOI = ExtractDOI(body)

		// Detect APA vs Vancouver format
		isAPA := reYearParen.MatchString(body) && strings.Contains(body, ", &") || isAPAFormat(body)

		if isAPA {
			parseAPAReference(&ref, body)
		} else {
			parseVancouverReference(&ref, body)
		}

		refs = append(refs, ref)
	}

	return refs, nil
}

// splitIntoBlocks splits cleaned lines into individual reference text blocks.
func splitIntoBlocks(lines []string) []string {
	// First check if lines are numbered (any format)
	numbered := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if reNumberedDot.MatchString(trimmed) || reNumberedBracket.MatchString(trimmed) || reNumberedParen.MatchString(trimmed) {
			numbered = true
			break
		}
	}

	if numbered {
		return splitNumberedBlocks(lines)
	}

	// Blank-line delimited
	return splitBlankLineBlocks(lines)
}

// splitNumberedBlocks groups lines starting with a number prefix, folding
// continuation lines into the previous block.
func splitNumberedBlocks(lines []string) []string {
	var blocks []string
	var current strings.Builder

	isStart := func(line string) bool {
		return reNumberedDot.MatchString(line) || reNumberedBracket.MatchString(line) || reNumberedParen.MatchString(line)
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isStart(trimmed) {
			if current.Len() > 0 {
				blocks = append(blocks, current.String())
				current.Reset()
			}
			current.WriteString(trimmed)
		} else {
			if current.Len() > 0 {
				current.WriteString(" ")
			}
			current.WriteString(trimmed)
		}
	}
	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}
	return blocks
}

// splitBlankLineBlocks splits on blank lines.
func splitBlankLineBlocks(lines []string) []string {
	var blocks []string
	var current strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if current.Len() > 0 {
				blocks = append(blocks, current.String())
				current.Reset()
			}
			continue
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(trimmed)
	}
	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}
	return blocks
}

// isAPAFormat checks if the body looks like APA style
// (e.g., "Author, A. B., & Author, C. D. (Year). Title.")
func isAPAFormat(body string) bool {
	return reAPAAuthor.MatchString(body) && reYearParen.MatchString(body)
}

// parseVancouverReference extracts fields from a Vancouver-style reference.
// Format: Authors. Title. Journal. Year;Vol(Issue):Pages. doi:X. PMID: Y
//
// Strategy:
// 1. Find the author/title boundary — either "et al. " or the first real
//    sentence boundary (period-space-uppercase, skipping initials).
// 2. Find the journal by locating ". Year;" anchor and taking the segment
//    just before it.
// 3. The title is everything between the author boundary and the journal.
func parseVancouverReference(ref *ParsedReference, body string) {
	// Extract year
	ref.Year = extractYear(body)

	// Extract volume/issue/pages
	extractVolumeIssuePages(ref, body)

	// Strip trailing DOI/PMID/PMCID for cleaner structural parsing
	cleanBody := removeDOIPMID(body)
	cleanBody = strings.TrimRight(cleanBody, " .")

	// Step 1: find the author-title boundary.
	// In Vancouver, authors end with either:
	//   - "et al. " followed by the title
	//   - "INITIALS. " (last author's initials + period) followed by the title
	// We look for "et al." first, then look for the initials-period-title pattern.
	authorEnd := -1 // index of the period ending the author section

	// Check for "et al." — this always marks end of authors
	reEtAl := regexp.MustCompile(`(?i)et\s+al\.`)
	if loc := reEtAl.FindStringIndex(cleanBody); loc != nil {
		authorEnd = loc[1] - 1 // index of the period in "et al."
	}

	// If no "et al.", look for the Vancouver author-title transition:
	// pattern: "LastName INITIALS. TitleWord" where INITIALS is 1-3 uppercase
	// letters and TitleWord starts with uppercase followed by lowercase.
	if authorEnd < 0 {
		reAuthorTitle := regexp.MustCompile(`[A-Z]{1,4}\.\s+[A-Z][a-z]`)
		if loc := reAuthorTitle.FindStringIndex(cleanBody); loc != nil {
			// The period is at loc[0] + length_of_initials
			// Find the actual period position
			for j := loc[0]; j < loc[1]; j++ {
				if cleanBody[j] == '.' {
					authorEnd = j
					break
				}
			}
		}
	}

	// Last resort: try generic first sentence boundary
	if authorEnd < 0 {
		authorEnd = findFirstSentenceBoundary(cleanBody)
	}

	if authorEnd < 0 {
		// Can't find any boundary — whole thing is authors
		ref.Authors = parseVancouverAuthors(cleanBody)
		return
	}

	authorPart := cleanBody[:authorEnd]
	afterAuthors := strings.TrimSpace(cleanBody[authorEnd+1:])

	ref.Authors = parseVancouverAuthors(authorPart)

	// Step 2: find the journal by locating ". Year;" or ". Year." in afterAuthors
	yearStr := ref.Year
	journalEnd := -1
	if yearStr != "" {
		for _, pat := range []string{". " + yearStr + ";", ". " + yearStr + "."} {
			idx := strings.Index(afterAuthors, pat)
			if idx >= 0 {
				journalEnd = idx
				break
			}
		}
		if journalEnd < 0 {
			pat := ". " + yearStr
			idx := strings.Index(afterAuthors, pat)
			if idx >= 0 {
				journalEnd = idx
			}
		}
	}

	if journalEnd >= 0 {
		// afterAuthors[:journalEnd] = "Title. Journal"
		titleAndJournal := afterAuthors[:journalEnd]

		// Find the last sentence boundary in titleAndJournal to split title from journal
		lastBound := findLastSentenceBoundary(titleAndJournal)
		if lastBound >= 0 {
			ref.Title = strings.TrimSpace(titleAndJournal[:lastBound])
			ref.Title = strings.TrimRight(ref.Title, ".")
			ref.Journal = strings.TrimSpace(titleAndJournal[lastBound+1:])
			ref.Journal = strings.TrimLeft(ref.Journal, " ")
		} else {
			// No boundary: the whole thing could be just the title (no journal)
			// or just the journal. Heuristic: if it's short (< 50 chars), it's
			// probably the journal. Otherwise it's the title.
			text := strings.TrimRight(titleAndJournal, ".")
			if len(text) < 50 {
				ref.Journal = strings.TrimSpace(text)
			} else {
				ref.Title = strings.TrimSpace(text)
			}
		}
	} else {
		// No year anchor — fall back to splitting afterAuthors by sentence boundaries
		parts := splitVancouverParts(afterAuthors)
		if len(parts) >= 1 {
			ref.Title = strings.TrimRight(parts[0], ".")
		}
		if len(parts) >= 2 {
			ref.Journal = strings.TrimRight(parts[1], ".")
		}
	}
}

// findLastSentenceBoundary finds the position of the last period that represents
// a sentence boundary (period followed by space and uppercase letter) in s.
// Returns the index of the period, or -1.
func findLastSentenceBoundary(s string) int {
	runes := []rune(s)
	lastBoundary := -1
	for i := 0; i < len(runes)-2; i++ {
		if runes[i] == '.' && runes[i+1] == ' ' {
			// Check if this period is a real sentence boundary
			if isSentenceBoundaryAt(runes, i) {
				lastBoundary = i
			}
		}
	}
	return lastBoundary
}

// findFirstSentenceBoundary finds the position of the first period that represents
// a sentence boundary (period followed by space and uppercase letter, and not an
// initial or "et al.") in s.
// Returns the index of the period, or -1.
func findFirstSentenceBoundary(s string) int {
	runes := []rune(s)
	for i := 0; i < len(runes)-2; i++ {
		if runes[i] == '.' && runes[i+1] == ' ' {
			if isSentenceBoundaryAt(runes, i) {
				return i
			}
		}
	}
	return -1
}

// isSentenceBoundaryAt checks if the period at position i in runes is a real
// sentence boundary (not an initial, not "et al.", and followed by uppercase letter).
func isSentenceBoundaryAt(runes []rune, i int) bool {
	// Must be followed by space then something
	if i+2 >= len(runes) {
		return false
	}
	if runes[i+1] != ' ' {
		return false
	}

	// Check character after the space
	nextChar := runes[i+2]
	// Must start with uppercase to be a new sentence/section
	if nextChar < 'A' || nextChar > 'Z' {
		return false
	}

	// Check if this is an initial period: single uppercase letter before the period
	if i > 0 && runes[i-1] >= 'A' && runes[i-1] <= 'Z' {
		// Check if the char before the uppercase is a space, period, or start of string
		if i < 2 || runes[i-2] == ' ' || runes[i-2] == '.' || (runes[i-2] >= 'A' && runes[i-2] <= 'Z') {
			return false
		}
	}

	// Check if this ends "et al."
	if i >= 5 {
		preceding := string(runes[i-5 : i+1])
		if strings.HasSuffix(preceding, "et al.") {
			return false
		}
	}
	if i >= 6 {
		preceding := string(runes[i-6 : i+1])
		if strings.HasSuffix(preceding, "et al.") {
			return false
		}
	}

	return true
}

// splitVancouverParts is a fallback that splits on ". " followed by uppercase,
// respecting initials and "et al."
func splitVancouverParts(s string) []string {
	var parts []string
	runes := []rune(s)
	start := 0

	for i := 0; i < len(runes); i++ {
		if runes[i] == '.' && i+2 < len(runes) && runes[i+1] == ' ' {
			if isSentenceBoundaryAt(runes, i) {
				part := strings.TrimSpace(string(runes[start : i+1]))
				part = strings.TrimRight(part, ".")
				if part != "" {
					parts = append(parts, part)
				}
				start = i + 2
			}
		}
	}
	// Add remaining
	remaining := strings.TrimSpace(string(runes[start:]))
	remaining = strings.TrimRight(remaining, ".")
	if remaining != "" {
		parts = append(parts, remaining)
	}
	return parts
}

// parseAPAReference extracts fields from an APA-style reference.
// Format: Author, A. B., & Author, C. D. (Year). Title. Journal, Vol(Issue), Pages.
func parseAPAReference(ref *ParsedReference, body string) {
	// Extract year from parentheses
	if m := reYearParen.FindStringSubmatch(body); m != nil {
		ref.Year = m[1]
	}

	// Split on "(Year). " to separate authors from the rest
	yearSep := "(" + ref.Year + ")."
	idx := strings.Index(body, yearSep)
	if idx < 0 {
		// Fallback: just extract what we can
		ref.Year = extractYear(body)
		ref.Authors = parseAPAAuthors(body)
		return
	}

	authorPart := body[:idx]
	rest := strings.TrimSpace(body[idx+len(yearSep):])

	ref.Authors = parseAPAAuthors(authorPart)

	// After "(Year). " the next sentence is the title, then journal info
	titleEnd := findTitleEnd(rest)
	if titleEnd > 0 {
		ref.Title = strings.TrimSpace(rest[:titleEnd])
		journalPart := strings.TrimSpace(rest[titleEnd+1:])
		// Journal is up to the first comma followed by volume
		if commaIdx := strings.Index(journalPart, ","); commaIdx > 0 {
			ref.Journal = strings.TrimSpace(journalPart[:commaIdx])
		} else {
			ref.Journal = journalPart
		}
	} else {
		ref.Title = rest
	}

	// Remove DOI from journal/title
	ref.Journal = removeDOIPMID(ref.Journal)
	ref.Journal = strings.TrimRight(ref.Journal, ". ")
	ref.Title = strings.TrimRight(ref.Title, ".")

	extractVolumeIssuePages(ref, body)
}

// extractYear finds the most likely publication year in the text.
func extractYear(s string) string {
	// First try year after semicolon (Vancouver: "2017;")
	reSemiYear := regexp.MustCompile(`(\d{4});`)
	if m := reSemiYear.FindStringSubmatch(s); m != nil {
		return m[1]
	}

	// Try year in parentheses (APA: "(2004)")
	if m := reYearParen.FindStringSubmatch(s); m != nil {
		return m[1]
	}

	// General year extraction — find all 4-digit numbers that look like years
	allYears := reYear.FindAllStringSubmatch(s, -1)
	for _, m := range allYears {
		y := m[1]
		yi := atoi(y)
		if yi >= 1900 && yi <= 2100 {
			return y
		}
	}

	return ""
}

// extractVolumeIssuePages extracts volume, issue, and pages from the text.
func extractVolumeIssuePages(ref *ParsedReference, s string) {
	if m := reVolIssuePage.FindStringSubmatch(s); m != nil {
		ref.Volume = m[1]
		ref.Issue = m[2]
		ref.Pages = m[3]
		return
	}
	// Try volume:pages without issue
	// Look for pattern after year; like "2017;3:17065"
	reYearVolPage := regexp.MustCompile(`\d{4};(\d+):(\d+[-–]?\d*)`)
	if m := reYearVolPage.FindStringSubmatch(s); m != nil {
		ref.Volume = m[1]
		ref.Pages = m[2]
	}
}

// parseVancouverAuthors parses "Smith JA, Johnson KL, Williams BR, et al."
// and returns last names ["Smith", "Johnson", "Williams"].
func parseVancouverAuthors(authorStr string) []string {
	// Remove "et al." and trailing punctuation
	authorStr = strings.TrimSpace(authorStr)
	authorStr = regexp.MustCompile(`(?i),?\s*et\s+al\.?`).ReplaceAllString(authorStr, "")
	authorStr = strings.TrimRight(authorStr, "., ")

	if authorStr == "" {
		return nil
	}

	// Split on comma to get individual authors
	parts := strings.Split(authorStr, ",")
	var authors []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		lastName := extractVancouverLastName(p)
		if lastName != "" {
			authors = append(authors, lastName)
		}
	}
	return authors
}

// extractVancouverLastName extracts the last name from a Vancouver author entry.
// "Smith JA" -> "Smith", "Berry-Kravis E" -> "Berry-Kravis", "Des Portes V" -> "Des Portes"
func extractVancouverLastName(entry string) string {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return ""
	}

	// Match: one or more name words followed by uppercase initials at the end
	re := regexp.MustCompile(`^(.+?)\s+[A-Z]{1,4}$`)
	if m := re.FindStringSubmatch(entry); m != nil {
		return strings.TrimSpace(m[1])
	}

	// Fallback: take the first word(s) before last token
	words := strings.Fields(entry)
	if len(words) == 1 {
		return words[0]
	}
	last := words[len(words)-1]
	if isInitials(last) {
		return strings.Join(words[:len(words)-1], " ")
	}
	return words[0]
}

// isInitials checks if a string looks like author initials (1-4 uppercase letters, possibly with periods).
func isInitials(s string) bool {
	cleaned := strings.ReplaceAll(s, ".", "")
	if len(cleaned) == 0 || len(cleaned) > 4 {
		return false
	}
	for _, c := range cleaned {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

// parseAPAAuthors parses "Bear, M. F., Huber, K. M., & Warren, S. T."
// and returns last names ["Bear", "Huber", "Warren"].
func parseAPAAuthors(authorStr string) []string {
	authorStr = regexp.MustCompile(`(?i),?\s*et\s+al\.?`).ReplaceAllString(authorStr, "")
	authorStr = strings.ReplaceAll(authorStr, "&", ",")
	authorStr = strings.TrimSpace(authorStr)
	authorStr = strings.TrimRight(authorStr, "., ")

	if authorStr == "" {
		return nil
	}

	var authors []string
	matches := reAPAAuthor.FindAllStringSubmatch(authorStr, -1)
	for _, m := range matches {
		lastName := strings.TrimSpace(m[1])
		if lastName != "" {
			authors = append(authors, lastName)
		}
	}

	return authors
}

// removeDOIPMID strips DOI and PMID annotations from a string.
func removeDOIPMID(s string) string {
	s = reDOI.ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)doi[:\s]*`).ReplaceAllString(s, "")
	s = rePMID.ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)PMID[:\s]*`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)PMCID[:\s]*PMC\d+`).ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// findTitleEnd finds the index of the period that ends the title in post-year text.
// Returns -1 if not found.
func findTitleEnd(s string) int {
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '.' {
			continue
		}
		// Skip if this looks like an initial
		if i > 0 && runes[i-1] >= 'A' && runes[i-1] <= 'Z' {
			if i < 2 || runes[i-2] == ' ' || runes[i-2] == '.' {
				continue
			}
		}
		// Check if followed by space + uppercase (likely journal name start)
		if i+2 < len(runes) && runes[i+1] == ' ' {
			next := runes[i+2]
			if next >= 'A' && next <= 'Z' {
				return i
			}
		}
	}
	return -1
}

// atoi converts a string to int. Returns 0 on failure.
func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}
