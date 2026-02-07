// Command pubmed provides a CLI for NCBI PubMed E-utilities.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
	"github.com/henrybloomingdale/pubmed-cli/internal/mesh"
	"github.com/henrybloomingdale/pubmed-cli/internal/ncbi"
	"github.com/henrybloomingdale/pubmed-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	flagJSON   bool
	flagHuman  bool
	flagFull   bool
	flagCSV    string
	flagLimit  int
	flagSort   string
	flagYear   string
	flagType   string
	flagAPIKey string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "pubmed",
	Short: "PubMed E-utilities CLI",
	Long:  `A command-line interface for searching and retrieving articles from NCBI PubMed using the E-utilities API.`,
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output as structured JSON")
	rootCmd.PersistentFlags().BoolVarP(&flagHuman, "human", "H", false, "Rich colorful terminal output")
	rootCmd.PersistentFlags().BoolVar(&flagFull, "full", false, "Show full abstract (with --human)")
	rootCmd.PersistentFlags().StringVar(&flagCSV, "csv", "", "Export results to CSV file")
	rootCmd.PersistentFlags().IntVar(&flagLimit, "limit", 20, "Maximum number of results")
	rootCmd.PersistentFlags().StringVar(&flagSort, "sort", "", "Sort order: relevance, date, or cited")
	rootCmd.PersistentFlags().StringVar(&flagYear, "year", "", "Filter by year range (e.g., 2020-2025)")
	rootCmd.PersistentFlags().StringVar(&flagType, "type", "", "Filter by publication type (review, trial, meta-analysis)")
	rootCmd.PersistentFlags().StringVar(&flagAPIKey, "api-key", "", "NCBI API key (or set NCBI_API_KEY env var)")

	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(citedByCmd)
	rootCmd.AddCommand(referencesCmd)
	rootCmd.AddCommand(relatedCmd)
	rootCmd.AddCommand(meshCmd)
}

func outputCfg() output.OutputConfig {
	return output.OutputConfig{
		JSON:    flagJSON,
		Human:   flagHuman,
		Full:    flagFull,
		CSVFile: flagCSV,
	}
}

func newBaseClient() *ncbi.BaseClient {
	apiKey := flagAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("NCBI_API_KEY")
	}
	var opts []ncbi.Option
	if apiKey != "" {
		opts = append(opts, ncbi.WithAPIKey(apiKey))
	}
	return ncbi.NewBaseClient(opts...)
}

func newEutilsClient() *eutils.Client {
	return eutils.NewClientWithBase(newBaseClient())
}

func newMeshClient() *mesh.Client {
	return mesh.NewClient(newBaseClient())
}

func buildQuery(args []string) string {
	query := strings.Join(args, " ")

	// Add publication type filter — multi-word types must be quoted.
	if flagType != "" {
		typeMap := map[string]string{
			"review":        `"review"[pt]`,
			"trial":         `"clinical trial"[pt]`,
			"meta-analysis": `"meta-analysis"[pt]`,
			"randomized":    `"randomized controlled trial"[pt]`,
			"case-report":   `"case reports"[pt]`,
		}
		if mapped, ok := typeMap[strings.ToLower(flagType)]; ok {
			query += " AND " + mapped
		} else {
			query += fmt.Sprintf(` AND "%s"[pt]`, flagType)
		}
	}

	// Add year filter
	if flagYear != "" {
		parts := strings.SplitN(flagYear, "-", 2)
		if len(parts) == 2 {
			query += fmt.Sprintf(" AND %s:%s[pdat]", parts[0], parts[1])
		} else {
			query += fmt.Sprintf(" AND %s[pdat]", parts[0])
		}
	}

	return query
}

