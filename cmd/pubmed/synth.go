package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/henrybloomingdale/pubmed-cli/internal/llm"
	"github.com/henrybloomingdale/pubmed-cli/internal/synth"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var (
	synthFlagPapers    int
	synthFlagSearch    int
	synthFlagRelevance int
	synthFlagWords     int
	synthFlagDocx      string
	synthFlagRIS       string
	synthFlagBibTeX    string
	synthFlagPMID      string
	synthFlagModel     string
	synthFlagBaseURL   string
	synthFlagClaude    bool
	synthFlagCodex     bool
	synthFlagOpus      bool
	synthFlagMd        bool
	synthFlagUnsafe    bool
)

func init() {
	synthCmd.Flags().IntVar(&synthFlagPapers, "papers", 5, "Number of papers to include in synthesis")
	synthCmd.Flags().IntVar(&synthFlagSearch, "search", 30, "Number of papers to search before filtering")
	synthCmd.Flags().IntVar(&synthFlagRelevance, "relevance", 7, "Minimum relevance score (1-10)")
	synthCmd.Flags().IntVar(&synthFlagWords, "words", 250, "Target word count")
	synthCmd.Flags().StringVar(&synthFlagDocx, "docx", "", "Output Word document")
	synthCmd.Flags().StringVar(&synthFlagRIS, "ris", "", "Output RIS file for reference managers")
	synthCmd.Flags().StringVar(&synthFlagBibTeX, "bibtex", "", "Output BibTeX file for LaTeX workflows")
	synthCmd.Flags().StringVar(&synthFlagPMID, "pmid", "", "Deep dive on single paper by PMID")
	synthCmd.Flags().StringVar(&synthFlagModel, "model", "", "LLM model (default: gpt-4o or LLM_MODEL env)")
	synthCmd.Flags().StringVar(&synthFlagBaseURL, "llm-url", "", "LLM API base URL")
	synthCmd.Flags().BoolVar(&synthFlagClaude, "claude", false, "Use Claude CLI (no API key needed)")
	synthCmd.Flags().BoolVar(&synthFlagCodex, "codex", false, "Use OpenAI Codex CLI (no API key needed)")
	synthCmd.Flags().BoolVar(&synthFlagOpus, "opus", false, "Use Claude Opus model (with --claude)")
	synthCmd.Flags().BoolVar(&synthFlagMd, "md", false, "Output markdown to stdout (default if no --docx)")
	synthCmd.Flags().BoolVar(&synthFlagUnsafe, "unsafe", false, "Enable full LLM access (DANGEROUS: bypasses sandbox)")

	rootCmd.AddCommand(synthCmd)
}

var synthCmd = &cobra.Command{
	Use:   "synth <question>",
	Short: "Synthesize literature on a topic with citations",
	Long: `Search PubMed, filter by relevance, and synthesize findings into paragraphs with citations.

Examples:
  # Basic synthesis (markdown output)
  pubmed synth "SGLT-2 inhibitors in liver fibrosis"

  # Word document + RIS file
  pubmed synth "CBT for pediatric anxiety" --docx review.docx --ris refs.ris

  # BibTeX export
  pubmed synth "CBT for pediatric anxiety" --bibtex refs.bib

  # More papers, longer output
  pubmed synth "autism biomarkers" --papers 10 --words 500

  # Single paper deep dive
  pubmed synth --pmid 41234567 --words 400

  # JSON for agents
  pubmed synth "treatments for fragile x" --json

Environment:
  LLM_API_KEY   - API key for LLM
  LLM_BASE_URL  - Base URL for OpenAI-compatible API
  LLM_MODEL     - Model name (default: gpt-4o)`,
	Args: cobra.ArbitraryArgs,
	RunE: runSynth,
}

// synthConfig holds resolved configuration for a synthesis run.
type synthConfig struct {
	question    string
	pmid        string
	useTUI      bool
	securityCfg llm.SecurityConfig
	synthCfg    synth.Config
}

