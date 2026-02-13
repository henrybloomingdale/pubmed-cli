// Package output provides formatting for PubMed CLI output.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
	"github.com/henrybloomingdale/pubmed-cli/internal/mesh"
)

// OutputConfig controls which output mode(s) are active.
type OutputConfig struct {
	JSON    bool   // Structured JSON
	Human   bool   // Rich terminal output with color
	Full    bool   // Show full abstract (human mode)
	CSVFile string // Export results to this CSV path (works alongside any mode)
	RISFile string // Export results to this RIS path (works alongside any mode)
}

// FormatSearchResult writes search results.
// articles may be non-nil when --human or --csv triggers an auto-fetch.
func FormatSearchResult(w io.Writer, result *eutils.SearchResult, articles []eutils.Article, cfg OutputConfig) error {
	if cfg.CSVFile != "" {
		if err := writeSearchCSV(cfg.CSVFile, result, articles); err != nil {
			return fmt.Errorf("CSV export failed: %w", err)
		}
	}
	if cfg.JSON {
		return writeJSON(w, result)
	}
	if cfg.Human {
		return formatSearchHuman(w, result, articles)
	}
	return formatSearchPlain(w, result)
}

// FormatArticles writes article details.
func FormatArticles(w io.Writer, articles []eutils.Article, cfg OutputConfig) error {
	if cfg.CSVFile != "" {
		if err := writeArticlesCSV(cfg.CSVFile, articles); err != nil {
			return fmt.Errorf("CSV export failed: %w", err)
		}
	}
	if cfg.RISFile != "" {
		if err := writeArticlesRIS(cfg.RISFile, articles); err != nil {
			return fmt.Errorf("RIS export failed: %w", err)
		}
	}
	if cfg.JSON {
		return writeJSON(w, articles)
	}
	if cfg.Human {
		return formatArticlesHuman(w, articles, cfg.Full)
	}
	return formatArticlesPlain(w, articles)
}

// FormatLinks writes link results.
func FormatLinks(w io.Writer, result *eutils.LinkResult, linkType string, cfg OutputConfig) error {
	if cfg.CSVFile != "" {
		if err := writeLinksCSV(cfg.CSVFile, result); err != nil {
			return fmt.Errorf("CSV export failed: %w", err)
		}
	}
	if cfg.JSON {
		return writeJSON(w, result)
	}
	if cfg.Human {
		return formatLinksHuman(w, result, linkType)
	}
	return formatLinksPlain(w, result, linkType)
}

// FormatMeSHRecord writes a MeSH record.
func FormatMeSHRecord(w io.Writer, record *mesh.MeSHRecord, cfg OutputConfig) error {
	if cfg.CSVFile != "" {
		if err := writeMeSHCSV(cfg.CSVFile, record); err != nil {
			return fmt.Errorf("CSV export failed: %w", err)
		}
	}
	if cfg.JSON {
		return writeJSON(w, record)
	}
	if cfg.Human {
		return formatMeSHHuman(w, record)
	}
	return formatMeSHPlain(w, record)
}

// --- Plain text formatters (default) ---

func formatSearchPlain(w io.Writer, result *eutils.SearchResult) error {
	if result.Count == 0 {
		fmt.Fprintln(w, "No results found.")
		return nil
	}

	fmt.Fprintf(w, "Found %d results", result.Count)
	if len(result.IDs) < result.Count {
		fmt.Fprintf(w, " (showing %d)", len(result.IDs))
	}
	fmt.Fprintln(w)

	if result.QueryTranslation != "" {
		fmt.Fprintf(w, "Query: %s\n", result.QueryTranslation)
	}
	fmt.Fprintln(w)

	for i, id := range result.IDs {
		fmt.Fprintf(w, "  %d. PMID: %s\n", i+1, id)
	}

	return nil
}

func formatArticlesPlain(w io.Writer, articles []eutils.Article) error {
	if len(articles) == 0 {
		fmt.Fprintln(w, "No articles found.")
		return nil
	}

	for i, a := range articles {
		if i > 0 {
			fmt.Fprintf(w, "\n%s\n\n", strings.Repeat("─", 80))
		}

		fmt.Fprintf(w, "PMID: %s\n", a.PMID)
		fmt.Fprintf(w, "Title: %s\n", a.Title)

		if len(a.Authors) > 0 {
			names := make([]string, len(a.Authors))
			for j, au := range a.Authors {
				names[j] = au.FullName()
			}
			fmt.Fprintf(w, "Authors: %s\n", strings.Join(names, ", "))
		}

		citation := a.Journal
		if a.Volume != "" {
			citation += " " + a.Volume
			if a.Issue != "" {
				citation += "(" + a.Issue + ")"
			}
		}
		if a.Pages != "" {
			citation += ":" + a.Pages
		}
		if a.Year != "" {
			citation += " (" + a.Year + ")"
		}
		fmt.Fprintf(w, "Journal: %s\n", citation)

		if a.DOI != "" {
			fmt.Fprintf(w, "DOI: %s\n", a.DOI)
		}
		if a.PMCID != "" {
			fmt.Fprintf(w, "PMCID: %s\n", a.PMCID)
		}
		if len(a.PublicationTypes) > 0 {
			fmt.Fprintf(w, "Type: %s\n", strings.Join(a.PublicationTypes, ", "))
		}
		if a.Abstract != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Abstract:")
			fmt.Fprintln(w, a.Abstract)
		}
		if len(a.MeSHTerms) > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "MeSH Terms:")
			for _, m := range a.MeSHTerms {
				marker := "  "
				if m.MajorTopic {
					marker = "* "
				}
				term := m.Descriptor
				if len(m.Qualifiers) > 0 {
					term += " / " + strings.Join(m.Qualifiers, ", ")
				}
				fmt.Fprintf(w, "  %s%s\n", marker, term)
			}
		}
	}

	return nil
}

func formatLinksPlain(w io.Writer, result *eutils.LinkResult, linkType string) error {
	if len(result.Links) == 0 {
		fmt.Fprintf(w, "No %s results for PMID %s.\n", linkType, result.SourceID)
		return nil
	}

	title := linkType
	switch linkType {
	case "cited-by":
		title = "Cited By"
	case "references":
		title = "References"
	case "related":
		title = "Related Articles"
	}

	fmt.Fprintf(w, "%s for PMID %s (%d results):\n\n", title, result.SourceID, len(result.Links))

	for i, link := range result.Links {
		if link.Score > 0 {
			fmt.Fprintf(w, "  %d. PMID: %s (score: %d)\n", i+1, link.ID, link.Score)
		} else {
			fmt.Fprintf(w, "  %d. PMID: %s\n", i+1, link.ID)
		}
	}

	return nil
}

func formatMeSHPlain(w io.Writer, record *mesh.MeSHRecord) error {
	fmt.Fprintf(w, "MeSH Term: %s\n", record.Name)
	fmt.Fprintf(w, "UI: %s\n", record.UI)

	if len(record.TreeNumbers) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Tree Numbers:")
		for _, tn := range record.TreeNumbers {
			fmt.Fprintf(w, "  %s\n", tn)
		}
	}

	if record.ScopeNote != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Scope Note:")
		fmt.Fprintf(w, "  %s\n", record.ScopeNote)
	}

	if len(record.EntryTerms) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Entry Terms (synonyms):")
		for _, et := range record.EntryTerms {
			fmt.Fprintf(w, "  - %s\n", et)
		}
	}

	if record.Annotation != "" {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Annotation: %s\n", record.Annotation)
	}

	return nil
}

func writeJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
