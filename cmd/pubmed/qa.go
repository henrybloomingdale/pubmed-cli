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
	qaFlagCodex      bool
	qaFlagOpus       bool
	qaFlagUnsafe     bool
)

func init() {
	qaCmd.Flags().IntVar(&qaFlagConfidence, "confidence", 7, "Confidence threshold for parametric answers (1-10)")
	qaCmd.Flags().BoolVar(&qaFlagRetrieval, "retrieve", false, "Force retrieval (skip confidence check)")
	qaCmd.Flags().BoolVar(&qaFlagParametric, "parametric", false, "Force parametric (never retrieve)")
	qaCmd.Flags().BoolVarP(&qaFlagExplain, "explain", "e", false, "Show reasoning and sources")
	qaCmd.Flags().StringVar(&qaFlagModel, "model", "", "LLM model (default: gpt-4o or LLM_MODEL env)")
	qaCmd.Flags().StringVar(&qaFlagBaseURL, "llm-url", "", "LLM API base URL (default: LLM_BASE_URL env)")
	qaCmd.Flags().BoolVar(&qaFlagClaude, "claude", false, "Use Claude CLI (no API key needed)")
	qaCmd.Flags().BoolVar(&qaFlagCodex, "codex", false, "Use OpenAI Codex CLI (no API key needed)")
	qaCmd.Flags().BoolVar(&qaFlagOpus, "opus", false, "Use Claude Opus model (with --claude)")
	qaCmd.Flags().BoolVar(&qaFlagUnsafe, "unsafe", false, "Enable full LLM access (DANGEROUS: bypasses sandbox)")

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

// qaConfig holds resolved configuration for QA command.
type qaConfig struct {
	useClaude   bool
	useCodex    bool
	useOpus     bool
	model       string
	baseURL     string
	unsafe      bool
	confidence  int
	retrieve    bool
	parametric  bool
	explain     bool
	jsonOutput  bool
	humanOutput bool
}

// resolveQAConfig gathers and validates all QA flags into a config struct.
func resolveQAConfig(cmd *cobra.Command) (*qaConfig, error) {
	cfg := &qaConfig{
		useClaude:   qaFlagClaude,
		useCodex:    qaFlagCodex,
		useOpus:     qaFlagOpus,
		model:       qaFlagModel,
		baseURL:     qaFlagBaseURL,
		unsafe:      qaFlagUnsafe,
		confidence:  qaFlagConfidence,
		retrieve:    qaFlagRetrieval,
		parametric:  qaFlagParametric,
		explain:     qaFlagExplain,
		jsonOutput:  flagJSON,
		humanOutput: flagHuman,
	}

	if cfg.useClaude && cfg.useCodex {
		return nil, fmt.Errorf("--claude and --codex are mutually exclusive")
	}

	if cfg.unsafe {
		fmt.Fprintln(cmd.ErrOrStderr(), "⚠️  WARNING: --unsafe enables full LLM access. The model can execute arbitrary commands.")
	}

	return cfg, nil
}

// createQAClient builds the appropriate LLM client based on config.
func createQAClient(cfg *qaConfig) (LLMCompleter, error) {
	securityCfg := llm.ForQA()
	if cfg.unsafe {
		securityCfg = securityCfg.WithFullAccess()
	}

	if cfg.useCodex {
		return createCodexClient(cfg, securityCfg)
	}
	if cfg.useClaude {
		return createClaudeClient(cfg, securityCfg)
	}
	return createOpenAIClient(cfg), nil
}

// createCodexClient builds a Codex LLM client.
func createCodexClient(cfg *qaConfig, securityCfg llm.SecurityConfig) (LLMCompleter, error) {
	opts := []llm.CodexOption{llm.WithSecurityConfig(securityCfg)}
	if cfg.model != "" {
		opts = append(opts, llm.WithCodexModel(cfg.model))
	}
	client, err := llm.NewCodexClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("codex setup: %w", err)
	}
	return client, nil
}

// createClaudeClient builds a Claude LLM client.
func createClaudeClient(cfg *qaConfig, securityCfg llm.SecurityConfig) (LLMCompleter, error) {
	opts := []llm.ClaudeOption{llm.WithClaudeSecurityConfig(securityCfg)}
	if cfg.model != "" {
		opts = append(opts, llm.WithClaudeModel(cfg.model))
	}
	if cfg.useOpus {
		opts = append(opts, llm.WithOpus(true))
	}
	client, err := llm.NewClaudeClientWithOptions(opts...)
	if err != nil {
		return nil, fmt.Errorf("claude setup: %w", err)
	}
	return client, nil
}

// createOpenAIClient builds an OpenAI-compatible LLM client.
func createOpenAIClient(cfg *qaConfig) LLMCompleter {
	var opts []llm.Option
	if cfg.model != "" {
		opts = append(opts, llm.WithModel(cfg.model))
	}
	if cfg.baseURL != "" {
		opts = append(opts, llm.WithBaseURL(cfg.baseURL))
	}
	return llm.NewClient(opts...)
}

// processQAQuestion runs the QA engine and returns the result.
func processQAQuestion(ctx context.Context, question string, cfg *qaConfig, client LLMCompleter) (*qa.Result, error) {
	engineCfg := qa.DefaultConfig()
	engineCfg.ConfidenceThreshold = cfg.confidence
	engineCfg.ForceRetrieval = cfg.retrieve
	engineCfg.ForceParametric = cfg.parametric
	engineCfg.Verbose = cfg.explain

	engine := qa.NewEngine(client, newEutilsClient(), engineCfg)

	result, err := engine.Answer(ctx, question)
	if err != nil {
		return nil, fmt.Errorf("qa failed: %w", err)
	}
	return result, nil
}

// formatQAResult outputs the result in the appropriate format.
func formatQAResult(result *qa.Result, cfg *qaConfig) error {
	if cfg.jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if cfg.explain || cfg.humanOutput {
		printExplainedResult(result)
	} else {
		fmt.Println(result.Answer)
	}
	return nil
}

func runQA(cmd *cobra.Command, args []string) error {
	question := strings.Join(args, " ")

	cfg, err := resolveQAConfig(cmd)
	if err != nil {
		return err
	}

	client, err := createQAClient(cfg)
	if err != nil {
		return err
	}

	result, err := processQAQuestion(cmd.Context(), question, cfg, client)
	if err != nil {
		return err
	}

	return formatQAResult(result, cfg)
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
