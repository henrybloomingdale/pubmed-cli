// Command pubmed provides a CLI for NCBI PubMed E-utilities.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
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
	flagRIS    string
	flagLimit  int
	flagSort   string
	flagYear   string
	flagType   string
	flagAPIKey string
)

var allowedSorts = map[string]struct{}{
	"relevance": {},
	"date":      {},
	"cited":     {},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "pubmed",
	Short: "PubMed E-utilities CLI",
	Long:  `A command-line interface for searching and retrieving articles from NCBI PubMed using the E-utilities API.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return validateGlobalFlags(cmd)
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output as structured JSON")
	rootCmd.PersistentFlags().BoolVarP(&flagHuman, "human", "H", false, "Rich colorful terminal output")
	rootCmd.PersistentFlags().BoolVar(&flagFull, "full", false, "Show full abstract (with --human)")
	rootCmd.PersistentFlags().StringVar(&flagCSV, "csv", "", "Export results to CSV file")
	rootCmd.PersistentFlags().StringVar(&flagRIS, "ris", "", "Export results to RIS file")
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
		RISFile: flagRIS,
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

	return query
}

func parseYearRange(value string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(value), "-", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", fmt.Errorf("year must be YYYY or YYYY-YYYY")
	}

	if len(parts[0]) != 4 {
		return "", "", fmt.Errorf("year must be YYYY or YYYY-YYYY")
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("year must be numeric")
	}

	if len(parts) == 1 {
		return parts[0], parts[0], nil
	}

	if parts[1] == "" || len(parts[1]) != 4 {
		return "", "", fmt.Errorf("year range must be YYYY-YYYY")
	}
	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("year range must be numeric")
	}
	if end < start {
		return "", "", fmt.Errorf("year range must be ascending")
	}

	return parts[0], parts[1], nil
}

func validateGlobalFlags(cmd *cobra.Command) error {
	if flagLimit <= 0 {
		return fmt.Errorf("--limit must be greater than 0")
	}

	if flagSort != "" {
		if _, ok := allowedSorts[strings.ToLower(flagSort)]; !ok {
			return fmt.Errorf("--sort must be one of: relevance, date, cited")
		}
	}

	if flagYear != "" {
		if _, _, err := parseYearRange(flagYear); err != nil {
			return fmt.Errorf("--year %q is invalid: %w", flagYear, err)
		}
	}

	if flagRIS != "" {
		switch cmd.Name() {
		case "search", "mesh":
			return fmt.Errorf("--ris is not supported for %q; use fetch, cited-by, references, or related", cmd.Name())
		}
	}

	return nil
}

func validatePMID(pmid string) error {
	if pmid == "" {
		return fmt.Errorf("PMID cannot be empty")
	}

	for _, r := range pmid {
		if r < '0' || r > '9' {
			return fmt.Errorf("PMID %q is invalid: only digits are allowed", pmid)
		}
	}

	return nil
}

func parsePMIDArg(pmidArg string) ([]string, error) {
	raw := strings.Split(pmidArg, ",")
	if len(raw) == 0 {
		return nil, fmt.Errorf("PMID cannot be empty")
	}

	pmids := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("PMID cannot be empty")
		}
		if err := validatePMID(p); err != nil {
			return nil, err
		}
		pmids = append(pmids, p)
	}

	return pmids, nil
}

func normalizePMIDArgs(args []string) ([]string, error) {
	normalized := make([]string, 0, len(args))
	for _, arg := range args {
		parts, err := parsePMIDArg(arg)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, parts...)
	}

	return normalized, nil
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
			Sort:  strings.ToLower(flagSort),
		}

		if flagYear != "" {
			minDate, maxDate, err := parseYearRange(flagYear)
			if err != nil {
				return fmt.Errorf("invalid --year value %q: %w", flagYear, err)
			}
			opts.MinDate = minDate
			opts.MaxDate = maxDate
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
		pmids, err := normalizePMIDArgs(args)
		if err != nil {
			return fmt.Errorf("invalid PMID(s): %w", err)
		}

		articles, err := client.Fetch(cmd.Context(), pmids)
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
		if err := validatePMID(args[0]); err != nil {
			return fmt.Errorf("invalid PMID: %w", err)
		}

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
		if err := validatePMID(args[0]); err != nil {
			return fmt.Errorf("invalid PMID: %w", err)
		}

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
		if err := validatePMID(args[0]); err != nil {
			return fmt.Errorf("invalid PMID: %w", err)
		}

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

	// If RIS export is requested with no links, still create/clear the target file.
	if len(result.Links) == 0 && cfg.RISFile != "" {
		if err := output.FormatArticles(io.Discard, []eutils.Article{}, output.OutputConfig{RISFile: cfg.RISFile}); err != nil {
			return fmt.Errorf("RIS export failed: %w", err)
		}
	}

	needsArticles := cfg.Human || cfg.RISFile != ""

	var (
		articles []eutils.Article
		limit    int
		fetchErr error
	)

	// For human and/or RIS mode, fetch article details for linked IDs.
	if needsArticles && len(result.Links) > 0 {
		limit = flagLimit
		if limit > len(result.Links) {
			limit = len(result.Links)
		}
		pmids := make([]string, limit)
		for i := 0; i < limit; i++ {
			pmids[i] = result.Links[i].ID
		}

		articles, fetchErr = client.Fetch(cmd.Context(), pmids)
	}

	if cfg.RISFile != "" {
		if fetchErr != nil {
			return fmt.Errorf("failed to export RIS: %w", fetchErr)
		}
		if err := output.FormatArticles(io.Discard, articles, output.OutputConfig{RISFile: cfg.RISFile}); err != nil {
			return fmt.Errorf("RIS export failed: %w", err)
		}
	}

	// For JSON or plain text, output links after optional RIS export.
	if cfg.JSON || !cfg.Human {
		return output.FormatLinks(os.Stdout, result, linkType, cfg)
	}

	if len(result.Links) == 0 {
		return output.FormatLinks(os.Stdout, result, linkType, cfg)
	}

	if fetchErr != nil {
		// Fall back to PMID-only display if fetch fails in human mode.
		return output.FormatLinks(os.Stdout, result, linkType, cfg)
	}

	articleMap := make(map[string]eutils.Article)
	for _, a := range articles {
		articleMap[a.PMID] = a
	}

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
