package main

import (
	"fmt"
	"os"

	"github.com/henrybloomingdale/pubmed-cli/internal/refcheck"
	"github.com/spf13/cobra"
)

var (
	flagAuditText bool
	flagRISOut    string
	flagCSVOut    string
)

var refcheckCmd = &cobra.Command{
	Use:   "refcheck <document.docx>",
	Short: "Verify document references against PubMed",
	Long: `Extract references from a .docx document, verify each against PubMed,
detect corrections, hallucinations, and optionally audit in-text citations.

Requires docx-review to be installed (https://github.com/drpedapati/docx-review).

Output formats:
  --json        Structured JSON report (default)
  --human       Human-readable terminal report
  --csv-out     Export to CSV file
  --ris-out     Export verified references as RIS citations`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		docxPath := args[0]

		// Verify docx-review is available.
		if _, err := refcheck.FindDocxReview(); err != nil {
			return fmt.Errorf("docx-review is required: %w", err)
		}

		// Verify input file exists.
		if _, err := os.Stat(docxPath); err != nil {
			return fmt.Errorf("cannot access %q: %w", docxPath, err)
		}

		ctx := cmd.Context()

		// Step 1: Extract document content via docx-review.
		fmt.Fprintf(os.Stderr, "Extracting document content...\n")
		doc, err := refcheck.ExtractFromFile(ctx, docxPath)
		if err != nil {
			return fmt.Errorf("failed to extract document: %w", err)
		}

		// Step 2: Split body and references.
		bodyText, refsText := refcheck.SplitBodyAndReferences(doc)
		if refsText == "" {
			return fmt.Errorf("no references section found in %q", docxPath)
		}

		// Step 3: Parse references.
		fmt.Fprintf(os.Stderr, "Parsing references...\n")
		refs, err := refcheck.ParseReferences(refsText)
		if err != nil {
			return fmt.Errorf("failed to parse references: %w", err)
		}
		if len(refs) == 0 {
			return fmt.Errorf("no references found in references section")
		}
		fmt.Fprintf(os.Stderr, "Found %d references\n", len(refs))

		// Step 4: Resolve each reference against PubMed.
		fmt.Fprintf(os.Stderr, "Verifying against PubMed...\n")
		client := newEutilsClient()
		resolver := refcheck.NewResolver(client)
		results := resolver.ResolveAll(ctx, refs)

		// Step 5: Hallucination detection on unresolved references.
		detector := refcheck.NewHallucinationDetector(client)
		for i := range results {
			detector.Check(ctx, results[i].Parsed, &results[i])
		}

		// Step 6: Optional in-text citation audit.
		var audit *refcheck.AuditResult
		if flagAuditText {
			fmt.Fprintf(os.Stderr, "Auditing in-text citations...\n")
			a := refcheck.AuditCitations(bodyText, refs)
			audit = &a
		}

		// Step 7: Build and output report.
		report := refcheck.BuildReport(docxPath, results, audit)

		// Export RIS if requested.
		if flagRISOut != "" {
			f, err := os.Create(flagRISOut)
			if err != nil {
				return fmt.Errorf("failed to create RIS file: %w", err)
			}
			defer f.Close()
			if err := refcheck.FormatRIS(f, report); err != nil {
				return fmt.Errorf("failed to write RIS: %w", err)
			}
			fmt.Fprintf(os.Stderr, "RIS exported to %s\n", flagRISOut)
		}

		// Export CSV if requested.
		if flagCSVOut != "" {
			f, err := os.Create(flagCSVOut)
			if err != nil {
				return fmt.Errorf("failed to create CSV file: %w", err)
			}
			defer f.Close()
			if err := refcheck.FormatCSV(f, report); err != nil {
				return fmt.Errorf("failed to write CSV: %w", err)
			}
			fmt.Fprintf(os.Stderr, "CSV exported to %s\n", flagCSVOut)
		}

		// Primary output.
		cfg := outputCfg()
		if cfg.Human {
			return refcheck.FormatHuman(os.Stdout, report)
		}
		return refcheck.FormatJSON(os.Stdout, report)
	},
}

func init() {
	refcheckCmd.Flags().BoolVar(&flagAuditText, "audit-text", false, "Audit in-text citations against reference list")
	refcheckCmd.Flags().StringVar(&flagRISOut, "ris-out", "", "Export verified references to RIS file")
	refcheckCmd.Flags().StringVar(&flagCSVOut, "csv-out", "", "Export report to CSV file")
}
