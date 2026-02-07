package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
	"github.com/henrybloomingdale/pubmed-cli/internal/mesh"
)

// --- Styles ---

var (
	cyan       = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	bold       = lipgloss.NewStyle().Bold(true)
	dim        = lipgloss.NewStyle().Faint(true)
	green      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	yellow     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	magenta    = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
	boxStyle   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("6")).
			Padding(0, 1)
)

// truncate cuts a string to maxLen characters, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// --- Search ---

func formatSearchHuman(w io.Writer, result *eutils.SearchResult, articles []eutils.Article) error {
	if result.Count == 0 {
		fmt.Fprintln(w, "🔬 No results found.")
		return nil
	}

	// Header
	header := fmt.Sprintf("🔬 Found %d results", result.Count)
	if len(result.IDs) < result.Count {
		header += fmt.Sprintf(" (showing %d)", len(result.IDs))
	}
	fmt.Fprintln(w, bold.Render(header))

	if result.QueryTranslation != "" {
		fmt.Fprintf(w, "   Query: %s\n", dim.Render(result.QueryTranslation))
	}
	fmt.Fprintln(w)

	if len(articles) > 0 {
		// Rich table with article info
		byPMID := make(map[string]eutils.Article, len(articles))
		for _, a := range articles {
			byPMID[a.PMID] = a
		}

		var rows [][]string
		for _, id := range result.IDs {
			a, ok := byPMID[id]
			if !ok {
				rows = append(rows, []string{cyan.Render(id), "", "", ""})
				continue
			}
			pubType := ""
			if len(a.PublicationTypes) > 0 {
				pubType = a.PublicationTypes[0]
			}
			rows = append(rows, []string{
				cyan.Render(a.PMID),
				bold.Render(truncate(a.Title, 50)),
				a.Year,
				pubType,
			})
		}

		t := table.New().
			Headers("PMID", "Title", "Year", "Type").
			Rows(rows...).
			Border(lipgloss.NormalBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("8"))).
			StyleFunc(func(row, col int) lipgloss.Style {
				if row == table.HeaderRow {
					return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
				}
				return lipgloss.NewStyle()
			})

		fmt.Fprintln(w, t.Render())
	} else {
		// Just PMIDs
		var rows [][]string
		for i, id := range result.IDs {
			rows = append(rows, []string{fmt.Sprintf("%d", i+1), cyan.Render(id)})
		}

		t := table.New().
			Headers("#", "PMID").
			Rows(rows...).
			Border(lipgloss.NormalBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("8"))).
			StyleFunc(func(row, col int) lipgloss.Style {
				if row == table.HeaderRow {
					return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
				}
				return lipgloss.NewStyle()
			})

		fmt.Fprintln(w, t.Render())
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, dim.Render("💾 Use --csv output.csv to export"))
	return nil
}

// --- Fetch / Articles ---