// validateSynthFlags validates command-line arguments and flag values.
func validateSynthFlags(cmd *cobra.Command, args []string) error {
	pmid := strings.TrimSpace(synthFlagPMID)
	if pmid == "" && len(args) == 0 {
		return fmt.Errorf("provide a question or use --pmid for single paper")
	}
	if pmid != "" && len(args) > 0 {
		return fmt.Errorf("provide either a question or --pmid, not both")
	}

	if synthFlagPapers < 1 {
		return fmt.Errorf("--papers must be >= 1")
	}
	if synthFlagSearch < 1 {
		return fmt.Errorf("--search must be >= 1")
	}
	if synthFlagWords < 1 {
		return fmt.Errorf("--words must be >= 1")
	}
	if synthFlagRelevance < 1 || synthFlagRelevance > 10 {
		return fmt.Errorf("--relevance must be 1-10")
	}

	if synthFlagClaude && synthFlagCodex {
		return fmt.Errorf("--claude and --codex are mutually exclusive")
	}

	return nil
}

// resolveSynthConfig builds configuration from command-line flags.
func resolveSynthConfig(cmd *cobra.Command, args []string) *synthConfig {
	// Adjust search to ensure we have enough papers.
	search := synthFlagSearch
	if synthFlagPapers > search {
		search = synthFlagPapers
	}

	// Determine security config.
	securityCfg := llm.ForSynthesis()
	if synthFlagUnsafe {
		fmt.Fprintln(cmd.ErrOrStderr(), "⚠️  WARNING: --unsafe enables full LLM access. The model can execute arbitrary commands.")
		securityCfg = securityCfg.WithFullAccess()
	}

	// Build synth config.
	cfg := synth.DefaultConfig()
	cfg.PapersToUse = synthFlagPapers
	cfg.PapersToSearch = search
	cfg.RelevanceThreshold = synthFlagRelevance
	cfg.TargetWords = synthFlagWords

	return &synthConfig{
		question:    strings.TrimSpace(strings.Join(args, " ")),
		pmid:        strings.TrimSpace(synthFlagPMID),
		useTUI:      isatty.IsTerminal(os.Stderr.Fd()) && isatty.IsTerminal(os.Stdin.Fd()),
		securityCfg: securityCfg,
		synthCfg:    cfg,
	}
}

// createSynthLLMClient creates the appropriate LLM client based on flags.
func createSynthLLMClient(securityCfg llm.SecurityConfig) (synth.LLMClient, error) {
	if synthFlagCodex {
		opts := []llm.CodexOption{llm.WithSecurityConfig(securityCfg)}
		if synthFlagModel != "" {
			opts = append(opts, llm.WithCodexModel(synthFlagModel))
		}
		return llm.NewCodexClient(opts...)
	}

	if synthFlagClaude {
		opts := []llm.ClaudeOption{llm.WithClaudeSecurityConfig(securityCfg)}
		if synthFlagModel != "" {
			opts = append(opts, llm.WithClaudeModel(synthFlagModel))
		}
		if synthFlagOpus {
			opts = append(opts, llm.WithOpus(true))
		}
		return llm.NewClaudeClientWithOptions(opts...)
	}

	// Default: OpenAI-compatible client.
	var opts []llm.Option
	if synthFlagModel != "" {
		opts = append(opts, llm.WithModel(synthFlagModel))
	}
	if synthFlagBaseURL != "" {
		opts = append(opts, llm.WithBaseURL(synthFlagBaseURL))
	}
	return llm.NewClient(opts...), nil
}

// runSynthWithTUI runs synthesis with an interactive terminal UI.
func runSynthWithTUI(ctx context.Context, engine *synth.Engine, cfg *synthConfig) (*synth.Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	progressCh := make(chan synth.ProgressUpdate, 1024)
	engine.WithProgress(func(u synth.ProgressUpdate) {
		select {
		case progressCh <- u:
		default:
		}
	})

	out := make(chan struct {
		res *synth.Result
		err error
	}, 1)

	go func() {
		defer close(progressCh)
		var r *synth.Result
		var e error
		if cfg.pmid != "" {
			r, e = engine.SynthesizePMID(ctx, cfg.pmid)
		} else {
			r, e = engine.Synthesize(ctx, cfg.question)
		}
		out <- struct {
			res *synth.Result
			err error
		}{res: r, err: e}
	}()

	p := tea.NewProgram(
		newSynthProgressModel(progressCh, cancel),
		tea.WithOutput(os.Stderr),
	)
	if _, uiErr := p.Run(); uiErr != nil {
		cancel()
		<-out
		return nil, uiErr
	}
	o := <-out
	return o.res, o.err
}