// searchCmd implements the search subcommand.
var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search PubMed with Boolean/MeSH queries",
	Long:  `Search PubMed using Boolean operators and MeSH terms. Returns PMIDs and result counts.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newEutilsClient()
		query := buildQuery(args)
		cfg := outputCfg()

		opts := &eutils.SearchOptions{
			Limit: flagLimit,
			Sort:  flagSort,
		}

		if flagYear != "" {
			parts := strings.SplitN(flagYear, "-", 2)
			if len(parts) == 2 {
				opts.MinDate = parts[0]
				opts.MaxDate = parts[1]
			}
		}

		result, err := client.Search(cmd.Context(), query, opts)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		// Auto-fetch articles for --human or --csv (rich table/export)
		var articles []eutils.Article
		if (cfg.Human || cfg.CSVFile != "") && len(result.IDs) > 0 {
			articles, err = client.Fetch(cmd.Context(), result.IDs)
			if err != nil {
				// Non-fatal: fall back to PMID-only display
				fmt.Fprintf(os.Stderr, "Warning: could not fetch article details: %v\n", err)
				articles = nil
			}
		}

		return output.FormatSearchResult(os.Stdout, result, articles, cfg)
	},
}

// fetchCmd implements the fetch subcommand.
var fetchCmd = &cobra.Command{
	Use:   "fetch <pmid> [pmid...]",
	Short: "Fetch full article details",
	Long:  `Retrieve full article details including abstract, authors, DOI, and MeSH terms for one or more PMIDs.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newEutilsClient()

		articles, err := client.Fetch(cmd.Context(), args)
		if err != nil {
			return fmt.Errorf("fetch failed: %w", err)
		}

		return output.FormatArticles(os.Stdout, articles, outputCfg())
	},
}

// citedByCmd implements the cited-by subcommand.
var citedByCmd = &cobra.Command{
	Use:   "cited-by <pmid>",
	Short: "Find papers that cite this article",
	Long:  `Find papers in PubMed that cite the given article.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newEutilsClient()

		result, err := client.CitedBy(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("cited-by lookup failed: %w", err)
		}

		return formatLinkResults(cmd, client, result, "cited-by")
	},
}

// referencesCmd implements the references subcommand.
var referencesCmd = &cobra.Command{
	Use:   "references <pmid>",
	Short: "Find papers cited by this article",
	Long:  `List the references cited by the given article.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newEutilsClient()

		result, err := client.References(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("references lookup failed: %w", err)
		}

		return formatLinkResults(cmd, client, result, "references")
	},
}

// relatedCmd implements the related subcommand.
var relatedCmd = &cobra.Command{
	Use:   "related <pmid>",
	Short: "Find similar articles",
	Long:  `Find articles similar to the given article, ranked by relevance score.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newEutilsClient()

		result, err := client.Related(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("related articles lookup failed: %w", err)
		}

		return formatLinkResults(cmd, client, result, "related")
	},
}

// formatLinkResults handles output for link commands, fetching article details for human mode.
func formatLinkResults(cmd *cobra.Command, client *eutils.Client, result *eutils.LinkResult, linkType string) error {
	cfg := outputCfg()

	// For JSON or plain text, just output the links
	if cfg.JSON || !cfg.Human {
		return output.FormatLinks(os.Stdout, result, linkType, cfg)
	}

	// For human mode, fetch article details to show titles
	if len(result.Links) == 0 {
		return output.FormatLinks(os.Stdout, result, linkType, cfg)
	}

	// Collect PMIDs to fetch (respect limit)
	limit := flagLimit
	if limit > len(result.Links) {
		limit = len(result.Links)
	}
	pmids := make([]string, limit)
	for i := 0; i < limit; i++ {
		pmids[i] = result.Links[i].ID
	}

	// Fetch article details
	articles, err := client.Fetch(cmd.Context(), pmids)
	if err != nil {
		// Fall back to PMID-only display if fetch fails
		return output.FormatLinks(os.Stdout, result, linkType, cfg)
	}

	// Build a map of PMID -> article for ordering
	articleMap := make(map[string]eutils.Article)
	for _, a := range articles {
		articleMap[a.PMID] = a
	}

	// Create enriched link result with scores preserved
	return output.FormatLinksWithArticles(os.Stdout, result, articles, articleMap, linkType, limit)
}

// meshCmd implements the mesh subcommand.
var meshCmd = &cobra.Command{
	Use:   "mesh <term>",
	Short: "Look up a MeSH term",
	Long:  `Search for a MeSH (Medical Subject Headings) term and display its record including tree numbers, scope note, and synonyms.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newMeshClient()
		term := strings.Join(args, " ")

		record, err := client.Lookup(cmd.Context(), term)
		if err != nil {
			return fmt.Errorf("MeSH lookup failed: %w", err)
		}

		return output.FormatMeSHRecord(os.Stdout, record, outputCfg())
	},
}
