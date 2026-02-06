package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/henrybloomingdale/pubmed-cli/internal/llm"
	"github.com/henrybloomingdale/pubmed-cli/internal/qa"
	"github.com/spf13/cobra"
)

var (
	qaFlagConfidence int
	qaFlagRetrieval  bool
	qaFlagParametric bool
	qaFlagExplain    bool
	qaFlagModel      string
	qaFlagBaseURL    string
	qaFlagClaude     bool
)

func init() {
	qaCmd.Flags().IntVar(&qaFlagConfidence, "confidence", 7, "Confidence threshold for parametric answers (1-10)")
	qaCmd.Flags().BoolVar(&qaFlagRetrieval, "retrieve", false, "Force retrieval (skip confidence check)")
	qaCmd.Flags().BoolVar(&qaFlagParametric, "parametric", false, "Force parametric (never retrieve)")
	qaCmd.Flags().BoolVarP(&qaFlagExplain, "explain", "e", false, "Show reasoning and sources")
	qaCmd.Flags().StringVar(&qaFlagModel, "model", "", "LLM model (default: gpt-4o or LLM_MODEL env)")
	qaCmd.Flags().StringVar(&qaFlagBaseURL, "llm-url", "", "LLM API base URL (default: LLM_BASE_URL env)")
	qaCmd.Flags().BoolVar(&qaFlagClaude, "claude", false, "Use Claude API (requires ANTHROPIC_API_KEY)")

	rootCmd.AddCommand(qaCmd)
}

var qaCmd = &cobra.Command{
	Use:   "qa <question>",
	Short: "Answer biomedical yes/no questions with adaptive retrieval",
	Long: `Answers biomedical questions using adaptive retrieval:

1. Detects if question requires novel (post-training) knowledge
2. Checks model confidence for established knowledge
3. Retrieves from PubMed only when necessary
4. Minifies abstracts to preserve key findings

Examples:
  pubmed qa "Does CBT help hypertension-related anxiety?"
  pubmed qa --explain "According to 2025 studies, does SGLT-2 reduce liver fibrosis?"
  pubmed qa --retrieve "Is metformin effective for PCOS?"

Environment variables:
  LLM_API_KEY   - API key for LLM (or OPENAI_API_KEY)
  LLM_BASE_URL  - Base URL for OpenAI-compatible API
  LLM_MODEL     - Model name (default: gpt-4o)`,
	Args: cobra.MinimumNArgs(1),
	RunE: runQA,
}

// LLMCompleter is the interface both OpenAI and Claude clients implement.
type LLMCompleter interface {
	Complete(ctx context.Context, prompt string, maxTokens int) (string, error)
}

func runQA(cmd *cobra.Command, args []string) error {
	question := strings.Join(args, " ")

	// Build LLM client
	var llmClient LLMCompleter
	var err error

	if qaFlagClaude {
		// Use Claude via OAuth tokens from keychain
		llmClient, err = llm.NewClaudeClient(qaFlagModel)
		if err != nil {
			return fmt.Errorf("claude setup: %w", err)
		}
	} else {
		// Use OpenAI-compatible API
		var llmOpts []llm.Option
		if qaFlagModel != "" {
			llmOpts = append(llmOpts, llm.WithModel(qaFlagModel))
		}
		if qaFlagBaseURL != "" {
			llmOpts = append(llmOpts, llm.WithBaseURL(qaFlagBaseURL))
		}
		llmClient = llm.NewClient(llmOpts...)
	}

	// Build QA engine
	cfg := qa.DefaultConfig()
	cfg.ConfidenceThreshold = qaFlagConfidence
	cfg.ForceRetrieval = qaFlagRetrieval
	cfg.ForceParametric = qaFlagParametric
	cfg.Verbose = qaFlagExplain

	engine := qa.NewEngine(llmClient, newEutilsClient(), cfg)

	// Get answer
	result, err := engine.Answer(cmd.Context(), question)
	if err != nil {
		return fmt.Errorf("qa failed: %w", err)
	}

	// Output
	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if qaFlagExplain || flagHuman {
		printExplainedResult(result)
	} else {
		fmt.Println(result.Answer)
	}

	return nil
}

func printExplainedResult(r *qa.Result) {
	// Strategy icon
	stratIcon := "🧠"
	if r.Strategy == qa.StrategyRetrieval {
		stratIcon = "🔍"
	}

	fmt.Printf("\n%s Answer: %s\n", stratIcon, strings.ToUpper(r.Answer))
	fmt.Printf("   Strategy: %s\n", r.Strategy)

	if r.NovelDetected {
		fmt.Println("   Novel knowledge detected: yes")
	}
	if r.Confidence > 0 {
		fmt.Printf("   Confidence: %d/10\n", r.Confidence)
	}
	if len(r.SourcePMIDs) > 0 {
		fmt.Printf("   Sources: %s\n", strings.Join(r.SourcePMIDs, ", "))
	}
	if r.MinifiedContext != "" && len(r.MinifiedContext) < 500 {
		fmt.Printf("\n   Context:\n   %s\n", strings.ReplaceAll(r.MinifiedContext, "\n", "\n   "))
	}
	fmt.Println()
}