// runSynthPlain runs synthesis with plain-text progress output.
func runSynthPlain(ctx context.Context, engine *synth.Engine, cfg *synthConfig) (*synth.Result, error) {
	lastMsg := ""
	engine.WithProgress(func(u synth.ProgressUpdate) {
		if u.Message != "" && u.Message != lastMsg {
			lastMsg = u.Message
			fmt.Fprintln(os.Stderr, u.Message)
		}
	})

	if cfg.pmid != "" {
		return engine.SynthesizePMID(ctx, cfg.pmid)
	}
	return engine.Synthesize(ctx, cfg.question)
}

// writeRISFile writes RIS format references to the specified path.
func writeRISFile(path string, result *synth.Result) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create RIS dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(result.RIS), 0o644); err != nil {
		return fmt.Errorf("write RIS file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ Wrote %s (%d references)\n", path, len(result.References))
	return nil
}

// writeBibTeXFile writes BibTeX format references to the specified path.
func writeBibTeXFile(path string, result *synth.Result) error {
	bibtex := synth.GenerateBibTeX(result.References)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create BibTeX dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(bibtex), 0o644); err != nil {
		return fmt.Errorf("write BibTeX file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ Wrote %s (%d references)\n", path, len(result.References))
	return nil
}

// handleSynthResult writes output files and displays results.
func handleSynthResult(ctx context.Context, result *synth.Result) error {
	if result == nil {
		return errors.New("synthesis returned nil result")
	}

	// Write RIS file if requested.
	if synthFlagRIS != "" {
		if err := writeRISFile(synthFlagRIS, result); err != nil {
			return err
		}
	}

	// Write BibTeX file if requested.
	if synthFlagBibTeX != "" {
		if err := writeBibTeXFile(synthFlagBibTeX, result); err != nil {
			return err
		}
	}

	// Write DOCX if requested.
	if synthFlagDocx != "" {
		if err := writeDocx(ctx, synthFlagDocx, result); err != nil {
			var w *docxFallbackWarning
			if errors.As(err, &w) {
				fmt.Fprintln(os.Stderr, w.Error())
			} else {
				return fmt.Errorf("write DOCX: %w", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "✓ Wrote %s\n", synthFlagDocx)
		}
	}

	// Output.
	if flagJSON {
		return outputJSON(result)
	}
	// If the user requested a file output, default to being quiet unless --md is set.
	if synthFlagDocx != "" && !synthFlagMd {
		return nil
	}
	return outputMarkdown(result)
}

// runSynth orchestrates the synthesis workflow.
func runSynth(cmd *cobra.Command, args []string) error {
	if err := validateSynthFlags(cmd, args); err != nil {
		return err
	}

	cfg := resolveSynthConfig(cmd, args)

	llmClient, err := createSynthLLMClient(cfg.securityCfg)
	if err != nil {
		return fmt.Errorf("llm setup: %w", err)
	}

	engine := synth.NewEngine(llmClient, newEutilsClient(), cfg.synthCfg)

	var result *synth.Result
	if cfg.useTUI {
		result, err = runSynthWithTUI(cmd.Context(), engine, cfg)
	} else {
		result, err = runSynthPlain(cmd.Context(), engine, cfg)
	}
	if err != nil {
		return fmt.Errorf("synthesize: %w", err)
	}

	return handleSynthResult(cmd.Context(), result)
}

func outputJSON(result *synth.Result) error {
	if result == nil {
		return errors.New("result is nil")
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	// Add BibTeX to the JSON output without changing the core synth.Result contract.
	out := struct {
		*synth.Result
		BibTeX string `json:"bibtex,omitempty"`
	}{
		Result: result,
		BibTeX: synth.GenerateBibTeX(result.References),
	}
	return enc.Encode(out)
}

func outputMarkdown(result *synth.Result) error {
	if result == nil {
		return errors.New("result is nil")
	}

	var sb strings.Builder

	// Header.
	sb.WriteString(fmt.Sprintf("# %s\n\n", result.Question))

	// Stats.
	sb.WriteString(fmt.Sprintf("*Searched %d papers, scored %d, used %d*\n\n",
		result.PapersSearched, result.PapersScored, result.PapersUsed))

	// Synthesis.
	sb.WriteString("## Synthesis\n\n")
	sb.WriteString(result.Synthesis)
	sb.WriteString("\n\n")

	// References.
	sb.WriteString("## References\n\n")
	for i, ref := range result.References {
		sb.WriteString(fmt.Sprintf("%d. %s (relevance: %d/10) [PMID: %s]\n",
			i+1, ref.CitationAPA, ref.RelevanceScore, ref.PMID))
	}

	// Token usage.
	sb.WriteString(fmt.Sprintf("\n---\n*Tokens: ~%d input, ~%d output, ~%d total*\n",
		result.Tokens.Input, result.Tokens.Output, result.Tokens.Total))

	_, err := fmt.Fprint(os.Stdout, sb.String())
	return err
}

type docxFallbackWarning struct {
	DocxPath     string
	MarkdownPath string
	Cause        error
}

func (w *docxFallbackWarning) Error() string {
	return fmt.Sprintf("DOCX conversion failed; wrote markdown instead: %s (requested DOCX: %s): %v", w.MarkdownPath, w.DocxPath, w.Cause)
}

func (w *docxFallbackWarning) Unwrap() error { return w.Cause }

// writeDocx creates a Word document with synthesis and references.
// Implementation strategy: write a temporary markdown file and convert via pandoc.
func writeDocx(ctx context.Context, filename string, result *synth.Result) error {
	// convertToDocx accepts a context for cancellation.
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return errors.New("filename is required")
	}
	if strings.HasSuffix(filename, "/") || strings.HasSuffix(filename, "\\") {
		return errors.New("filename must be a file path, not a directory")
	}
	if result == nil {
		return errors.New("result is nil")
	}

	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	f, err := os.CreateTemp("", "pubmed-synth-*.md")
	if err != nil {
		return fmt.Errorf("create temp markdown: %w", err)
	}
	tmpMD := f.Name()
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp markdown: %w", err)
	}
	defer os.Remove(tmpMD) // best-effort cleanup

	if err := saveMarkdownFile(tmpMD, result); err != nil {
		return fmt.Errorf("write temp markdown: %w", err)
	}
	if err := convertToDocxContext(ctx, tmpMD, filename); err != nil {
		mdOut := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".md"
		if err2 := saveMarkdownFile(mdOut, result); err2 != nil {
			return fmt.Errorf("pandoc conversion failed (%w); additionally failed to write markdown fallback %q: %w", err, mdOut, err2)
		}
		return &docxFallbackWarning{DocxPath: filename, MarkdownPath: mdOut, Cause: err}
	}
	return nil
}

