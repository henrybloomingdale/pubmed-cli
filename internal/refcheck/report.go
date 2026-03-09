package refcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// BuildReport constructs a Report from verified references.
func BuildReport(docPath string, results []VerifiedReference, audit *AuditResult) Report {
	r := Report{
		DocumentPath: docPath,
		RefCount:     len(results),
		Results:      results,
		Audit:        audit,
	}
	for _, v := range results {
		r.Summary.Total++
		switch v.Status {
		case StatusVerifiedExact:
			r.Summary.VerifiedExact++
		case StatusVerifiedCorrected:
			r.Summary.VerifiedCorrected++
		case StatusVerifiedByTitle:
			r.Summary.VerifiedByTitle++
		case StatusCandidate:
			r.Summary.Candidate++
		case StatusNotInPubMed:
			r.Summary.NotInPubMed++
		case StatusPossiblyFabricated:
			r.Summary.PossiblyFabricated++
		}
	}
	return r
}

// FormatJSON writes the report as indented JSON.
func FormatJSON(w io.Writer, report Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// FormatHuman writes a human-readable summary of the report.
func FormatHuman(w io.Writer, report Report) error {
	s := report.Summary

	fmt.Fprintf(w, "Reference Check Report: %s\n", report.DocumentPath)
	fmt.Fprintf(w, "═══════════════════════════════════════════════\n\n")
	fmt.Fprintf(w, "Total references: %d\n\n", s.Total)

	fmt.Fprintf(w, "  ✓ Verified (exact):      %d\n", s.VerifiedExact)
	fmt.Fprintf(w, "  ~ Verified (corrected):   %d\n", s.VerifiedCorrected)
	fmt.Fprintf(w, "  ≈ Verified (by title):    %d\n", s.VerifiedByTitle)
	fmt.Fprintf(w, "  ? Candidate match:        %d\n", s.Candidate)
	fmt.Fprintf(w, "  ✗ Not in PubMed:          %d\n", s.NotInPubMed)
	if s.PossiblyFabricated > 0 {
		fmt.Fprintf(w, "  ⚠ Possibly fabricated:    %d\n", s.PossiblyFabricated)
	}
	fmt.Fprintln(w)

	// Detail each reference.
	for _, vr := range report.Results {
		writeRefDetail(w, vr)
	}

	// Audit section.
	if report.Audit != nil {
		writeAuditSection(w, report.Audit)
	}

	return nil
}

func writeRefDetail(w io.Writer, vr VerifiedReference) {
	icon := statusIcon(vr.Status)
	fmt.Fprintf(w, "%s [%d] %s\n", icon, vr.Parsed.Index, truncateStr(vr.Parsed.Raw, 100))
	fmt.Fprintf(w, "   Status: %s (confidence: %.0f%%)\n", vr.Status, vr.Confidence*100)

	if vr.Match != nil {
		fmt.Fprintf(w, "   Match:  PMID %s — %s\n", vr.Match.PMID, truncateStr(vr.Match.Title, 80))
		if vr.Match.DOI != "" {
			fmt.Fprintf(w, "   DOI:    %s\n", vr.Match.DOI)
		}
	}

	for _, c := range vr.Corrections {
		fmt.Fprintf(w, "   Fix:    %s\n", c)
	}

	if vr.Notes != "" {
		fmt.Fprintf(w, "   Note:   %s\n", vr.Notes)
	}

	if len(vr.QueryTiers) > 0 {
		fmt.Fprintf(w, "   Tiers:  %s\n", strings.Join(vr.QueryTiers, " → "))
	}

	fmt.Fprintln(w)
}

func writeAuditSection(w io.Writer, audit *AuditResult) {
	fmt.Fprintf(w, "In-Text Citation Audit\n")
	fmt.Fprintf(w, "───────────────────────────────────────────────\n\n")

	if len(audit.Uncited) > 0 {
		fmt.Fprintf(w, "Uncited references (in reference list but not cited in text):\n")
		for _, idx := range audit.Uncited {
			fmt.Fprintf(w, "  - Reference [%d]\n", idx)
		}
		fmt.Fprintln(w)
	}

	if len(audit.OrphanMarkers) > 0 {
		fmt.Fprintf(w, "Orphan citations (cited in text but no matching reference):\n")
		for _, m := range audit.OrphanMarkers {
			fmt.Fprintf(w, "  - %s\n", m)
		}
		fmt.Fprintln(w)
	}

	if len(audit.Uncited) == 0 && len(audit.OrphanMarkers) == 0 {
		fmt.Fprintf(w, "All references are cited and all citations have matching references.\n\n")
	}
}

// FormatCSV writes the report as CSV.
func FormatCSV(w io.Writer, report Report) error {
	fmt.Fprintln(w, "Index,Status,Confidence,PMID,DOI,Title,Corrections,Notes")
	for _, vr := range report.Results {
		pmid := ""
		doi := ""
		title := ""
		if vr.Match != nil {
			pmid = vr.Match.PMID
			doi = vr.Match.DOI
			title = vr.Match.Title
		}
		corrections := strings.Join(vr.Corrections, "; ")
		fmt.Fprintf(w, "%d,%s,%.2f,%s,%s,%s,%s,%s\n",
			vr.Parsed.Index,
			vr.Status,
			vr.Confidence,
			csvEscape(pmid),
			csvEscape(doi),
			csvEscape(title),
			csvEscape(corrections),
			csvEscape(vr.Notes),
		)
	}
	return nil
}

// FormatRIS writes verified references as RIS citation format.
func FormatRIS(w io.Writer, report Report) error {
	for _, vr := range report.Results {
		if vr.Match == nil {
			continue
		}
		art := vr.Match
		fmt.Fprintln(w, "TY  - JOUR")
		fmt.Fprintf(w, "TI  - %s\n", art.Title)
		for _, a := range art.Authors {
			fmt.Fprintf(w, "AU  - %s, %s\n", a.LastName, a.ForeName)
		}
		fmt.Fprintf(w, "PY  - %s\n", art.Year)
		if art.Journal != "" {
			fmt.Fprintf(w, "JO  - %s\n", art.Journal)
		}
		if art.Volume != "" {
			fmt.Fprintf(w, "VL  - %s\n", art.Volume)
		}
		if art.Issue != "" {
			fmt.Fprintf(w, "IS  - %s\n", art.Issue)
		}
		if art.Pages != "" {
			parts := strings.SplitN(art.Pages, "-", 2)
			fmt.Fprintf(w, "SP  - %s\n", parts[0])
			if len(parts) > 1 {
				fmt.Fprintf(w, "EP  - %s\n", parts[1])
			}
		}
		if art.DOI != "" {
			fmt.Fprintf(w, "DO  - %s\n", art.DOI)
		}
		if art.PMID != "" {
			fmt.Fprintf(w, "AN  - PMID:%s\n", art.PMID)
		}
		if art.Abstract != "" {
			fmt.Fprintf(w, "AB  - %s\n", art.Abstract)
		}
		fmt.Fprintln(w, "ER  - ")
		fmt.Fprintln(w)
	}
	return nil
}

func statusIcon(s VerificationStatus) string {
	switch s {
	case StatusVerifiedExact:
		return "✓"
	case StatusVerifiedCorrected:
		return "~"
	case StatusVerifiedByTitle:
		return "≈"
	case StatusCandidate:
		return "?"
	case StatusNotInPubMed:
		return "✗"
	case StatusPossiblyFabricated:
		return "⚠"
	default:
		return " "
	}
}

func truncateStr(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}