func formatArticlesHuman(w io.Writer, articles []eutils.Article, full bool) error {
	if len(articles) == 0 {
		fmt.Fprintln(w, "No articles found.")
		return nil
	}

	for i, a := range articles {
		if i > 0 {
			fmt.Fprintln(w)
		}

		// Title card
		titleLine := bold.Render(a.Title)
		meta := cyan.Render("PMID: " + a.PMID)
		if a.Year != "" {
			meta += dim.Render(" · ") + a.Year
		}
		card := titleLine + "\n" + meta
		fmt.Fprintln(w, boxStyle.Render(card))
		fmt.Fprintln(w)

		// Fields
		if len(a.Authors) > 0 {
			names := make([]string, len(a.Authors))
			for j, au := range a.Authors {
				names[j] = au.FullName()
			}
			fmt.Fprintf(w, "  %s %s\n", labelStyle.Render("Authors:"), strings.Join(names, ", "))
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
		fmt.Fprintf(w, "  %s %s\n", labelStyle.Render("Journal:"), citation)

		if a.DOI != "" {
			fmt.Fprintf(w, "  %s %s\n", labelStyle.Render("DOI:"), yellow.Render(a.DOI))
		}
		if len(a.PublicationTypes) > 0 {
			fmt.Fprintf(w, "  %s %s\n", labelStyle.Render("Type:"), strings.Join(a.PublicationTypes, ", "))
		}

		// MeSH terms
		if len(a.MeSHTerms) > 0 {
			var terms []string
			for _, m := range a.MeSHTerms {
				t := m.Descriptor
				if m.MajorTopic {
					t = green.Render("*" + t)
				}
				terms = append(terms, t)
			}
			fmt.Fprintf(w, "  %s %s\n", labelStyle.Render("MeSH:"), strings.Join(terms, ", "))
		}

		// Abstract
		if a.Abstract != "" {
			fmt.Fprintln(w)
			fmt.Fprintf(w, "  %s\n", labelStyle.Render("Abstract:"))
			abstract := a.Abstract
			if !full && len(abstract) > 500 {
				abstract = abstract[:497] + "..."
				fmt.Fprintf(w, "  %s\n", abstract)
				fmt.Fprintf(w, "  %s\n", dim.Render("[use --full for complete abstract]"))
			} else {
				fmt.Fprintf(w, "  %s\n", abstract)
			}
		}
	}

	return nil
}

// --- Links ---

func formatLinksHuman(w io.Writer, result *eutils.LinkResult, linkType string) error {
	emoji := "🔗"
	title := linkType
	switch linkType {
	case "cited-by":
		emoji = "📚"
		title = "Cited By"
	case "references":
		emoji = "📖"
		title = "References"
	case "related":
		emoji = "🔍"
		title = "Related Articles"
	}

	if len(result.Links) == 0 {
		fmt.Fprintf(w, "%s No %s results for PMID %s.\n", emoji, linkType, cyan.Render(result.SourceID))
		return nil
	}

	fmt.Fprintf(w, "%s %s for PMID %s (%d results)\n\n",
		emoji,
		bold.Render(title),
		cyan.Render(result.SourceID),
		len(result.Links))

	var rows [][]string
	hasScores := false
	for _, link := range result.Links {
		if link.Score > 0 {
			hasScores = true
			break
		}
	}

	for i, link := range result.Links {
		row := []string{
			fmt.Sprintf("%d", i+1),
			cyan.Render(link.ID),
		}
		if hasScores {
			if link.Score > 0 {
				row = append(row, dim.Render(fmt.Sprintf("%d", link.Score)))
			} else {
				row = append(row, "")
			}
		}
		rows = append(rows, row)
	}

	headers := []string{"#", "PMID"}
	if hasScores {
		headers = append(headers, "Score")
	}

	t := table.New().
		Headers(headers...).
		Rows(rows...).
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("8"))).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
			}
			return lipgloss.NewStyle()
		})

	fmt.Fprintln(w, t.Render())
	return nil
}

// --- MeSH ---

func formatMeSHHuman(w io.Writer, record *mesh.MeSHRecord) error {
	// Name + UI header
	fmt.Fprintf(w, "🏷️  %s  %s\n\n", bold.Render(record.Name), dim.Render(record.UI))

	// Tree numbers
	if len(record.TreeNumbers) > 0 {
		fmt.Fprintf(w, "  %s\n", labelStyle.Render("Tree Numbers:"))
		for _, tn := range record.TreeNumbers {
			fmt.Fprintf(w, "    %s %s\n", magenta.Render("├"), tn)
		}
		fmt.Fprintln(w)
	}

	// Scope note
	if record.ScopeNote != "" {
		fmt.Fprintf(w, "  %s\n", labelStyle.Render("Scope Note:"))
		// Word-wrap at ~80 chars
		wrapped := wordWrap(record.ScopeNote, 76)
		for _, line := range strings.Split(wrapped, "\n") {
			fmt.Fprintf(w, "    %s\n", line)
		}
		fmt.Fprintln(w)
	}

	// Entry terms (synonyms)
	if len(record.EntryTerms) > 0 {
		fmt.Fprintf(w, "  %s ", labelStyle.Render("Synonyms:"))
		colored := make([]string, len(record.EntryTerms))
		for i, et := range record.EntryTerms {
			colored[i] = yellow.Render(et)
		}
		fmt.Fprintln(w, strings.Join(colored, ", "))
		fmt.Fprintln(w)
	}

	// Annotation
	if record.Annotation != "" {
		fmt.Fprintf(w, "  %s %s\n", labelStyle.Render("Annotation:"), record.Annotation)
	}

	return nil
}

// wordWrap wraps text at the given width, breaking at spaces.
func wordWrap(text string, width int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	current := words[0]

	for _, word := range words[1:] {
		if len(current)+1+len(word) > width {
			lines = append(lines, current)
			current = word
		} else {
			current += " " + word
		}
	}
	lines = append(lines, current)
	return strings.Join(lines, "\n")
}