// --- progress UI (Charm) ---

type synthProgressMsg synth.ProgressUpdate

type synthProgressDoneMsg struct{}

type synthProgressModel struct {
	ch     <-chan synth.ProgressUpdate
	cancel context.CancelFunc

	spinner spinner.Model
	bar     progress.Model

	phase   synth.ProgressPhase
	message string
	current int
	total   int
}

func newSynthProgressModel(ch <-chan synth.ProgressUpdate, cancel context.CancelFunc) synthProgressModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	bar := progress.New(progress.WithDefaultGradient())
	bar.Width = 32

	return synthProgressModel{
		ch:      ch,
		cancel:  cancel,
		spinner: sp,
		bar:     bar,
		message: "Working...",
	}
}

func (m synthProgressModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, waitSynthProgress(m.ch))
}

func waitSynthProgress(ch <-chan synth.ProgressUpdate) tea.Cmd {
	return func() tea.Msg {
		u, ok := <-ch
		if !ok {
			return synthProgressDoneMsg{}
		}
		return synthProgressMsg(u)
	}
}

func (m synthProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case synthProgressMsg:
		u := synth.ProgressUpdate(msg)
		m.phase = u.Phase
		if strings.TrimSpace(u.Message) != "" {
			m.message = u.Message
		}
		m.current = u.Current
		m.total = u.Total
		return m, waitSynthProgress(m.ch)

	case synthProgressDoneMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m synthProgressModel) View() string {
	bold := lipgloss.NewStyle().Bold(true)
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	line := fmt.Sprintf("%s %s", m.spinner.View(), bold.Render(m.message))
	if m.phase == synth.ProgressScore && m.total > 0 {
		pct := float64(m.current) / float64(m.total)
		bar := m.bar.ViewAs(pct)
		count := subtle.Render(fmt.Sprintf(" %d/%d", m.current, m.total))
		return line + "\n" + bar + count + "\n"
	}
	return line + "\n"
}
